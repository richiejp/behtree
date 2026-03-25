package benchcases

import "github.com/richiejp/behtree"

// RegisterRobotHandlers registers the robot scenario action handlers
// (NavigateTo, PickUp, DropIn) on the given registry.
// Condition handlers (IsHolding, IsAt) are no longer needed since PA-BT
// auto-generates condition nodes with their own handlers.
func RegisterRobotHandlers(registry *behtree.ActionRegistry) {
	RegisterObserveHandler(registry)

	registry.Register("NavigateTo", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		loc := params["location"].(string)
		robotLoc, _ := s.Get("robot", "location")

		if req == behtree.RequestRunning {
			return behtree.HandlerResult{Status: behtree.Running, Compatible: true}
		}
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}

		if robotLoc == loc {
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		}
		_ = s.Set("robot", "location", loc)
		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	})

	registry.Register("PickUp", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		obj := params["object"].(string)
		robotLoc, _ := s.Get("robot", "location")
		objLoc, _ := s.Get(obj, "location")
		if robotLoc != objLoc {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: req == behtree.RequestFailure}
		}
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}
		_ = s.Set("robot", "holding", obj)
		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	})

	registry.Register("DropIn", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		obj := params["object"].(string)
		container := params["container"].(string)
		holding, _ := s.Get("robot", "holding")
		robotLoc, _ := s.Get("robot", "location")
		if holding != obj || robotLoc != container {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: req == behtree.RequestFailure}
		}
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}
		_ = s.Set("robot", "holding", "")
		_ = s.Set(obj, "location", container)
		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	})
}

// RegisterObserveHandler registers the generic Observe action handler.
// In a real implementation, the handler would dispatch to target-specific
// observers (screenshot, sway tree, etc.) and update the target's fields
// from observed reality. The handler can internally cache or use cheaper
// heuristics.
func RegisterObserveHandler(registry *behtree.ActionRegistry) {
	registry.Register("Observe", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		target := params["target"].(string)
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}
		_ = s.Set(target, "observed", "true")
		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	})
}

// RegisterDesktopInnerHandlers registers handlers for the desktop inner tree
// actions (Observe, QuerySwayTree, OpenApp, OpenURL). PA-BT auto-generates
// condition node handlers, so only action handlers are needed.
func RegisterDesktopInnerHandlers(registry *behtree.ActionRegistry) {
	RegisterObserveHandler(registry)
	registry.Register("QuerySwayTree", func(_ behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}
		_ = s.Set("sway_state", "refreshed", "true")
		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	})

	registry.Register("OpenApp", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		app := params["app"].(string)
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}
		_ = s.Set(app, "open", "true")
		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	})

	registry.Register("OpenURL", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		app := params["app"].(string)
		url := params["url"].(string)
		open, _ := s.Get(app, "open")
		if open != "true" {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: req == behtree.RequestFailure}
		}
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}
		_ = s.Set(app, "active_page", url)
		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	})
}

// RegisterDesktopOuterHandlers registers handlers for the desktop outer tree
// actions (HandleSpeech, Idle). RunTaskTree is handled by the interpreter
// directly (tickRunTaskTree), so it does not need a registry handler.
func RegisterDesktopOuterHandlers(registry *behtree.ActionRegistry) {
	RegisterObserveHandler(registry)

	registry.Register("HandleSpeech", func(_ behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}
		// In a real implementation, this would wait for speech to finish
		// and process the utterance (possibly creating a task tree).
		_ = s.Set("speech", "active", "false")
		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	})

	registry.Register("Idle", func(_ behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		if req == behtree.RequestFailure {
			return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
		}
		_ = s.Set("system", "idle", "true")
		return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
	})
}
