package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/richiejp/behtree"
)

type traceConfig struct {
	scenario int
	failures bool
	summary  bool
	state    bool
}

func parseTraceFlags() (traceConfig, string) {
	cfg := traceConfig{}
	flag.IntVar(&cfg.scenario, "scenario", -1, "Show specific scenario by index")
	flag.BoolVar(&cfg.failures, "failures", true, "Show only failed scenarios")
	flag.BoolVar(&cfg.summary, "summary", false, "Show aggregate summary only")
	flag.BoolVar(&cfg.state, "state", false, "Include state in output")
	flag.Parse()

	args := flag.Args()
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: beht trace [flags] <trace-file.jsonl>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	return cfg, args[0]
}

func runTrace() {
	cfg, filePath := parseTraceFlags()

	f, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("open trace file: %v", err)
	}
	defer func() { _ = f.Close() }()

	meta, err := behtree.ReadTraceMetadata(f)
	if err != nil {
		log.Fatalf("read metadata: %v", err)
	}

	fmt.Printf("Case: %s\n", meta.CaseName)
	if meta.Model != "" {
		fmt.Printf("Model: %s\n", meta.Model)
	}
	fmt.Printf("Timestamp: %s\n", meta.Timestamp)
	fmt.Println()

	// Reopen to read from the start (metadata was consumed)
	_ = f.Close()
	f, err = os.Open(filePath)
	if err != nil {
		log.Fatalf("reopen trace file: %v", err)
	}
	defer func() { _ = f.Close() }()

	if cfg.summary {
		printTraceSummary(f)
		return
	}

	if cfg.scenario >= 0 {
		printSingleScenario(f, cfg)
		return
	}

	printFilteredScenarios(f, cfg)
}

func printTraceSummary(f *os.File) {
	total := 0
	failed := 0
	skipped := 0

	err := behtree.ReadScenarioTraces(f, func(_ int, trace *behtree.ScenarioTrace) bool {
		total++
		if trace.Skipped {
			skipped++
		} else if trace.Failed {
			failed++
		}
		return true
	})
	if err != nil {
		log.Fatalf("read traces: %v", err)
	}

	passed := total - failed - skipped
	fmt.Printf("Total:   %d\n", total)
	fmt.Printf("Passed:  %d\n", passed)
	fmt.Printf("Failed:  %d\n", failed)
	fmt.Printf("Skipped: %d\n", skipped)
}

func printSingleScenario(f *os.File, cfg traceConfig) {
	found := false
	err := behtree.ReadScenarioTraces(f, func(idx int, trace *behtree.ScenarioTrace) bool {
		if idx == cfg.scenario {
			found = true
			fmt.Printf("Scenario %d:\n", idx)
			printTrace(trace, cfg)
			return false
		}
		return true
	})
	if err != nil {
		log.Fatalf("read traces: %v", err)
	}
	if !found {
		log.Fatalf("scenario %d not found", cfg.scenario)
	}
}

func printFilteredScenarios(f *os.File, cfg traceConfig) {
	err := behtree.ReadScenarioTraces(f, func(idx int, trace *behtree.ScenarioTrace) bool {
		if cfg.failures && !trace.Failed {
			return true
		}
		fmt.Printf("--- Scenario %d ---\n", idx)
		printTrace(trace, cfg)
		fmt.Println()
		return true
	})
	if err != nil {
		log.Fatalf("read traces: %v", err)
	}
}

func printTrace(trace *behtree.ScenarioTrace, cfg traceConfig) {
	var sb strings.Builder
	behtree.PrintScenarioTrace(trace, &sb)
	output := sb.String()

	if !cfg.state {
		// Strip final state section
		if idx := strings.Index(output, "Final State:"); idx >= 0 {
			output = output[:idx]
		}
	}

	fmt.Print(output)
}
