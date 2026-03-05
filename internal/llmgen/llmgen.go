package llmgen

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mudler/cogito"
	"github.com/richiejp/behtree"
	openai "github.com/sashabaranov/go-openai"
)

// ActionSelectionGBNF constrains LLM output to the action selection format.
const ActionSelectionGBNF = `root        ::= "{" ws goal-part "," ws actions-part ws "}"
goal-part   ::= "\"goal\"" ws ":" ws "[" ws cond-list ws "]"
actions-part ::= "\"actions\"" ws ":" ws "[" ws action-list ws "]"
cond-list   ::= cond ( "," ws cond )*
cond        ::= "{" ws "\"object\"" ws ":" ws string "," ws "\"field\"" ws ":" ws string "," ws "\"value\"" ws ":" ws string ws "}"
action-list ::= action ( "," ws action )*
action      ::= "{" ws "\"name\"" ws ":" ws string "," ws "\"params\"" ws ":" ws params ws "}"
params      ::= "{" ws param-list? ws "}"
param-list  ::= param ( "," ws param )*
param       ::= string ws ":" ws string
string      ::= "\"" chars "\""
chars       ::= char*
char        ::= [^"\\] | "\\" escape-char
escape-char ::= ["\\/bfnrt]
ws          ::= [ \t\n]*
`

// BehTreeGBNF is kept for backward compatibility with LocalAI provider setup.
var BehTreeGBNF = ActionSelectionGBNF

// LLMResponse is the expected JSON structure from the LLM.
type LLMResponse struct {
	Goal    []behtree.Condition       `json:"goal"`
	Actions []behtree.ActionSelection `json:"actions"`
}

// GenerateActionsFunc generates grounded action selections from an LLM.
type GenerateActionsFunc = func(prompt string, env *behtree.Environment) (*LLMResponse, error)

// GenerateTreeFunc is the legacy signature kept for backward compatibility.
type GenerateTreeFunc = func(prompt string, env *behtree.Environment) ([]byte, error)

const systemPrompt = `You are an expert at robot task planning. Given available actions and a task description, select which actions to execute (with grounded parameters) and specify the goal conditions.

## Output Format

Output a JSON object with two fields:
- "goal": array of conditions that must be true when the task is complete
- "actions": array of actions to execute, each with "name" and "params"

Example:
{"goal":[{"object":"wrapper","field":"location","value":"bin"}],"actions":[{"name":"NavigateTo","params":{"location":"table"}},{"name":"PickUp","params":{"object":"wrapper"}}]}

## Available Actions

Each action has parameters (which must reference existing objects), preconditions (what must be true before the action can execute), and postconditions (what becomes true after the action executes).

%s

## Objects

%s

Select the actions needed to achieve the task described in the user message. Ground each parameter with a specific object name from the environment. Include ALL actions needed, even if some preconditions will be satisfied by earlier actions. Order actions logically (the PA-BT algorithm will construct the correct tree structure).`

type actionDoc struct {
	Name           string                       `json:"name"`
	Params         map[string]behtree.ParamType `json:"params,omitempty"`
	Preconditions  []behtree.Condition          `json:"preconditions,omitempty"`
	Postconditions []behtree.Condition          `json:"postconditions,omitempty"`
}

// BuildSystemPrompt constructs the system prompt for action grounding.
func BuildSystemPrompt(env *behtree.Environment) string {
	var actions []actionDoc
	for _, a := range env.Actions {
		actions = append(actions, actionDoc{
			Name:           a.Name,
			Params:         a.Params,
			Preconditions:  a.Preconditions,
			Postconditions: a.Postconditions,
		})
	}
	actionsJSON, _ := json.MarshalIndent(actions, "", "  ")
	objectsJSON, _ := json.MarshalIndent(env.Objects, "", "  ")
	return fmt.Sprintf(systemPrompt, string(actionsJSON), string(objectsJSON))
}

// NewGenerateActions returns a function that uses the given LLM to generate
// action selections for the PA-BT pipeline.
func NewGenerateActions(llm cogito.LLM, model string, verbose bool) GenerateActionsFunc {
	return func(prompt string, env *behtree.Environment) (*LLMResponse, error) {
		sysPrompt := BuildSystemPrompt(env)

		req := openai.ChatCompletionRequest{
			Model:     model,
			MaxTokens: 4096,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: sysPrompt},
				{Role: openai.ChatMessageRoleUser, Content: prompt},
			},
		}

		reply, usage, err := llm.CreateChatCompletion(context.Background(), req)
		if err != nil {
			return nil, fmt.Errorf("LLM call failed: %w", err)
		}

		if verbose {
			fmt.Printf("  tokens: prompt=%d completion=%d total=%d\n",
				usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
			fmt.Printf("  stop_reason: %s\n", reply.ChatCompletionResponse.Choices[0].FinishReason)
		}

		choices := reply.ChatCompletionResponse.Choices
		if len(choices) == 0 {
			return nil, fmt.Errorf("LLM returned no choices")
		}

		content := choices[0].Message.Content
		if verbose {
			fmt.Printf("  raw response: %s\n", content)
		}
		if content == "" {
			return nil, fmt.Errorf("LLM returned empty content")
		}

		content = ExtractJSON(content)

		var resp LLMResponse
		if err := json.Unmarshal([]byte(content), &resp); err != nil {
			return nil, fmt.Errorf("parse LLM response: %w", err)
		}

		return &resp, nil
	}
}

// NewGenerateTree wraps NewGenerateActions to return raw JSON bytes for
// backward compatibility with the benchmark framework.
func NewGenerateTree(llm cogito.LLM, model string, verbose bool) GenerateTreeFunc {
	genActions := NewGenerateActions(llm, model, verbose)
	return func(prompt string, env *behtree.Environment) ([]byte, error) {
		resp, err := genActions(prompt, env)
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(resp)
		if err != nil {
			return nil, fmt.Errorf("marshal LLM response: %w", err)
		}
		return data, nil
	}
}

// ExtractJSON tries to extract valid JSON from LLM output that may
// be wrapped in markdown code blocks or contain extra text.
func ExtractJSON(s string) string {
	s = strings.TrimSpace(s)

	if json.Valid([]byte(s)) {
		return s
	}

	if idx := strings.Index(s, "```json"); idx >= 0 {
		start := idx + len("```json")
		if end := strings.Index(s[start:], "```"); end >= 0 {
			candidate := strings.TrimSpace(s[start : start+end])
			if json.Valid([]byte(candidate)) {
				return candidate
			}
		}
	}
	if idx := strings.Index(s, "```"); idx >= 0 {
		start := idx + len("```")
		if end := strings.Index(s[start:], "```"); end >= 0 {
			candidate := strings.TrimSpace(s[start : start+end])
			if json.Valid([]byte(candidate)) {
				return candidate
			}
		}
	}

	first := strings.IndexByte(s, '{')
	last := strings.LastIndexByte(s, '}')
	if first >= 0 && last > first {
		candidate := s[first : last+1]
		if json.Valid([]byte(candidate)) {
			return candidate
		}
	}

	return s
}
