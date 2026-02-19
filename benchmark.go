package behtree

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

type BenchmarkCase struct {
	Name        string
	Description string
	Difficulty  BenchmarkDifficulty
	Environment *Environment
	Prompt      string
	Validate    func(tree *Node, env *Environment) []ValidationError
	Simulate    func(tree *Node, env *Environment, registry *BehaviourRegistry) []*ScenarioResult
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
	Cases []*BenchmarkCase
}

func NewBenchmarkSuite() *BenchmarkSuite {
	return &BenchmarkSuite{}
}

func (s *BenchmarkSuite) Add(c *BenchmarkCase) {
	s.Cases = append(s.Cases, c)
}

func (s *BenchmarkSuite) Run(generateTree func(prompt string, env *Environment) ([]byte, error)) []*BenchmarkResult {
	var results []*BenchmarkResult

	for _, bc := range s.Cases {
		result := &BenchmarkResult{Case: bc}

		treeJSON, err := generateTree(bc.Prompt, bc.Environment)
		if err != nil {
			result.ParseError = err
			results = append(results, result)
			continue
		}

		doc, err := ParseDocument(treeJSON)
		if err != nil {
			result.ParseError = err
			results = append(results, result)
			continue
		}

		result.GeneratedTree = doc.Tree

		env := &Environment{}
		env.Merge(bc.Environment)
		if doc.Tree != nil {
			env.Trees = append(env.Trees, doc.Tree)
		}

		if bc.Validate != nil {
			result.Validation = bc.Validate(doc.Tree, env)
		} else {
			result.Validation = Validate(env)
		}

		if len(result.Validation) > 0 {
			results = append(results, result)
			continue
		}

		if bc.Simulate != nil {
			registry := NewBehaviourRegistry()
			result.Scenarios = bc.Simulate(doc.Tree, env, registry)
		}

		result.Passed = len(result.Validation) == 0 && result.ParseError == nil
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

		results = append(results, result)
	}

	return results
}
