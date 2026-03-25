package behtree

import (
	"fmt"
	"io"
	"slices"
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
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, 0, len(p))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%v", p[k]))
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
	if len(env.Actions) > 0 {
		_, _ = fmt.Fprintln(w, "Actions:")
		for _, a := range env.Actions {
			params := ""
			if len(a.Params) > 0 {
				parts := make([]string, 0, len(a.Params))
				for name, ptype := range a.Params {
					parts = append(parts, fmt.Sprintf("%s: %s", name, ptype))
				}
				params = "(" + strings.Join(parts, ", ") + ")"
			}
			_, _ = fmt.Fprintf(w, "  [%s] %s%s\n", a.Type, a.Name, params)
		}
	}
}

func PrintEnvironmentString(env *Environment) string {
	var sb strings.Builder
	PrintEnvironment(env, &sb)
	return sb.String()
}
