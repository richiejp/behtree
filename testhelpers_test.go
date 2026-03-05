package behtree_test

import "github.com/richiejp/behtree"

// robotTestState creates the initial state for the robot scenario.
func robotTestState() *behtree.State {
	state := behtree.NewState()
	state.Objects["robot"] = map[string]any{"location": "start", "holding": ""}
	state.Objects["wrapper"] = map[string]any{"location": "table"}
	state.Objects["table"] = map[string]any{"type": "surface"}
	state.Objects["bin"] = map[string]any{"type": "container"}
	return state
}

// robotTestActions returns the grounded actions for the robot scenario.
func robotTestActions() []behtree.GroundedAction {
	return []behtree.GroundedAction{
		{
			Name: "NavigateTo", Params: behtree.Params{"location": "table"}, Async: true,
			Postconditions: []behtree.ResolvedCondition{
				{Object: "robot", Field: "location", Value: "table"},
			},
		},
		{
			Name: "PickUp", Params: behtree.Params{"object": "wrapper"},
			Preconditions: []behtree.ResolvedCondition{
				{Object: "robot", Field: "location", ValueRef: &behtree.StateRef{Object: "wrapper", Field: "location"}},
				{Object: "robot", Field: "holding", Value: ""},
			},
			Postconditions: []behtree.ResolvedCondition{
				{Object: "robot", Field: "holding", Value: "wrapper"},
			},
		},
		{
			Name: "NavigateTo", Params: behtree.Params{"location": "bin"}, Async: true,
			Postconditions: []behtree.ResolvedCondition{
				{Object: "robot", Field: "location", Value: "bin"},
			},
		},
		{
			Name: "DropIn", Params: behtree.Params{"object": "wrapper", "container": "bin"},
			Preconditions: []behtree.ResolvedCondition{
				{Object: "robot", Field: "holding", Value: "wrapper"},
				{Object: "robot", Field: "location", Value: "bin"},
			},
			Postconditions: []behtree.ResolvedCondition{
				{Object: "robot", Field: "holding", Value: ""},
				{Object: "wrapper", Field: "location", Value: "bin"},
			},
		},
	}
}

// robotTestGoal returns the goal conditions for the robot scenario.
func robotTestGoal() []behtree.ResolvedCondition {
	return []behtree.ResolvedCondition{
		{Object: "wrapper", Field: "location", Value: "bin"},
	}
}

const desktopTestURL = "https://github.com/mudler/LocalAI"

// desktopTestState creates the initial state for the desktop scenario.
func desktopTestState() *behtree.State {
	state := behtree.NewState()
	state.Objects["sway_state"] = map[string]any{"refreshed": "false"}
	state.Objects["firefox"] = map[string]any{"open": "false", "active_page": ""}
	state.Objects["utterance"] = map[string]any{"text": "", "processed": "false"}
	state.Objects["task_tree"] = map[string]any{"tree": nil}
	return state
}

// desktopTestActions returns the grounded actions for the desktop inner tree.
func desktopTestActions() []behtree.GroundedAction {
	return []behtree.GroundedAction{
		{
			Name: "QuerySwayTree", Params: behtree.Params{},
			Postconditions: []behtree.ResolvedCondition{
				{Object: "sway_state", Field: "refreshed", Value: "true"},
			},
		},
		{
			Name: "OpenApp", Params: behtree.Params{"app": "firefox"},
			Preconditions: []behtree.ResolvedCondition{
				{Object: "sway_state", Field: "refreshed", Value: "true"},
			},
			Postconditions: []behtree.ResolvedCondition{
				{Object: "firefox", Field: "open", Value: "true"},
			},
		},
		{
			Name: "OpenURL", Params: behtree.Params{"app": "firefox", "url": desktopTestURL},
			Preconditions: []behtree.ResolvedCondition{
				{Object: "firefox", Field: "open", Value: "true"},
			},
			Postconditions: []behtree.ResolvedCondition{
				{Object: "firefox", Field: "active_page", Value: desktopTestURL},
			},
		},
	}
}

// desktopTestGoal returns the goal conditions for the desktop scenario.
func desktopTestGoal() []behtree.ResolvedCondition {
	return []behtree.ResolvedCondition{
		{Object: "firefox", Field: "active_page", Value: desktopTestURL},
	}
}
