package behtree

import (
	"encoding/json"
	"io"
	"time"
)

type BenchmarkDifficulty int

const (
	DifficultyTrivial BenchmarkDifficulty = iota
	DifficultySimple
	DifficultyModerate
	DifficultyComplex
)

func (d BenchmarkDifficulty) String() string {
	switch d {
	case DifficultyTrivial:
		return "trivial"
	case DifficultySimple:
		return "simple"
	case DifficultyModerate:
		return "moderate"
	case DifficultyComplex:
		return "complex"
	default:
		return "unknown"
	}
}

// SimulateOptions controls simulation behavior.
type SimulateOptions struct {
	TraceEnabled bool
	CaptureState bool
}

type BenchmarkCase struct {
	Name        string
	Description string
	Difficulty  BenchmarkDifficulty
	Environment *Environment
	Prompt      string
	Validate    func(tree *Node, env *Environment) []ValidationError
	Simulate    func(tree *Node, env *Environment, registry *BehaviourRegistry, opts SimulateOptions) []*ScenarioResult
}

type BenchmarkResult struct {
	Case          *BenchmarkCase
	GeneratedTree *Node
	ParseError    error
	Validation    []ValidationError
	Scenarios     []*ScenarioResult
	Passed        bool
}

type BenchmarkSuite struct {
	Cases        []*BenchmarkCase
	SimulateOpts SimulateOptions
}

type SavedTree struct {
	CaseName  string    `json:"case_name"`
	Model     string    `json:"model"`
	Timestamp time.Time `json:"timestamp"`
	Tree      *Node     `json:"tree"`
}

func WriteSavedTree(w io.Writer, st *SavedTree) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(st)
}

func ReadSavedTree(r io.Reader) (*SavedTree, error) {
	var st SavedTree
	if err := json.NewDecoder(r).Decode(&st); err != nil {
		return nil, err
	}
	return &st, nil
}

func NewBenchmarkSuite() *BenchmarkSuite {
	return &BenchmarkSuite{}
}

func (s *BenchmarkSuite) Add(c *BenchmarkCase) {
	s.Cases = append(s.Cases, c)
}

func (s *BenchmarkSuite) EvalTree(bc *BenchmarkCase, tree *Node) *BenchmarkResult {
	result := &BenchmarkResult{
		Case:          bc,
		GeneratedTree: tree,
	}

	env := &Environment{}
	env.Merge(bc.Environment)
	if tree != nil {
		env.Trees = append(env.Trees, tree)
	}

	if bc.Validate != nil {
		result.Validation = bc.Validate(tree, env)
	} else {
		result.Validation = Validate(env)
	}

	if len(result.Validation) > 0 {
		return result
	}

	if bc.Simulate != nil {
		registry := NewBehaviourRegistry()
		result.Scenarios = bc.Simulate(tree, env, registry, s.SimulateOpts)
	}

	result.Passed = true
	if result.Scenarios != nil {
		for _, sr := range result.Scenarios {
			if sr.Skipped {
				continue
			}
			lastTick := sr.Ticks[len(sr.Ticks)-1]
			if lastTick.Err != nil || lastTick.Status == Failure {
				result.Passed = false
				break
			}
		}
	}

	return result
}

func (s *BenchmarkSuite) Run(generateTree func(prompt string, env *Environment) ([]byte, error)) []*BenchmarkResult {
	var results []*BenchmarkResult

	for _, bc := range s.Cases {
		treeJSON, err := generateTree(bc.Prompt, bc.Environment)
		if err != nil {
			results = append(results, &BenchmarkResult{Case: bc, ParseError: err})
			continue
		}

		doc, err := ParseDocument(treeJSON)
		if err != nil {
			results = append(results, &BenchmarkResult{Case: bc, ParseError: err})
			continue
		}

		results = append(results, s.EvalTree(bc, doc.Tree))
	}

	return results
}
