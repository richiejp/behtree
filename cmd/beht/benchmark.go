package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mudler/cogito"
	"github.com/mudler/cogito/clients"
	"github.com/richiejp/behtree"
	"github.com/richiejp/behtree/internal/benchcases"
	"github.com/richiejp/behtree/internal/llmgen"
)

type benchConfig struct {
	model        string
	apiKey       string
	baseURL      string
	provider     string
	caseFilter   string
	verbose      bool
	traceDir     string
	traceAll     bool
	saveTreesDir string
}

func parseBenchFlags() benchConfig {
	cfg := benchConfig{}
	flag.StringVar(&cfg.model, "model", "", "LLM model name (required)")
	flag.StringVar(&cfg.apiKey, "api-key", "", "API key (env: API_KEY)")
	flag.StringVar(&cfg.baseURL, "base-url", "", "API base URL (env: BASE_URL)")
	flag.StringVar(&cfg.provider, "provider", "openai", "LLM provider: openai or localai")
	flag.StringVar(&cfg.caseFilter, "case", "", "Run only the named case")
	flag.BoolVar(&cfg.verbose, "verbose", false, "Print generated trees and extra info")
	flag.StringVar(&cfg.traceDir, "trace-dir", "", "Directory to write trace files")
	flag.BoolVar(&cfg.traceAll, "trace-all", false, "Trace all scenarios, not just failures")
	flag.StringVar(&cfg.saveTreesDir, "save-trees", "", "Directory to save generated trees")
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

func createLLM(cfg benchConfig) cogito.LLM {
	switch cfg.provider {
	case "openai":
		return clients.NewOpenAILLM(cfg.model, cfg.apiKey, cfg.baseURL)
	case "localai":
		client := clients.NewLocalAILLM(cfg.model, cfg.apiKey, cfg.baseURL)
		client.SetGrammar(llmgen.BehTreeGBNF)
		return client
	default:
		log.Fatalf("unknown provider %q (use openai or localai)", cfg.provider)
		return nil
	}
}

func printBenchResults(results []*behtree.BenchmarkResult, cfg benchConfig) int {
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
			printScenarioSummary(r, cfg)
		}

		if cfg.verbose && r.GeneratedTree != nil {
			fmt.Printf("  environment:\n")
			for _, line := range strings.Split(behtree.PrintEnvironmentString(r.Case.Environment), "\n") {
				if line != "" {
					fmt.Printf("    %s\n", line)
				}
			}
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

func printScenarioSummary(r *behtree.BenchmarkResult, cfg benchConfig) {
	total := len(r.Scenarios)
	skipped := 0
	failed := 0
	var failedTraces []*behtree.ScenarioTrace

	for _, sr := range r.Scenarios {
		if sr.Skipped {
			skipped++
			continue
		}
		lastTick := sr.Ticks[len(sr.Ticks)-1]
		if lastTick.Err != nil || lastTick.Status == behtree.Failure {
			failed++
			if sr.Trace != nil {
				failedTraces = append(failedTraces, sr.Trace)
			}
		}
	}
	fmt.Printf("  scenarios: %d total, %d skipped, %d failed\n", total, skipped, failed)

	if cfg.traceDir != "" {
		writeTraceFile(r, cfg, failedTraces)
	} else if cfg.verbose && len(failedTraces) > 0 {
		// Show first few inline when no trace dir
		limit := 5
		if len(failedTraces) < limit {
			limit = len(failedTraces)
		}
		fmt.Printf("  first %d failed scenario traces:\n", limit)
		for i := range limit {
			fmt.Printf("    --- scenario %d ---\n", i+1)
			var sb strings.Builder
			behtree.PrintScenarioTrace(failedTraces[i], &sb)
			for _, line := range strings.Split(sb.String(), "\n") {
				if line != "" {
					fmt.Printf("      %s\n", line)
				}
			}
		}
		if len(failedTraces) > limit {
			fmt.Printf("    ... and %d more (use -trace-dir to write all)\n", len(failedTraces)-limit)
		}
	}
}

func writeTraceFile(r *behtree.BenchmarkResult, cfg benchConfig, failedTraces []*behtree.ScenarioTrace) {
	if err := os.MkdirAll(cfg.traceDir, 0o755); err != nil {
		fmt.Printf("  error creating trace dir: %v\n", err)
		return
	}

	var traces []*behtree.ScenarioTrace
	if cfg.traceAll {
		for _, sr := range r.Scenarios {
			if sr.Trace != nil {
				traces = append(traces, sr.Trace)
			}
		}
	} else {
		traces = failedTraces
	}

	if len(traces) == 0 {
		return
	}

	treeJSON := ""
	if r.GeneratedTree != nil {
		if data, err := json.Marshal(r.GeneratedTree); err == nil {
			treeJSON = string(data)
		}
	}

	meta := behtree.TraceMetadata{
		CaseName:  r.Case.Name,
		Model:     cfg.model,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		TreeJSON:  treeJSON,
	}

	safeName := strings.ReplaceAll(r.Case.Name, " ", "_")
	path := filepath.Join(cfg.traceDir, safeName+".jsonl")

	f, err := os.Create(path)
	if err != nil {
		fmt.Printf("  error creating trace file: %v\n", err)
		return
	}
	defer func() { _ = f.Close() }()

	if err := behtree.WriteTraces(f, meta, traces); err != nil {
		fmt.Printf("  error writing traces: %v\n", err)
		return
	}

	fmt.Printf("  traces written to %s (%d scenarios)\n", path, len(traces))
}

func runBenchmark() {
	cfg := parseBenchFlags()
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

	if cfg.traceDir != "" || cfg.verbose {
		suite.SimulateOpts = behtree.SimulateOptions{
			TraceEnabled: true,
			CaptureState: cfg.verbose,
		}
	}

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

	generateTree := llmgen.NewGenerateTree(llm, cfg.model, cfg.verbose)

	fmt.Printf("Running %d benchmark case(s) with model %q (provider: %s)\n\n",
		len(suite.Cases), cfg.model, cfg.provider)

	results := suite.Run(generateTree)
	passed := printBenchResults(results, cfg)

	if cfg.saveTreesDir != "" {
		saveTrees(results, cfg)
	}

	fmt.Printf("Results: %d/%d passed\n", passed, len(results))

	if passed < len(results) {
		os.Exit(1)
	}
}

func saveTrees(results []*behtree.BenchmarkResult, cfg benchConfig) {
	if err := os.MkdirAll(cfg.saveTreesDir, 0o755); err != nil {
		log.Fatalf("create save-trees dir: %v", err)
	}

	for _, r := range results {
		if r.GeneratedTree == nil {
			continue
		}

		st := &behtree.SavedTree{
			CaseName:  r.Case.Name,
			Model:     cfg.model,
			Timestamp: time.Now().UTC(),
			Tree:      r.GeneratedTree,
		}

		safeName := strings.ReplaceAll(r.Case.Name, " ", "_")
		path := filepath.Join(cfg.saveTreesDir, safeName+".json")

		f, err := os.Create(path)
		if err != nil {
			log.Fatalf("create saved tree file: %v", err)
		}

		if err := behtree.WriteSavedTree(f, st); err != nil {
			_ = f.Close()
			log.Fatalf("write saved tree: %v", err)
		}
		_ = f.Close()

		fmt.Printf("Saved tree to %s\n", path)
	}
}
