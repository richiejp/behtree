package behtree

import "fmt"

type ScenarioResult struct {
	Requests []OutcomeRequest
	Ticks    []TickResult
	Skipped  bool
	SkipMsg  string
	Final    *State
}

type TickResult struct {
	Status Status
	Err    error
}

type SimulationHarness struct {
	env      *Environment
	registry *BehaviourRegistry
	tree     *Node
}

func NewSimulationHarness(env *Environment, registry *BehaviourRegistry, tree *Node) *SimulationHarness {
	return &SimulationHarness{
		env:      env,
		registry: registry,
		tree:     tree,
	}
}

func (h *SimulationHarness) RunScenario(requests []OutcomeRequest, initialState *State, maxTicks int) *ScenarioResult {
	result := &ScenarioResult{
		Requests: requests,
	}

	state := initialState.Clone()
	ip := NewInterpreter(h.env, h.registry, state)

	requestIdx := 0
	for tick := 0; tick < maxTicks; tick++ {
		if requestIdx < len(requests) {
			ip.SetOutcomeRequest(requests[requestIdx])
		} else {
			ip.SetOutcomeRequest(RequestSuccess)
		}

		status, err := ip.Tick(h.tree)
		tr := TickResult{Status: status, Err: err}
		result.Ticks = append(result.Ticks, tr)

		if err != nil {
			if isIncompatible(err) {
				result.Skipped = true
				result.SkipMsg = err.Error()
				result.Final = state
				return result
			}
		}

		if requestIdx < len(requests) {
			requestIdx++
		}

		if status == Success || status == Failure {
			result.Final = state
			return result
		}
	}

	result.Final = state
	return result
}

func (h *SimulationHarness) RunAllOutcomes(maxTicks int) []*ScenarioResult {
	leafCount := countLeaves(h.tree)
	outcomes := []OutcomeRequest{RequestSuccess, RequestFailure, RequestRunning}

	var results []*ScenarioResult
	generateCombinations(outcomes, leafCount, func(combo []OutcomeRequest) {
		state := NewStateFromEnvironment(h.env)
		result := h.RunScenario(combo, state, maxTicks)
		results = append(results, result)
	})

	return results
}

func countLeaves(n *Node) int {
	if n == nil {
		return 0
	}
	if n.Type.IsLeaf() {
		return 1
	}
	count := 0
	for _, child := range n.Children {
		count += countLeaves(child)
	}
	return count
}

func generateCombinations(options []OutcomeRequest, length int, fn func([]OutcomeRequest)) {
	if length == 0 {
		fn(nil)
		return
	}

	combo := make([]OutcomeRequest, length)
	var generate func(pos int)
	generate = func(pos int) {
		if pos == length {
			cp := make([]OutcomeRequest, length)
			copy(cp, combo)
			fn(cp)
			return
		}
		for _, opt := range options {
			combo[pos] = opt
			generate(pos + 1)
		}
	}
	generate(0)
}

func isIncompatible(err error) bool {
	if err == nil {
		return false
	}
	return fmt.Sprintf("%v", err) != "" && containsIncompatible(err.Error())
}

func containsIncompatible(s string) bool {
	return len(s) > 20 && s[len(s)-len("incompatible with request"):] != "" &&
		false ||
		stringContains(s, "incompatible")
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
