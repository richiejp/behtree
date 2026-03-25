package behtree_test

import "github.com/richiejp/behtree"

// robotTestState creates the initial state for the robot scenario.
func robotTestState() *behtree.State {
	state := behtree.NewState()
	state.EphemeralFields = []string{"observed"}
	state.Objects["robot"] = map[string]any{"location": "start", "holding": "", "observed": "false"}
	state.Objects["wrapper"] = map[string]any{"location": "table", "observed": "false"}
	state.Objects["table"] = map[string]any{"type": "surface", "location": "table"}
	state.Objects["bin"] = map[string]any{"type": "container", "location": "corner", "observed": "false"}
	return state
}

// robotTestActions returns the grounded actions for the robot scenario.
func robotTestActions() []behtree.GroundedAction {
	return []behtree.GroundedAction{
		{
			Name: "Observe", Params: behtree.Params{"target": "wrapper"},
			Postconditions: []behtree.ResolvedCondition{
				{Object: "wrapper", Field: "observed", Value: "true"},
			},
		},
		{
			Name: "Observe", Params: behtree.Params{"target": "robot"},
			Postconditions: []behtree.ResolvedCondition{
				{Object: "robot", Field: "observed", Value: "true"},
			},
		},
		{
			Name: "Observe", Params: behtree.Params{"target": "bin"},
			Postconditions: []behtree.ResolvedCondition{
				{Object: "bin", Field: "observed", Value: "true"},
			},
		},
		{
			Name: "NavigateTo", Params: behtree.Params{"location": "table"}, Async: true,
			Postconditions: []behtree.ResolvedCondition{
				{Object: "robot", Field: "location", ValueRef: &behtree.StateRef{Object: "table", Field: "location"}},
			},
		},
		{
			Name: "PickUp", Params: behtree.Params{"object": "wrapper"},
			Preconditions: []behtree.ResolvedCondition{
				{Object: "robot", Field: "location", ValueRef: &behtree.StateRef{Object: "wrapper", Field: "location"}},
				{Object: "robot", Field: "holding", Value: ""},
				{Object: "wrapper", Field: "observed", Value: "true"},
				{Object: "robot", Field: "observed", Value: "true"},
			},
			Postconditions: []behtree.ResolvedCondition{
				{Object: "robot", Field: "holding", Value: "wrapper"},
			},
		},
		{
			Name: "NavigateTo", Params: behtree.Params{"location": "bin"}, Async: true,
			Postconditions: []behtree.ResolvedCondition{
				{Object: "robot", Field: "location", ValueRef: &behtree.StateRef{Object: "bin", Field: "location"}},
			},
		},
		{
			Name: "DropIn", Params: behtree.Params{"object": "wrapper", "container": "bin"},
			Preconditions: []behtree.ResolvedCondition{
				{Object: "robot", Field: "holding", Value: "wrapper"},
				{Object: "robot", Field: "location", ValueRef: &behtree.StateRef{Object: "bin", Field: "location"}},
				{Object: "bin", Field: "observed", Value: "true"},
				{Object: "robot", Field: "observed", Value: "true"},
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
	state.EphemeralFields = []string{"observed"}
	state.Objects["sway_state"] = map[string]any{"refreshed": "false"}
	state.Objects["firefox"] = map[string]any{"open": "false", "active_page": "", "observed": "false"}
	state.Objects["utterance"] = map[string]any{"text": "", "processed": "false"}
	state.Objects["task_tree"] = map[string]any{"tree": nil}
	return state
}

// desktopTestActions returns the grounded actions for the desktop inner tree.
func desktopTestActions() []behtree.GroundedAction {
	return []behtree.GroundedAction{
		{
			Name: "Observe", Params: behtree.Params{"target": "firefox"},
			Postconditions: []behtree.ResolvedCondition{
				{Object: "firefox", Field: "observed", Value: "true"},
			},
		},
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
				{Object: "firefox", Field: "observed", Value: "true"},
			},
			Postconditions: []behtree.ResolvedCondition{
				{Object: "firefox", Field: "open", Value: "true"},
			},
		},
		{
			Name: "OpenURL", Params: behtree.Params{"app": "firefox", "url": desktopTestURL},
			Preconditions: []behtree.ResolvedCondition{
				{Object: "firefox", Field: "open", Value: "true"},
				{Object: "firefox", Field: "observed", Value: "true"},
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
		{Object: "firefox", Field: "observed", Value: "true"},
		{Object: "firefox", Field: "active_page", Value: desktopTestURL},
	}
}

// desktopOuterTestState creates the initial state for the desktop outer tree.
// Uses worst-case initial state (speech active, task pending) so PA-BT builds
// fallback structures for all conditions.
func desktopOuterTestState() *behtree.State {
	state := behtree.NewState()
	state.EphemeralFields = []string{"observed", "idle"}
	state.Objects["speech"] = map[string]any{"active": "true", "observed": "false"}
	state.Objects["task_tree"] = map[string]any{"pending": "true", "tree": nil}
	state.Objects["system"] = map[string]any{"idle": "false"}
	return state
}

// desktopOuterTestActions returns the grounded actions for the desktop outer tree.
func desktopOuterTestActions() []behtree.GroundedAction {
	return []behtree.GroundedAction{
		{
			Name: "Observe", Params: behtree.Params{"target": "speech"},
			Postconditions: []behtree.ResolvedCondition{
				{Object: "speech", Field: "observed", Value: "true"},
			},
		},
		{
			Name: "HandleSpeech", Params: behtree.Params{}, Async: true,
			Preconditions: []behtree.ResolvedCondition{
				{Object: "speech", Field: "observed", Value: "true"},
			},
			Postconditions: []behtree.ResolvedCondition{
				{Object: "speech", Field: "active", Value: "false"},
			},
		},
		{
			Name: "RunTaskTree", Params: behtree.Params{"tree_variable": "task_tree.tree"}, Async: true,
			Preconditions: []behtree.ResolvedCondition{
				{Object: "speech", Field: "active", Value: "false"},
				{Object: "speech", Field: "observed", Value: "true"},
			},
			Postconditions: []behtree.ResolvedCondition{
				{Object: "task_tree", Field: "pending", Value: "false"},
			},
		},
		{
			Name: "Idle", Params: behtree.Params{},
			Preconditions: []behtree.ResolvedCondition{
				{Object: "speech", Field: "active", Value: "false"},
				{Object: "task_tree", Field: "pending", Value: "false"},
				{Object: "speech", Field: "observed", Value: "true"},
			},
			Postconditions: []behtree.ResolvedCondition{
				{Object: "system", Field: "idle", Value: "true"},
			},
		},
	}
}

// desktopOuterTestGoal returns the goal conditions for the desktop outer tree.
func desktopOuterTestGoal() []behtree.ResolvedCondition {
	return []behtree.ResolvedCondition{
		{Object: "speech", Field: "observed", Value: "true"},
		{Object: "system", Field: "idle", Value: "true"},
	}
}
