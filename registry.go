package behtree

import "fmt"

type OutcomeRequest int

const (
	RequestSuccess OutcomeRequest = iota
	RequestFailure
	RequestRunning
)

func (o OutcomeRequest) String() string {
	switch o {
	case RequestSuccess:
		return "RequestSuccess"
	case RequestFailure:
		return "RequestFailure"
	case RequestRunning:
		return "RequestRunning"
	default:
		return "UnknownRequest"
	}
}

type HandlerResult struct {
	Status     Status
	Compatible bool
	Logs       []LogEntry
}

type Handler func(params Params, state *State, request OutcomeRequest) HandlerResult

type ActionRegistry struct {
	handlers map[string]Handler
}

// BehaviourRegistry is an alias for backward compatibility.
type BehaviourRegistry = ActionRegistry

func NewActionRegistry() *ActionRegistry {
	return &ActionRegistry{
		handlers: make(map[string]Handler),
	}
}

// NewBehaviourRegistry is an alias for backward compatibility.
func NewBehaviourRegistry() *ActionRegistry {
	return NewActionRegistry()
}

func (r *ActionRegistry) Register(name string, h Handler) {
	r.handlers[name] = h
}

func (r *ActionRegistry) Get(name string) (Handler, error) {
	h, ok := r.handlers[name]
	if !ok {
		return nil, fmt.Errorf("no handler registered for %q", name)
	}
	return h, nil
}

func (r *ActionRegistry) Has(name string) bool {
	_, ok := r.handlers[name]
	return ok
}

// Merge copies all handlers from other into this registry.
// Existing handlers with the same name are overwritten.
func (r *ActionRegistry) Merge(other *ActionRegistry) {
	for name, handler := range other.handlers {
		r.handlers[name] = handler
	}
}
