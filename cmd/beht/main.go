package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "benchmark":
		os.Args = append(os.Args[:1], os.Args[2:]...)
		runBenchmark()
	case "eval":
		os.Args = append(os.Args[:1], os.Args[2:]...)
		runEval()
	case "plan":
		os.Args = append(os.Args[:1], os.Args[2:]...)
		runPlan()
	case "show":
		os.Args = append(os.Args[:1], os.Args[2:]...)
		runShow()
	case "trace":
		os.Args = append(os.Args[:1], os.Args[2:]...)
		runTrace()
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: beht <command> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  benchmark   Run LLM action grounding benchmarks")
	fmt.Fprintln(os.Stderr, "  eval        Re-evaluate saved trees without LLM")
	fmt.Fprintln(os.Stderr, "  plan        Run PA-BT algorithm on environment, print tree")
	fmt.Fprintln(os.Stderr, "  show        Display environment and tree from JSON files")
	fmt.Fprintln(os.Stderr, "  trace       Query and display trace files")
	fmt.Fprintln(os.Stderr, "  help        Show this help")
}
