package behtree

import "encoding/json"

type Status int

const (
	Success Status = iota
	Failure
	Running
)

func (s Status) String() string {
	switch s {
	case Success:
		return "SUCCESS"
	case Failure:
		return "FAILURE"
	case Running:
		return "RUNNING"
	default:
		return "UNKNOWN"
	}
}

type NodeType string

const (
	SequenceNode  NodeType = "Sequence"
	FallbackNode  NodeType = "Fallback"
	ConditionNode NodeType = "Condition"
	ActionNode    NodeType = "Action"

	InverterNode             NodeType = "Inverter"
	ForceSuccessNode         NodeType = "ForceSuccess"
	ForceFailureNode         NodeType = "ForceFailure"
	RetryUntilSuccessfulNode NodeType = "RetryUntilSuccessful"
)

func (n NodeType) IsControl() bool {
	return n == SequenceNode || n == FallbackNode
}

func (n NodeType) IsDecorator() bool {
	switch n {
	case InverterNode, ForceSuccessNode, ForceFailureNode, RetryUntilSuccessfulNode:
		return true
	}
	return false
}

func (n NodeType) IsLeaf() bool {
	return n == ConditionNode || n == ActionNode
}

type Params map[string]any

type Node struct {
	Type     NodeType `json:"type"`
	Name     string   `json:"name,omitempty"`
	Params   Params   `json:"params,omitempty"`
	Children []*Node  `json:"children,omitempty"`
}

func (n *Node) UnmarshalJSON(data []byte) error {
	type Alias Node
	aux := &Alias{}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	*n = Node(*aux)
	return nil
}
