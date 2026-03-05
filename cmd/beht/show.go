package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/richiejp/behtree"
)

func runShow() {
	flags := flag.NewFlagSet("show", flag.ExitOnError)
	envOnly := flags.Bool("env", false, "Show only the environment (objects, interfaces, behaviours)")
	treeOnly := flags.Bool("tree", false, "Show only the tree")

	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(1)
	}

	paths := flags.Args()
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: beht show [flags] <file.json> [file.json ...]")
		os.Exit(1)
	}

	env, err := behtree.LoadEnvironment(paths...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading environment: %v\n", err)
		os.Exit(1)
	}

	showEnv := !*treeOnly
	showTree := !*envOnly

	if showEnv {
		behtree.PrintEnvironment(env, os.Stdout)
	}

	if showTree && len(env.Trees) > 0 {
		for i, tree := range env.Trees {
			if showEnv || i > 0 {
				fmt.Println()
			}
			if len(env.Trees) > 1 {
				fmt.Printf("Tree %d:\n", i+1)
			} else if showEnv {
				fmt.Println("Tree:")
			}
			behtree.Print(tree, os.Stdout)
		}
	}
}
