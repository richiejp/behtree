package benchcases

import "github.com/richiejp/behtree"

// RegisterRobotHandlers registers the 5 robot scenario handlers
// (IsHolding, IsAt, NavigateTo, PickUp, DropIn) on the given registry.
func RegisterRobotHandlers(registry *behtree.BehaviourRegistry) {
	registry.Register("IsHolding", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		obj := params["object"].(string)
		holding, _ := s.Get("robot", "holding")
		if holding == obj {
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		}
		return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
	})

	registry.Register("IsAt", func(params behtree.Params, s *behtree.State, req behtree.OutcomeRequest) behtree.HandlerResult {
		loc := params["location"].(string)
		robotLoc, _ := s.Get("robot", "location")
		if robotLoc == loc {
			return behtree.HandlerResult{Status: behtree.Success, Compatible: true}
		}
		return behtree.HandlerResult{Status: behtree.Failure, Compatible: true}
	})

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
