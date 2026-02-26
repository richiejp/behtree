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

// GenerateTreeFunc is the signature for a function that generates a
// behaviour tree from an LLM given a prompt and environment.
type GenerateTreeFunc = func(prompt string, env *behtree.Environment) ([]byte, error)

// TreeSchema defines the JSON schema for structured output mode.
// Uses strict:false because the params field needs dynamic keys
// (additionalProperties), which is incompatible with strict:true
// in the OpenAI API. Providers that don't enforce strict mode
// (LocalAI, Anthropic) work fine either way.
var TreeSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"tree": { "$ref": "#/$defs/node" }
	},
	"required": ["tree"],
	"additionalProperties": false,
	"$defs": {
		"node": {
			"type": "object",
			"properties": {
				"type": {
					"type": "string",
					"enum": ["Sequence", "Fallback", "Condition", "Action"]
				},
				"name": { "type": "string" },
				"params": {
					"type": "object",
					"additionalProperties": { "type": "string" }
				},
				"children": {
					"type": "array",
					"items": { "$ref": "#/$defs/node" }
				}
			},
			"required": ["type"],
			"additionalProperties": false
		}
	}
}`)

const systemPrompt = `You are an expert at constructing behaviour trees. Generate a behaviour tree in JSON format for the given task.

## Node Types

- **Sequence**: Executes children left to right. SUCCESS if ALL succeed. FAILURE immediately if any child fails. RUNNING if a child returns RUNNING.
- **Fallback**: Executes children left to right. SUCCESS immediately if any child succeeds. FAILURE if ALL fail. RUNNING if a child returns RUNNING.
- **Condition**: Leaf node that checks a condition. Returns SUCCESS or FAILURE (never RUNNING). Must reference a defined behaviour by name.
- **Action**: Leaf node that performs an action. Returns SUCCESS, FAILURE, or RUNNING. Must reference a defined behaviour by name.

## Key Pattern: Check-or-Do with Fallback

Use Fallback to implement "check if already done, otherwise do it":

    {
      "type": "Fallback",
      "children": [
        {"type": "Condition", "name": "IsAlreadyDone", "params": {"key": "value"}},
        {"type": "Action", "name": "DoTheThing", "params": {"key": "value"}}
      ]
    }

This ensures the action is only attempted if the condition is not already satisfied. Always prefer this pattern when a precondition must hold before continuing.

## JSON Format

Output a JSON document with a "tree" field containing the root node:

    {"tree": {"type": "Sequence", "children": [...]}}

- Control nodes (Sequence, Fallback): have "type" and "children". No "name" or "params".
- Leaf nodes (Condition, Action): have "type", "name", and "params". No "children".
- Parameters with type "object_ref" take the name of an object defined in the environment as their value.

## Environment

The following objects and behaviours are available:

%s

Generate a behaviour tree for the task described in the user message.`

type envContext struct {
	Objects    []behtree.ObjectDef    `json:"objects"`
	Behaviours []behtree.BehaviourDef `json:"behaviours"`
}

// BuildSystemPrompt constructs the system prompt for tree generation,
// embedding the environment's objects and behaviours as JSON context.
func BuildSystemPrompt(env *behtree.Environment) string {
	ctx := envContext{
		Objects:    env.Objects,
		Behaviours: env.Behaviours,
	}
	data, _ := json.MarshalIndent(ctx, "", "  ")
	return fmt.Sprintf(systemPrompt, string(data))
}

// NewGenerateTree returns a GenerateTreeFunc that uses the given LLM
// to generate behaviour trees.
func NewGenerateTree(llm cogito.LLM, model, jsonMode string, verbose bool) GenerateTreeFunc {
	return func(prompt string, env *behtree.Environment) ([]byte, error) {
		sysPrompt := BuildSystemPrompt(env)

		req := openai.ChatCompletionRequest{
			Model: model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: sysPrompt},
				{Role: openai.ChatMessageRoleUser, Content: prompt},
			},
		}

		switch jsonMode {
		case "json_schema":
			req.ResponseFormat = &openai.ChatCompletionResponseFormat{
				Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
				JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
					Name:   "behaviour_tree",
					Schema: TreeSchema,
					Strict: false,
				},
			}
		case "object":
			req.ResponseFormat = &openai.ChatCompletionResponseFormat{
				Type: openai.ChatCompletionResponseFormatTypeJSONObject,
			}
		}

		reply, usage, err := llm.CreateChatCompletion(context.Background(), req)
		if err != nil {
			return nil, fmt.Errorf("LLM call failed: %w", err)
		}

		if verbose {
			fmt.Printf("  tokens: prompt=%d completion=%d total=%d\n",
				usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
		}

		choices := reply.ChatCompletionResponse.Choices
		if len(choices) == 0 {
			return nil, fmt.Errorf("LLM returned no choices")
		}

		content := choices[0].Message.Content
		if content == "" {
			return nil, fmt.Errorf("LLM returned empty content")
		}

		if jsonMode == "object" {
			content = ExtractJSON(content)
		}

		return []byte(content), nil
	}
}

// ExtractJSON tries to extract valid JSON from LLM output that may
// be wrapped in markdown code blocks or contain extra text.
func ExtractJSON(s string) string {
	s = strings.TrimSpace(s)

	// Already valid JSON
	if json.Valid([]byte(s)) {
		return s
	}

	// Try markdown ```json ... ``` blocks
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

	// Try outermost { ... }
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
