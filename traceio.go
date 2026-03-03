package behtree

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// TraceMetadata is the first line of a trace JSONL file.
type TraceMetadata struct {
	CaseName  string `json:"case_name"`
	Model     string `json:"model,omitempty"`
	Timestamp string `json:"timestamp"`
	TreeJSON  string `json:"tree_json,omitempty"`
}

// WriteTraces writes trace metadata and scenario traces as JSON Lines.
func WriteTraces(w io.Writer, meta TraceMetadata, traces []*ScenarioTrace) error {
	enc := json.NewEncoder(w)
	if err := enc.Encode(meta); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	for i, t := range traces {
		if err := enc.Encode(t); err != nil {
			return fmt.Errorf("write scenario %d: %w", i, err)
		}
	}
	return nil
}

// ReadTraceMetadata reads only the first line (metadata) from a trace file.
func ReadTraceMetadata(r io.Reader) (*TraceMetadata, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("empty trace file")
	}
	var meta TraceMetadata
	if err := json.Unmarshal(scanner.Bytes(), &meta); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}
	return &meta, nil
}

// ReadScenarioTraces reads scenario traces lazily, calling fn for each.
// Return false from fn to stop reading.
func ReadScenarioTraces(r io.Reader, fn func(index int, trace *ScenarioTrace) bool) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	// Skip metadata line
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return err
		}
		return fmt.Errorf("empty trace file")
	}

	idx := 0
	for scanner.Scan() {
		var trace ScenarioTrace
		if err := json.Unmarshal(scanner.Bytes(), &trace); err != nil {
			return fmt.Errorf("parse scenario %d: %w", idx, err)
		}
		if !fn(idx, &trace) {
			return nil
		}
		idx++
	}
	return scanner.Err()
}
