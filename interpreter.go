package behtree

import "fmt"

type Interpreter struct {
	env      *Environment
	registry *BehaviourRegistry
	state    *State
	request  OutcomeRequest
}

func NewInterpreter(env *Environment, registry *BehaviourRegistry, state *State) *Interpreter {
	return &Interpreter{
		env:      env,
		registry: registry,
		state:    state,
		request:  RequestSuccess,
	}
}

func (ip *Interpreter) State() *State {
	return ip.state
}

func (ip *Interpreter) SetOutcomeRequest(r OutcomeRequest) {
	ip.request = r
}

func (ip *Interpreter) Tick(n *Node) (Status, error) {
	if n == nil {
		return Failure, fmt.Errorf("nil node")
	}

	switch n.Type {
	case SequenceNode:
		return ip.tickSequence(n)
	case FallbackNode:
		return ip.tickFallback(n)
	case ConditionNode, ActionNode:
		return ip.tickLeaf(n)
	case InverterNode:
		return ip.tickInverter(n)
	case ForceSuccessNode:
		return ip.tickForceSuccess(n)
	case ForceFailureNode:
		return ip.tickForceFailure(n)
	case RetryUntilSuccessfulNode:
		return ip.tickRetryUntilSuccessful(n)
	default:
		return Failure, fmt.Errorf("unknown node type %q", n.Type)
	}
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

	result := handler(n.Params, ip.state, ip.request)
	if !result.Compatible {
		return Failure, fmt.Errorf("handler %q: state incompatible with request %s", n.Name, ip.request)
	}
	return result.Status, nil
}

func (ip *Interpreter) tickInverter(n *Node) (Status, error) {
	if len(n.Children) != 1 {
		return Failure, fmt.Errorf("inverter requires exactly 1 child")
	}
	status, err := ip.Tick(n.Children[0])
	if err != nil {
		return Failure, err
	}
	switch status {
	case Success:
		return Failure, nil
	case Failure:
		return Success, nil
	default:
		return status, nil
	}
}

func (ip *Interpreter) tickForceSuccess(n *Node) (Status, error) {
	if len(n.Children) != 1 {
		return Failure, fmt.Errorf("ForceSuccess requires exactly 1 child")
	}
	status, err := ip.Tick(n.Children[0])
	if err != nil {
		return Failure, err
	}
	if status == Running {
		return Running, nil
	}
	return Success, nil
}

func (ip *Interpreter) tickForceFailure(n *Node) (Status, error) {
	if len(n.Children) != 1 {
		return Failure, fmt.Errorf("ForceFailure requires exactly 1 child")
	}
	status, err := ip.Tick(n.Children[0])
	if err != nil {
		return Failure, err
	}
	if status == Running {
		return Running, nil
	}
	return Failure, nil
}

func (ip *Interpreter) tickRetryUntilSuccessful(n *Node) (Status, error) {
	if len(n.Children) != 1 {
		return Failure, fmt.Errorf("RetryUntilSuccessful requires exactly 1 child")
	}
	status, err := ip.Tick(n.Children[0])
	if err != nil {
		return Failure, err
	}
	if status == Success {
		return Success, nil
	}
	return Running, nil
}
