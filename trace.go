package behtree

type LogLevel int

const (
	LogInfo LogLevel = iota
	LogError
)

func (l LogLevel) String() string {
	switch l {
	case LogInfo:
		return "info"
	case LogError:
		return "error"
	default:
		return "unknown"
	}
}

type LogEntry struct {
	Level   LogLevel `json:"level"`
	Message string   `json:"message"`
}

// Span represents one node evaluation during a tree tick.
// Children nest to mirror the tree execution structure.
type Span struct {
	NodeType       NodeType                  `json:"node_type"`
	NodeName       string                    `json:"node_name,omitempty"`
	Params         Params                    `json:"params,omitempty"`
	OutcomeRequest *OutcomeRequest           `json:"outcome_request,omitempty"`
	Status         Status                    `json:"status"`
	Err            string                    `json:"error,omitempty"`
	Logs           []LogEntry                `json:"logs,omitempty"`
	StateAfter     map[string]map[string]any `json:"state_after,omitempty"`
	Children       []*Span                   `json:"children,omitempty"`
}

// TickTrace captures the full trace of one Tick() call.
type TickTrace struct {
	TickIndex      int            `json:"tick_index"`
	OutcomeRequest OutcomeRequest `json:"outcome_request"`
	Root           *Span          `json:"root"`
}

// ScenarioTrace captures all tick traces for one scenario run.
type ScenarioTrace struct {
	Requests   []OutcomeRequest `json:"requests"`
	Ticks      []TickTrace      `json:"ticks"`
	FinalState *State           `json:"final_state,omitempty"`
	Skipped    bool             `json:"skipped"`
	SkipMsg    string           `json:"skip_msg,omitempty"`
	Failed     bool             `json:"failed"`
}

// Tracer receives events from the interpreter during execution.
type Tracer interface {
	EnterNode(n *Node)
	ExitNode(status Status, err error)
	RecordRequest(req OutcomeRequest)
	LogHandler(logs []LogEntry)
	SnapshotState(state *State)
}

// NoopTracer is a zero-allocation tracer that does nothing.
type NoopTracer struct{}

func (NoopTracer) EnterNode(*Node)              {}
func (NoopTracer) ExitNode(Status, error)       {}
func (NoopTracer) RecordRequest(OutcomeRequest) {}
func (NoopTracer) LogHandler([]LogEntry)        {}
func (NoopTracer) SnapshotState(*State)         {}

// RecordingTracer builds a tree of Spans as the interpreter executes.
type RecordingTracer struct {
	root         *Span
	stack        []*Span
	captureState bool
}

// NewRecordingTracer creates a tracer that records a span tree.
// If captureState is true, it snapshots state after each leaf execution.
func NewRecordingTracer(captureState bool) *RecordingTracer {
	return &RecordingTracer{
		captureState: captureState,
	}
}

func (t *RecordingTracer) EnterNode(n *Node) {
	span := &Span{
		NodeType: n.Type,
		NodeName: n.Name,
		Params:   n.Params,
	}

	if len(t.stack) > 0 {
		parent := t.stack[len(t.stack)-1]
		parent.Children = append(parent.Children, span)
	} else {
		t.root = span
	}

	t.stack = append(t.stack, span)
}

func (t *RecordingTracer) ExitNode(status Status, err error) {
	if len(t.stack) == 0 {
		return
	}
	span := t.stack[len(t.stack)-1]
	t.stack = t.stack[:len(t.stack)-1]

	span.Status = status
	if err != nil {
		span.Err = err.Error()
	}
}

func (t *RecordingTracer) RecordRequest(req OutcomeRequest) {
	if len(t.stack) == 0 {
		return
	}
	span := t.stack[len(t.stack)-1]
	span.OutcomeRequest = &req
}

func (t *RecordingTracer) LogHandler(logs []LogEntry) {
	if len(t.stack) == 0 {
		return
	}
	span := t.stack[len(t.stack)-1]
	span.Logs = append(span.Logs, logs...)
}

func (t *RecordingTracer) SnapshotState(state *State) {
	if !t.captureState || len(t.stack) == 0 {
		return
	}
	span := t.stack[len(t.stack)-1]
	span.StateAfter = state.Clone().Objects
}

// Root returns the root span after the tick completes.
func (t *RecordingTracer) Root() *Span {
	return t.root
}

// Reset clears the tracer for the next tick.
func (t *RecordingTracer) Reset() {
	t.root = nil
	t.stack = t.stack[:0]
}
