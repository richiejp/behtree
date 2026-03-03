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

func PrintEnvironment(env *Environment, w io.Writer) {
	if len(env.Objects) > 0 {
		_, _ = fmt.Fprintln(w, "Objects:")
		for _, obj := range env.Objects {
			_, _ = fmt.Fprintf(w, "  %s\n", obj.Name)
			for field, ftype := range obj.Fields {
				_, _ = fmt.Fprintf(w, "    %s: %s\n", field, ftype)
			}
		}
	}
	if len(env.Interfaces) > 0 {
		_, _ = fmt.Fprintln(w, "Interfaces:")
		for _, iface := range env.Interfaces {
			if iface.Description != "" {
				_, _ = fmt.Fprintf(w, "  %s - %s\n", iface.Name, iface.Description)
			} else {
				_, _ = fmt.Fprintf(w, "  %s\n", iface.Name)
			}
		}
	}
	if len(env.Behaviours) > 0 {
		_, _ = fmt.Fprintln(w, "Behaviours:")
		for _, b := range env.Behaviours {
			params := ""
			if len(b.Params) > 0 {
				parts := make([]string, 0, len(b.Params))
				for name, ptype := range b.Params {
					parts = append(parts, fmt.Sprintf("%s: %s", name, ptype))
				}
				params = "(" + strings.Join(parts, ", ") + ")"
			}
			_, _ = fmt.Fprintf(w, "  [%s] %s%s\n", b.Type, b.Name, params)
		}
	}
}

func PrintEnvironmentString(env *Environment) string {
	var sb strings.Builder
	PrintEnvironment(env, &sb)
	return sb.String()
}
