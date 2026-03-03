package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/richiejp/behtree"
	"github.com/richiejp/behtree/internal/benchcases"
)

type evalConfig struct {
	verbose    bool
	traceDir   string
	traceAll   bool
	caseFilter string
}

func parseEvalFlags() (evalConfig, []string) {
	cfg := evalConfig{}
	flag.BoolVar(&cfg.verbose, "verbose", false, "Print trees and extra info")
	flag.StringVar(&cfg.traceDir, "trace-dir", "", "Directory to write trace files")
	flag.BoolVar(&cfg.traceAll, "trace-all", false, "Trace all scenarios, not just failures")
	flag.StringVar(&cfg.caseFilter, "case", "", "Evaluate only the named case")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: beht eval [flags] <dir-or-file>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	return cfg, args
}

func loadSavedTrees(paths []string) []*behtree.SavedTree {
	var trees []*behtree.SavedTree

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			log.Fatalf("stat %s: %v", p, err)
		}

		if info.IsDir() {
			matches, err := filepath.Glob(filepath.Join(p, "*.json"))
			if err != nil {
				log.Fatalf("glob %s: %v", p, err)
			}
			for _, m := range matches {
				trees = append(trees, readSavedTreeFile(m))
			}
		} else {
			trees = append(trees, readSavedTreeFile(p))
		}
	}

	return trees
}

func readSavedTreeFile(path string) *behtree.SavedTree {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	st, err := behtree.ReadSavedTree(f)
	if err != nil {
		log.Fatalf("read saved tree %s: %v", path, err)
	}

	return st
}

func buildCaseMap() map[string]*behtree.BenchmarkCase {
	robotCase, err := benchcases.RobotCase("testdata")
	if err != nil {
		log.Fatalf("Failed to load robot case: %v", err)
	}

	desktopCase, err := benchcases.DesktopCase("testdata")
	if err != nil {
		log.Fatalf("Failed to load desktop case: %v", err)
	}

	return map[string]*behtree.BenchmarkCase{
		robotCase.Name:   robotCase,
		desktopCase.Name: desktopCase,
	}
}

func runEval() {
	cfg, args := parseEvalFlags()

	savedTrees := loadSavedTrees(args)
	if len(savedTrees) == 0 {
		log.Fatal("no saved tree files found")
	}

	caseMap := buildCaseMap()

	suite := behtree.NewBenchmarkSuite()
	if cfg.traceDir != "" || cfg.verbose {
		suite.SimulateOpts = behtree.SimulateOptions{
			TraceEnabled: true,
			CaptureState: cfg.verbose,
		}
	}

	var results []*behtree.BenchmarkResult
	model := ""

	for _, st := range savedTrees {
		if cfg.caseFilter != "" && st.CaseName != cfg.caseFilter {
			continue
		}

		bc, ok := caseMap[st.CaseName]
		if !ok {
			var known []string
			for k := range caseMap {
				known = append(known, k)
			}
			log.Fatalf("unknown case %q (known: %s)", st.CaseName, strings.Join(known, ", "))
		}

		if model == "" {
			model = st.Model
		}

		results = append(results, suite.EvalTree(bc, st.Tree))
	}

	if len(results) == 0 {
		log.Fatal("no cases matched after filtering")
	}

	// Construct a benchConfig for printBenchResults reuse
	bCfg := benchConfig{
		model:    model,
		verbose:  cfg.verbose,
		traceDir: cfg.traceDir,
		traceAll: cfg.traceAll,
	}

	fmt.Printf("Evaluating %d saved tree(s) (model: %s)\n\n", len(results), model)
	passed := printBenchResults(results, bCfg)
	fmt.Printf("Results: %d/%d passed\n", passed, len(results))

	if passed < len(results) {
		os.Exit(1)
	}
}
