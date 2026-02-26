package behtree

import (
	"fmt"
	"io"
	"strings"
)

func Print(n *Node, w io.Writer) {
	printNode(n, w, "", true)
}

func PrintTree(n *Node) string {
	var sb strings.Builder
	printNode(n, &sb, "", true)
	return sb.String()
}

func printNode(n *Node, w io.Writer, prefix string, isRoot bool) {
	label := nodeLabel(n)

	if isRoot {
		_, _ = fmt.Fprintln(w, label)
	}

	for i, child := range n.Children {
		isLast := i == len(n.Children)-1
		connector := "├── "
		childPrefix := "│   "
		if isLast {
			connector = "└── "
			childPrefix = "    "
		}

		childLabel := nodeLabel(child)
		_, _ = fmt.Fprintf(w, "%s%s%s\n", prefix, connector, childLabel)

		if len(child.Children) > 0 {
			printNode(child, w, prefix+childPrefix, false)
		}
	}
}

func nodeLabel(n *Node) string {
	if n.Type.IsLeaf() {
		label := fmt.Sprintf("%s: %s", n.Type, n.Name)
		if len(n.Params) > 0 {
			label += "(" + formatParams(n.Params) + ")"
		}
		return label
	}
	return string(n.Type)
}

func formatParams(p Params) string {
	parts := make([]string, 0, len(p))
	for _, v := range p {
		parts = append(parts, fmt.Sprintf("%v", v))
	}
	return strings.Join(parts, ", ")
}
