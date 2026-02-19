package behtree

import (
	"encoding/json"
	"fmt"
)

func (ip *Interpreter) tickRunTaskTree(n *Node) (Status, error) {
	varName, ok := n.Params["tree_variable"]
	if !ok {
		return Failure, fmt.Errorf("RunTaskTree requires 'tree_variable' parameter")
	}

	varStr, ok := varName.(string)
	if !ok {
		return Failure, fmt.Errorf("RunTaskTree 'tree_variable' must be a string")
	}

	parts := splitVarPath(varStr)
	if len(parts) != 2 {
		return Failure, fmt.Errorf("RunTaskTree 'tree_variable' must be 'object.field', got %q", varStr)
	}

	val, err := ip.state.Get(parts[0], parts[1])
	if err != nil {
		return Failure, fmt.Errorf("RunTaskTree: %w", err)
	}

	if val == nil {
		return Failure, nil
	}

	var subtree *Node
	switch v := val.(type) {
	case *Node:
		subtree = v
	case map[string]any:
		data, err := json.Marshal(v)
		if err != nil {
			return Failure, fmt.Errorf("RunTaskTree: marshal subtree: %w", err)
		}
		subtree = &Node{}
		if err := json.Unmarshal(data, subtree); err != nil {
			return Failure, fmt.Errorf("RunTaskTree: unmarshal subtree: %w", err)
		}
	default:
		return Failure, fmt.Errorf("RunTaskTree: variable %q is %T, expected *Node or map", varStr, val)
	}

	return ip.Tick(subtree)
}

func splitVarPath(s string) []string {
	for i, c := range s {
		if c == '.' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}
