package behtree

import "fmt"

type Interpreter struct {
	env         *Environment
	registry    *BehaviourRegistry
	state       *State
	requestFunc func() OutcomeRequest
	tracer      Tracer
	tickDepth   int
}

func NewInterpreter(env *Environment, registry *BehaviourRegistry, state *State) *Interpreter {
	return &Interpreter{
		env:         env,
		registry:    registry,
		state:       state,
		requestFunc: fixedRequest(RequestSuccess),
		tracer:      NoopTracer{},
	}
}

func fixedRequest(r OutcomeRequest) func() OutcomeRequest {
	return func() OutcomeRequest { return r }
}

// SetTracer sets the tracer for this interpreter. Pass nil to disable.
func (ip *Interpreter) SetTracer(t Tracer) {
	if t == nil {
		ip.tracer = NoopTracer{}
		return
	}
	ip.tracer = t
}

func (ip *Interpreter) State() *State {
	return ip.state
}

func (ip *Interpreter) SetOutcomeRequest(r OutcomeRequest) {
	ip.requestFunc = fixedRequest(r)
}

// SetRequestSource sets a function that provides outcome requests per leaf visit.
// Each call to the function should return the next request in the sequence.
func (ip *Interpreter) SetRequestSource(fn func() OutcomeRequest) {
	ip.requestFunc = fn
}

func (ip *Interpreter) Tick(n *Node) (Status, error) {
	if n == nil {
		return Failure, fmt.Errorf("nil node")
	}

	if ip.tickDepth == 0 {
		ip.state.ResetEphemeral()
	}
	ip.tickDepth++
	defer func() { ip.tickDepth-- }()

	ip.tracer.EnterNode(n)

	var status Status
	var err error

	switch n.Type {
	case SequenceNode:
		status, err = ip.tickSequence(n)
	case FallbackNode:
		status, err = ip.tickFallback(n)
	case ConditionNode, ActionNode:
		status, err = ip.tickLeaf(n)
	default:
		status, err = Failure, fmt.Errorf("unknown node type %q", n.Type)
	}

	ip.tracer.ExitNode(status, err)
	return status, err
}

func (ip *Interpreter) tickSequence(n *Node) (Status, error) {
	for _, child := range n.Children {
		status, err := ip.Tick(child)
		if err != nil {
			return Failure, err
		}
		if status != Success {
			return status, nil
		}
	}
	return Success, nil
}

func (ip *Interpreter) tickFallback(n *Node) (Status, error) {
	for _, child := range n.Children {
		status, err := ip.Tick(child)
		if err != nil {
			return Failure, err
		}
		if status != Failure {
			return status, nil
		}
	}
	return Failure, nil
}

func (ip *Interpreter) tickLeaf(n *Node) (Status, error) {
	if n.Name == "RunTaskTree" {
		return ip.tickRunTaskTree(n)
	}

	handler, err := ip.registry.Get(n.Name)
	if err != nil {
		return Failure, err
	}

	request := ip.requestFunc()
	ip.tracer.RecordRequest(request)

	result := handler(n.Params, ip.state, request)
	if len(result.Logs) > 0 {
		ip.tracer.LogHandler(result.Logs)
	}
	ip.tracer.SnapshotState(ip.state)

	if !result.Compatible {
		return Failure, fmt.Errorf("handler %q: state incompatible with request %s", n.Name, request)
	}
	return result.Status, nil
}
