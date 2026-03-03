package behtree

import (
	"fmt"
	"io"
	"strings"
)

// PrintSpanTree renders a span tree as indented ASCII.
func PrintSpanTree(span *Span, w io.Writer) {
	printSpan(span, w, "", true)
}

func printSpan(span *Span, w io.Writer, prefix string, isRoot bool) {
	label := spanLabel(span)

	if isRoot {
		_, _ = fmt.Fprintln(w, label)
	}

	printSpanLogs(span, w, prefix)
	printSpanState(span, w, prefix)

	for i, child := range span.Children {
		isLast := i == len(span.Children)-1
		connector := "├── "
		childPrefix := "│   "
		if isLast {
			connector = "└── "
			childPrefix = "    "
		}

		_, _ = fmt.Fprintf(w, "%s%s%s\n", prefix, connector, spanLabel(child))
		printSpanLogs(child, w, prefix+childPrefix)
		printSpanState(child, w, prefix+childPrefix)

		if len(child.Children) > 0 {
			printSpan(child, w, prefix+childPrefix, false)
		}
	}
}

func spanLabel(span *Span) string {
	var sb strings.Builder
	if span.NodeType.IsLeaf() {
		fmt.Fprintf(&sb, "%s: %s", span.NodeType, span.NodeName)
		if len(span.Params) > 0 {
			fmt.Fprintf(&sb, "(%s)", formatParams(span.Params))
		}
		if span.OutcomeRequest != nil {
			fmt.Fprintf(&sb, " [%s]", span.OutcomeRequest)
		}
	} else {
		sb.WriteString(string(span.NodeType))
	}
	fmt.Fprintf(&sb, " -> %s", span.Status)
	if span.Err != "" {
		fmt.Fprintf(&sb, " (%s)", span.Err)
	}
	return sb.String()
}

func printSpanLogs(span *Span, w io.Writer, prefix string) {
	for _, log := range span.Logs {
		_, _ = fmt.Fprintf(w, "%s[%s] %s\n", prefix, log.Level, log.Message)
	}
}

func printSpanState(span *Span, w io.Writer, prefix string) {
	if span.StateAfter == nil {
		return
	}
	parts := make([]string, 0)
	for obj, fields := range span.StateAfter {
		for field, val := range fields {
			parts = append(parts, fmt.Sprintf("%s.%s=%v", obj, field, val))
		}
	}
	if len(parts) > 0 {
		_, _ = fmt.Fprintf(w, "%sstate: %s\n", prefix, strings.Join(parts, " "))
	}
}

// PrintTickTrace renders a single tick trace.
func PrintTickTrace(tt *TickTrace, w io.Writer) {
	_, _ = fmt.Fprintf(w, "Tick %d:\n", tt.TickIndex+1)
	if tt.Root != nil {
		PrintSpanTree(tt.Root, &indentWriter{w: w, prefix: "  "})
	}
}

// PrintScenarioTrace renders a full scenario trace.
func PrintScenarioTrace(st *ScenarioTrace, w io.Writer) {
	reqs := make([]string, len(st.Requests))
	for i, r := range st.Requests {
		reqs[i] = r.String()
	}
	_, _ = fmt.Fprintf(w, "Requests: [%s]\n", strings.Join(reqs, ", "))

	if st.Skipped {
		_, _ = fmt.Fprintf(w, "Skipped: %s\n", st.SkipMsg)
		return
	}

	for i := range st.Ticks {
		PrintTickTrace(&st.Ticks[i], w)
	}

	if st.FinalState != nil {
		_, _ = fmt.Fprintln(w, "Final State:")
		for obj, fields := range st.FinalState.Objects {
			for field, val := range fields {
				_, _ = fmt.Fprintf(w, "  %s.%s = %v\n", obj, field, val)
			}
		}
	}
}

// indentWriter prefixes every line written to it.
type indentWriter struct {
	w      io.Writer
	prefix string
	atBOL  bool
}

func (iw *indentWriter) Write(p []byte) (int, error) {
	written := 0
	for _, b := range p {
		if iw.atBOL || written == 0 {
			if _, err := fmt.Fprint(iw.w, iw.prefix); err != nil {
				return written, err
			}
			iw.atBOL = false
		}
		n, err := iw.w.Write([]byte{b})
		written += n
		if err != nil {
			return written, err
		}
		if b == '\n' {
			iw.atBOL = true
		}
	}
	return written, nil
}
