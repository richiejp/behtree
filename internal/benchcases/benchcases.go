package benchcases

import (
	"path/filepath"

	"github.com/richiejp/behtree"
)

// RobotCase constructs the robot pick-and-place benchmark case.
// testdataDir is the path to the directory containing robot_v2.json.
func RobotCase(testdataDir string) (*behtree.BenchmarkCase, error) {
	doc, err := behtree.LoadDocument(filepath.Join(testdataDir, "robot_v2.json"))
	if err != nil {
		return nil, err
	}

	env := behtree.MergeDocuments(doc)

	return &behtree.BenchmarkCase{
		Name:        "Robot Pick-and-Place",
		Description: "Select and ground actions to pick up a wrapper from the table and drop it in the bin",
		Difficulty:  behtree.DifficultySimple,
		Environment: env,
		Prompt:      "Pick up the wrapper from the table and drop it in the bin.",
		Simulate:    robotSimulate,
	}, nil
}

func robotSimulate(tree *behtree.Node, env *behtree.Environment, registry *behtree.ActionRegistry, opts behtree.SimulateOptions) []*behtree.ScenarioResult {
	RegisterRobotHandlers(registry)
	harness := behtree.NewSimulationHarness(env, registry, tree)
	harness.SetTracing(opts.TraceEnabled)
	harness.SetCaptureState(opts.CaptureState)
	return harness.RunAllOutcomes(100)
}

// DesktopCase constructs the desktop open-URL benchmark case.
// testdataDir is the path to the directory containing desktop_v2.json.
func DesktopCase(testdataDir string) (*behtree.BenchmarkCase, error) {
	doc, err := behtree.LoadDocument(filepath.Join(testdataDir, "desktop_v2.json"))
	if err != nil {
		return nil, err
	}

	env := behtree.MergeDocuments(doc)

	return &behtree.BenchmarkCase{
		Name:        "Desktop Open URL",
		Description: "Select and ground actions to open a URL in Firefox via PA-BT",
		Difficulty:  behtree.DifficultyModerate,
		Environment: env,
		Prompt:      "Open the LocalAI GitHub page (https://github.com/mudler/LocalAI) in Firefox.",
		Simulate:    desktopSimulate,
	}, nil
}

func desktopSimulate(tree *behtree.Node, env *behtree.Environment, registry *behtree.ActionRegistry, opts behtree.SimulateOptions) []*behtree.ScenarioResult {
	RegisterDesktopInnerHandlers(registry)
	harness := behtree.NewSimulationHarness(env, registry, tree)
	harness.SetTracing(opts.TraceEnabled)
	harness.SetCaptureState(opts.CaptureState)
	return harness.RunAllOutcomes(100)
}
