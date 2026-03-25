package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mudler/cogito/clients"
	"github.com/richiejp/behtree"
	"github.com/richiejp/behtree/internal/galcheck"
)

func runGalleryCheck() {
	flags := flag.NewFlagSet("gallery-check", flag.ExitOnError)
	galleryPath := flags.String("gallery-path", "", "Path to gallery index.yaml (required)")
	envPath := flags.String("env", "testdata/gallery_check.json", "Path to environment JSON")
	actionsPath := flags.String("actions", "testdata/gallery_check_actions.json", "Path to actions JSON")
	output := flags.String("output", "", "Output summary file (default: stdout)")
	outputDir := flags.String("output-dir", "", "Directory for per-model report files (enables resume)")
	apply := flags.Bool("apply", false, "Apply approved reports from output-dir to gallery YAML")
	limit := flags.Int("limit", 0, "Max models to check (0 = unlimited)")
	maxAgeDays := flags.Int("max-age", 90, "Max days since last check before re-checking")
	dryRun := flags.Bool("dry-run", false, "Scan only, don't fetch or verify")
	verbose := flags.Bool("verbose", false, "Print detailed progress")
	maxTicks := flags.Int("max-ticks", 1000, "Max interpreter ticks before stopping")
	maxDelay := flags.Int("max-delay", 7, "Max backoff delay in seconds on repeated failures")
	timeoutSecs := flags.Int("timeout", 30, "Timeout in seconds for LLM and HF API calls")

	// LLM flags for tag/description synthesis
	model := flags.String("model", "", "LLM model name for metadata synthesis (optional)")
	apiKey := flags.String("api-key", "", "LLM API key (env: API_KEY)")
	baseURL := flags.String("base-url", "", "LLM API base URL (env: BASE_URL)")
	provider := flags.String("provider", "openai", "LLM provider: openai or localai")

	flags.Usage = func() { printGalleryCheckUsage(flags) }

	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(1)
	}

	if *galleryPath == "" && !*apply {
		printGalleryCheckUsage(flags)
		os.Exit(1)
	}

	// Apply mode: skip BT setup, just apply reports to gallery
	if *apply {
		runGalleryApply(*galleryPath, *outputDir, *verbose)
		return
	}

	// Resolve env vars for LLM config
	if *apiKey == "" {
		*apiKey = os.Getenv("API_KEY")
	}
	if *baseURL == "" {
		*baseURL = os.Getenv("BASE_URL")
	}

	// Load environment
	env, err := behtree.LoadEnvironment(*envPath)
	if err != nil {
		log.Fatalf("load environment: %v", err)
	}

	if len(env.Goal) == 0 {
		log.Fatal("environment has no goal conditions")
	}

	goal, err := behtree.ResolveGoal(env.Goal)
	if err != nil {
		log.Fatalf("resolve goal: %v", err)
	}

	// Load action selections
	data, err := os.ReadFile(*actionsPath)
	if err != nil {
		log.Fatalf("read actions: %v", err)
	}
	var selections []behtree.ActionSelection
	if err := json.Unmarshal(data, &selections); err != nil {
		log.Fatalf("parse actions: %v", err)
	}

	grounded, err := behtree.GroundActions(env, selections)
	if err != nil {
		log.Fatalf("ground actions: %v", err)
	}

	// Build tree
	state := behtree.NewStateFromEnvironment(env)
	result, err := behtree.BuildTree(goal, grounded, state)
	if err != nil {
		log.Fatalf("build tree: %v", err)
	}

	if *verbose {
		fmt.Fprintln(os.Stderr, "Generated tree:")
		behtree.Print(result.Tree, os.Stderr)
		fmt.Fprintln(os.Stderr)
	}

	// Set up runtime state
	runtimeState := behtree.NewStateFromEnvironment(env)

	// Set up registry: merge PA-BT auto-registered condition handlers with action handlers
	registry := behtree.NewActionRegistry()
	registry.Merge(result.Registry)

	timeout := time.Duration(*timeoutSecs) * time.Second
	cfg := &galcheck.Config{
		GalleryPath: *galleryPath,
		OutputDir:   *outputDir,
		HF:          galcheck.NewHFClient(timeout),
		MaxAge:      time.Duration(*maxAgeDays) * 24 * time.Hour,
		Timeout:     timeout,
		Limit:       *limit,
		DryRun:      *dryRun,
		Verbose:     *verbose,
	}

	// Set up LLM if model is specified
	if *model != "" {
		switch *provider {
		case "openai":
			cfg.LLM = clients.NewOpenAILLM(*model, *apiKey, *baseURL)
		case "localai":
			cfg.LLM = clients.NewLocalAILLM(*model, *apiKey, *baseURL)
		default:
			log.Fatalf("unknown provider %q (use openai or localai)", *provider)
		}
		cfg.LLMModel = *model
		if *verbose {
			log.Printf("LLM enabled: provider=%s model=%s", *provider, *model)
		}
	}

	galcheck.RegisterHandlers(registry, cfg)

	// Resume from output directory if it exists
	if *outputDir != "" {
		if err := os.MkdirAll(*outputDir, 0o755); err != nil {
			log.Fatalf("create output dir: %v", err)
		}
		if err := galcheck.ResumeFromDir(cfg); err != nil {
			log.Fatalf("resume: %v", err)
		}
	}

	// Create interpreter and run tick loop
	interp := behtree.NewInterpreter(env, registry, runtimeState)

	if *verbose {
		log.Printf("Starting gallery check: gallery=%s, limit=%d, max-age=%dd, dry-run=%v",
			*galleryPath, *limit, *maxAgeDays, *dryRun)
	}

	var consecutiveFailures uint
	for tick := 0; tick < *maxTicks; tick++ {
		prevChecked := len(cfg.Reports)

		status, tickErr := interp.Tick(result.Tree)
		if tickErr != nil {
			log.Fatalf("tick %d: %v", tick, tickErr)
		}

		if *verbose {
			log.Printf("tick %d: %s (models checked: %d)", tick, status, len(cfg.Reports))
		}

		if status == behtree.Success {
			consecutiveFailures = 0
			if len(cfg.Reports) > prevChecked {
				// A model was processed this tick; there might be more.
				// Continue ticking so ScanGallery can find the next one.
				continue
			}
			// No model was processed — truly idle.
			break
		}

		if status == behtree.Failure {
			consecutiveFailures++
			delay := backoff(consecutiveFailures, time.Duration(*maxDelay)*time.Second)
			if *verbose {
				log.Printf("tick %d: failure, retrying in %v...", tick, delay)
			}
			time.Sleep(delay)
			continue
		}

		// Running: async action in progress, continue ticking
		consecutiveFailures = 0
	}

	// Write summary report
	var w *os.File
	if *output != "" {
		w, err = os.Create(*output)
		if err != nil {
			log.Fatalf("create output: %v", err)
		}
		defer w.Close()
	} else {
		w = os.Stdout
	}

	if len(cfg.Reports) > 0 {
		galcheck.WriteSummary(w, cfg.Reports)
		for _, report := range cfg.Reports {
			galcheck.WriteReport(w, report)
		}
	} else {
		fmt.Fprintln(w, "No models needed checking.")
	}

	if *verbose {
		log.Printf("Done: %d models checked", len(cfg.Reports))
	}
}

func runGalleryApply(galleryPath, outputDir string, verbose bool) {
	if outputDir == "" {
		log.Fatal("apply mode requires -output-dir")
	}

	reports, err := galcheck.LoadReports(outputDir)
	if err != nil {
		log.Fatalf("load reports: %v", err)
	}

	if len(reports) == 0 {
		fmt.Println("No reports found in", outputDir)
		return
	}

	applied, err := galcheck.ApplyReports(galleryPath, reports)
	if err != nil {
		log.Fatalf("apply reports: %v", err)
	}

	if verbose {
		for _, r := range reports {
			if r.ProposedEntry != nil {
				log.Printf("Applied: %s (%d findings)", r.Name, len(r.Findings))
			}
		}
	}

	fmt.Printf("Applied %d of %d reports to %s\n", applied, len(reports), galleryPath)
}

func printGalleryCheckUsage(flags *flag.FlagSet) {
	fmt.Fprintln(os.Stderr, "Usage: beht gallery-check -gallery-path <index.yaml> [flags]")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Checks gallery model metadata against HuggingFace, generates per-model reports.")
	fmt.Fprintln(os.Stderr, "Supports stop/resume via -output-dir and applying approved changes via -apply.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintln(os.Stderr, "  # Check models (resumable)")
	fmt.Fprintln(os.Stderr, "  beht gallery-check -gallery-path index.yaml -output-dir ./results -verbose")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "  # With LLM for tag/description synthesis")
	fmt.Fprintln(os.Stderr, "  beht gallery-check -gallery-path index.yaml -output-dir ./results \\")
	fmt.Fprintln(os.Stderr, "    -model Qwen3.5-9B-GGUF -base-url http://localhost:8080 -provider localai")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "  # Apply approved reports back to gallery")
	fmt.Fprintln(os.Stderr, "  beht gallery-check -gallery-path index.yaml -output-dir ./results -apply")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Flags:")
	flags.PrintDefaults()
}

// backoff returns an exponential backoff delay: 1s, 2s, 4s, ... capped at maxDelay.
func backoff(consecutiveFailures uint, maxDelay time.Duration) time.Duration {
	shift := min(consecutiveFailures-1, 10)
	d := time.Duration(1<<shift) * time.Second
	return min(d, maxDelay)
}
