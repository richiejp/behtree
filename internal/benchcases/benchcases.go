package benchcases

import (
	"path/filepath"

	"github.com/richiejp/behtree"
)

// RobotCase constructs the robot pick-and-place benchmark case.
// testdataDir is the path to the directory containing robot.json.
func RobotCase(testdataDir string) (*behtree.BenchmarkCase, error) {
	doc, err := behtree.LoadDocument(filepath.Join(testdataDir, "robot.json"))
	if err != nil {
		return nil, err
	}

	env := behtree.MergeDocuments(&behtree.Document{
		Objects:    doc.Objects,
		Behaviours: doc.Behaviours,
	})

	return &behtree.BenchmarkCase{
		Name:        "Robot Pick-and-Place",
		Description: "Generate a tree to pick up a wrapper from the table and drop it in the bin",
		Difficulty:  behtree.DifficultySimple,
		Environment: env,
		Prompt:      "Pick up the wrapper from the table and drop it in the bin.",
		Simulate:    robotSimulate,
	}, nil
}

func robotSimulate(tree *behtree.Node, env *behtree.Environment, registry *behtree.BehaviourRegistry) []*behtree.ScenarioResult {
	RegisterRobotHandlers(registry)
	harness := behtree.NewSimulationHarness(env, registry, tree)
	return harness.RunAllOutcomes(10)
}

// DesktopCase constructs the desktop open-URL benchmark case.
// testdataDir is the path to the directory containing desktop_env.json.
func DesktopCase(testdataDir string) (*behtree.BenchmarkCase, error) {
	doc, err := behtree.LoadDocument(filepath.Join(testdataDir, "desktop_env.json"))
	if err != nil {
		return nil, err
	}

	env := behtree.MergeDocuments(doc)

	return &behtree.BenchmarkCase{
		Name:        "Desktop Open URL",
		Description: "Generate a tree to open a URL in Firefox, ensuring the browser is open and window is focused",
		Difficulty:  behtree.DifficultyModerate,
		Environment: env,
		Prompt:      "Open the LocalAI GitHub page (https://github.com/mudler/LocalAI) in Firefox. First ensure Firefox is open, then navigate to the URL, then ensure the window is focused.",
	}, nil
}
