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
	output := flags.String("output", "", "Output report file (default: stdout)")
	limit := flags.Int("limit", 0, "Max models to check (0 = unlimited)")
	maxAgeDays := flags.Int("max-age", 90, "Max days since last check before re-checking")
	dryRun := flags.Bool("dry-run", false, "Scan only, don't fetch or verify")
	verbose := flags.Bool("verbose", false, "Print detailed progress")
	maxTicks := flags.Int("max-ticks", 1000, "Max interpreter ticks before stopping")

	// LLM flags for tag/description synthesis
	model := flags.String("model", "", "LLM model name for metadata synthesis (optional)")
	apiKey := flags.String("api-key", "", "LLM API key (env: API_KEY)")
	baseURL := flags.String("base-url", "", "LLM API base URL (env: BASE_URL)")
	provider := flags.String("provider", "openai", "LLM provider: openai or localai")

	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(1)
	}

	if *galleryPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: beht gallery-check -gallery-path <index.yaml> [flags]")
		fmt.Fprintln(os.Stderr, "  Checks gallery model metadata against HuggingFace, generates a report.")
		fmt.Fprintln(os.Stderr, "  Add -model <name> to enable LLM-assisted tag/description suggestions.")
		fmt.Fprintln(os.Stderr)
		flags.PrintDefaults()
		os.Exit(1)
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

	cfg := &galcheck.Config{
		GalleryPath: *galleryPath,
		HF:          galcheck.NewHFClient(),
		MaxAge:      time.Duration(*maxAgeDays) * 24 * time.Hour,
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

	// Create interpreter and run tick loop
	interp := behtree.NewInterpreter(env, registry, runtimeState)

	if *verbose {
		log.Printf("Starting gallery check: gallery=%s, limit=%d, max-age=%dd, dry-run=%v",
			*galleryPath, *limit, *maxAgeDays, *dryRun)
	}

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
			if len(cfg.Reports) > prevChecked {
				// A model was processed this tick; there might be more.
				// Continue ticking so ScanGallery can find the next one.
				continue
			}
			// No model was processed — truly idle.
			break
		}

		if status == behtree.Failure {
			if *verbose {
				log.Printf("tick %d: failure, retrying...", tick)
			}
			continue
		}

		// Running: async action in progress, continue ticking
	}

	// Write report
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
