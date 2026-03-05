package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/richiejp/behtree"
)

func runPlan() {
	flags := flag.NewFlagSet("plan", flag.ExitOnError)
	actionsJSON := flags.String("actions", "", "JSON file with action selections (optional, grounds all actions if omitted)")

	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(1)
	}

	paths := flags.Args()
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: beht plan [flags] <environment.json>")
		fmt.Fprintln(os.Stderr, "  Runs PA-BT algorithm on the environment's goal and actions, prints resulting tree.")
		flags.PrintDefaults()
		os.Exit(1)
	}

	env, err := behtree.LoadEnvironment(paths...)
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

	var grounded []behtree.GroundedAction

	if *actionsJSON != "" {
		data, readErr := os.ReadFile(*actionsJSON)
		if readErr != nil {
			log.Fatalf("read actions file: %v", readErr)
		}
		var selections []behtree.ActionSelection
		if jsonErr := json.Unmarshal(data, &selections); jsonErr != nil {
			log.Fatalf("parse actions file: %v", jsonErr)
		}
		grounded, err = behtree.GroundActions(env, selections)
		if err != nil {
			log.Fatalf("ground actions: %v", err)
		}
	} else {
		grounded, err = groundAllActions(env)
		if err != nil {
			log.Fatalf("ground all actions: %v", err)
		}
	}

	state := behtree.NewStateFromEnvironment(env)
	result, err := behtree.BuildTree(goal, grounded, state)
	if err != nil {
		log.Fatalf("build tree: %v", err)
	}

	fmt.Println("Goal:")
	for _, g := range goal {
		fmt.Printf("  %s\n", g.Name())
	}
	fmt.Println()

	fmt.Println("Grounded actions:")
	for _, a := range grounded {
		fmt.Printf("  %s(%v)\n", a.Name, a.Params)
	}
	fmt.Println()

	fmt.Println("Generated tree:")
	behtree.Print(result.Tree, os.Stdout)
}

// groundAllActions generates all possible groundings for each action
// using every valid object combination for object_ref params.
func groundAllActions(env *behtree.Environment) ([]behtree.GroundedAction, error) {
	var result []behtree.GroundedAction

	for i := range env.Actions {
		action := &env.Actions[i]
		paramNames := make([]string, 0, len(action.Params))
		for name := range action.Params {
			paramNames = append(paramNames, name)
		}

		if len(paramNames) == 0 {
			ga, err := behtree.GroundAction(action, behtree.Params{})
			if err != nil {
				return nil, err
			}
			result = append(result, *ga)
			continue
		}

		// Generate all object combinations for object_ref params
		combos := generateParamCombos(env, action, paramNames, 0, behtree.Params{})
		for _, params := range combos {
			ga, err := behtree.GroundAction(action, params)
			if err != nil {
				return nil, err
			}
			result = append(result, *ga)
		}
	}

	return result, nil
}

func generateParamCombos(env *behtree.Environment, action *behtree.ActionDef, paramNames []string, idx int, current behtree.Params) []behtree.Params {
	if idx >= len(paramNames) {
		cp := make(behtree.Params, len(current))
		for k, v := range current {
			cp[k] = v
		}
		return []behtree.Params{cp}
	}

	name := paramNames[idx]
	ptype := action.Params[name]

	var values []string
	if ptype == behtree.ParamObjectRef {
		for _, obj := range env.Objects {
			values = append(values, obj.Name)
		}
	} else {
		values = []string{""}
	}

	var results []behtree.Params
	for _, v := range values {
		current[name] = v
		results = append(results, generateParamCombos(env, action, paramNames, idx+1, current)...)
	}

	return results
}
