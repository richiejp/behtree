package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mudler/cogito"
	"github.com/mudler/cogito/clients"
	"github.com/richiejp/behtree"
	openai "github.com/sashabaranov/go-openai"
)

// treeSchema defines the JSON schema for structured output mode.
// Uses strict:false because the params field needs dynamic keys
// (additionalProperties), which is incompatible with strict:true
// in the OpenAI API. Providers that don't enforce strict mode
// (LocalAI, Anthropic) work fine either way.
var treeSchema = json.RawMessage(`{
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

func buildSystemPrompt(env *behtree.Environment) string {
	ctx := envContext{
		Objects:    env.Objects,
		Behaviours: env.Behaviours,
	}
	data, _ := json.MarshalIndent(ctx, "", "  ")
	return fmt.Sprintf(systemPrompt, string(data))
}

func makeGenerateTree(llm cogito.LLM, model, jsonMode string, verbose bool) func(string, *behtree.Environment) ([]byte, error) {
	return func(prompt string, env *behtree.Environment) ([]byte, error) {
		sysPrompt := buildSystemPrompt(env)

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
					Schema: treeSchema,
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
			content = extractJSON(content)
		}

		return []byte(content), nil
	}
}

// extractJSON tries to extract valid JSON from LLM output that may
// be wrapped in markdown code blocks or contain extra text.
func extractJSON(s string) string {
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

func robotCase() *behtree.BenchmarkCase {
	doc, err := behtree.LoadDocument("testdata/robot.json")
	if err != nil {
		log.Fatalf("Failed to load robot.json: %v", err)
	}

	env := behtree.MergeDocuments(&behtree.Document{
		Objects:    doc.Objects,
		Behaviours: doc.Behaviours,
	})

	return &behtree.BenchmarkCase{
		Name:        "Robot Pick-and-Place",
		Description: "Generate a tree to pick up a wrapper from the table and drop it in the bin",
		Difficulty:  behtree.DifficultySimple,
		Environment: env,
		Prompt:      "Pick up the wrapper from the table and drop it in the bin.",
		Simulate:    robotSimulate,
	}
}

func robotSimulate(tree *behtree.Node, env *behtree.Environment, registry *behtree.BehaviourRegistry) []*behtree.ScenarioResult {
	registry.Register("IsHolding", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		obj := params["object"].(string)
		holding, _ := s.Get("robot", "holding")
		if holding == obj {
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		}
		return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
	})

	registry.Register("IsAt", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		loc := params["location"].(string)
		robotLoc, _ := s.Get("robot", "location")
		if robotLoc == loc {
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		}
		return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
	})

	registry.Register("NavigateTo", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		loc := params["location"].(string)
		robotLoc, _ := s.Get("robot", "location")

		if req == behtree.RequestRunning {
			return behtree.HandlerResult{Status: behtree.Running, Compatible: true}
		}
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}

		if robotLoc == loc {
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		}
		_ = s.Set("robot", "location", loc)
		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	})

	registry.Register("PickUp", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		obj := params["object"].(string)
		robotLoc, _ := s.Get("robot", "location")
		objLoc, _ := s.Get(obj, "location")
		if robotLoc != objLoc {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: req == behtree.RequestFailure}
		}
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}
		_ = s.Set("robot", "holding", obj)
		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	})

	registry.Register("DropIn", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		obj := params["object"].(string)
		container := params["container"].(string)
		holding, _ := s.Get("robot", "holding")
		robotLoc, _ := s.Get("robot", "location")
		if holding != obj || robotLoc != container {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: req == behtree.RequestFailure}
		}
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}
		_ = s.Set("robot", "holding", "")
		_ = s.Set(obj, "location", container)
		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	})

	harness := behtree.NewSimulationHarness(env, registry, tree)
	return harness.RunAllOutcomes(10)
}

func desktopCase() *behtree.BenchmarkCase {
	doc, err := behtree.LoadDocument("testdata/desktop_env.json")
	if err != nil {
		log.Fatalf("Failed to load desktop_env.json: %v", err)
	}

	env := behtree.MergeDocuments(doc)

	return &behtree.BenchmarkCase{
		Name:        "Desktop Open URL",
		Description: "Generate a tree to open a URL in Firefox, ensuring the browser is open and window is focused",
		Difficulty:  behtree.DifficultyModerate,
		Environment: env,
		Prompt:      "Open the LocalAI GitHub page (https://github.com/mudler/LocalAI) in Firefox. First ensure Firefox is open, then navigate to the URL, then ensure the window is focused.",
	}
}

type config struct {
	model      string
	apiKey     string
	baseURL    string
	provider   string
	caseFilter string
	verbose    bool
	jsonMode   string
}

func parseFlags() config {
	cfg := config{}
	flag.StringVar(&cfg.model, "model", "", "LLM model name (required)")
	flag.StringVar(&cfg.apiKey, "api-key", "", "API key (env: API_KEY)")
	flag.StringVar(&cfg.baseURL, "base-url", "", "API base URL (env: BASE_URL)")
	flag.StringVar(&cfg.provider, "provider", "openai", "LLM provider: openai or localai")
	flag.StringVar(&cfg.caseFilter, "case", "", "Run only the named case")
	flag.BoolVar(&cfg.verbose, "verbose", false, "Print generated trees and extra info")
	flag.StringVar(&cfg.jsonMode, "json-mode", "json_schema", "JSON output mode: json_schema or object")
	flag.Parse()

	if cfg.apiKey == "" {
		cfg.apiKey = os.Getenv("API_KEY")
	}
	if cfg.baseURL == "" {
		cfg.baseURL = os.Getenv("BASE_URL")
	}
	if cfg.model == "" {
		log.Fatal("-model is required")
	}

	return cfg
}

func createLLM(cfg config) cogito.LLM {
	switch cfg.provider {
	case "openai":
		return clients.NewOpenAILLM(cfg.model, cfg.apiKey, cfg.baseURL)
	case "localai":
		return clients.NewLocalAILLM(cfg.model, cfg.apiKey, cfg.baseURL)
	default:
		log.Fatalf("unknown provider %q (use openai or localai)", cfg.provider)
		return nil
	}
}

func printResults(results []*behtree.BenchmarkResult, verbose bool) int {
	passed := 0
	for _, r := range results {
		status := "FAIL"
		if r.Passed {
			status = "PASS"
			passed++
		}

		fmt.Printf("[%s] %s (difficulty: %s)\n", status, r.Case.Name, r.Case.Difficulty)

		if r.ParseError != nil {
			fmt.Printf("  parse error: %v\n", r.ParseError)
		}

		if len(r.Validation) > 0 {
			fmt.Printf("  validation errors (%d):\n", len(r.Validation))
			for _, ve := range r.Validation {
				fmt.Printf("    - %s\n", ve)
			}
		}

		if r.Scenarios != nil {
			total := len(r.Scenarios)
			skipped := 0
			failed := 0
			for _, sr := range r.Scenarios {
				if sr.Skipped {
					skipped++
					continue
				}
				lastTick := sr.Ticks[len(sr.Ticks)-1]
				if lastTick.Err != nil || lastTick.Status == behtree.Failure {
					failed++
				}
			}
			fmt.Printf("  scenarios: %d total, %d skipped, %d failed\n",
				total, skipped, failed)
		}

		if verbose && r.GeneratedTree != nil {
			fmt.Printf("  tree:\n")
			for _, line := range strings.Split(behtree.PrintTree(r.GeneratedTree), "\n") {
				if line != "" {
					fmt.Printf("    %s\n", line)
				}
			}
		}

		fmt.Println()
	}

	return passed
}

func main() {
	cfg := parseFlags()
	llm := createLLM(cfg)

	suite := behtree.NewBenchmarkSuite()
	suite.Add(robotCase())
	suite.Add(desktopCase())

	if cfg.caseFilter != "" {
		var filtered []*behtree.BenchmarkCase
		for _, c := range suite.Cases {
			if c.Name == cfg.caseFilter {
				filtered = append(filtered, c)
			}
		}
		if len(filtered) == 0 {
			log.Fatalf("no case matching %q", cfg.caseFilter)
		}
		suite.Cases = filtered
	}

	generateTree := makeGenerateTree(llm, cfg.model, cfg.jsonMode, cfg.verbose)

	fmt.Printf("Running %d benchmark case(s) with model %q (provider: %s, json-mode: %s)\n\n",
		len(suite.Cases), cfg.model, cfg.provider, cfg.jsonMode)

	results := suite.Run(generateTree)
	passed := printResults(results, cfg.verbose)

	fmt.Printf("Results: %d/%d passed\n", passed, len(results))

	if passed < len(results) {
		os.Exit(1)
	}
}
