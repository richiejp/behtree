package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mudler/cogito"
	"github.com/mudler/cogito/clients"
	"github.com/richiejp/behtree"
	"github.com/richiejp/behtree/internal/benchcases"
	"github.com/richiejp/behtree/internal/llmgen"
)

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

	robotCase, err := benchcases.RobotCase("testdata")
	if err != nil {
		log.Fatalf("Failed to load robot case: %v", err)
	}

	desktopCase, err := benchcases.DesktopCase("testdata")
	if err != nil {
		log.Fatalf("Failed to load desktop case: %v", err)
	}

	suite := behtree.NewBenchmarkSuite()
	suite.Add(robotCase)
	suite.Add(desktopCase)

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

	generateTree := llmgen.NewGenerateTree(llm, cfg.model, cfg.jsonMode, cfg.verbose)

	fmt.Printf("Running %d benchmark case(s) with model %q (provider: %s, json-mode: %s)\n\n",
		len(suite.Cases), cfg.model, cfg.provider, cfg.jsonMode)

	results := suite.Run(generateTree)
	passed := printResults(results, cfg.verbose)

	fmt.Printf("Results: %d/%d passed\n", passed, len(results))

	if passed < len(results) {
		os.Exit(1)
	}
}
