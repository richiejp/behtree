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

type BehaviourRegistry struct {
	handlers map[string]Handler
}

func NewBehaviourRegistry() *BehaviourRegistry {
	return &BehaviourRegistry{
		handlers: make(map[string]Handler),
	}
}

func (r *BehaviourRegistry) Register(name string, h Handler) {
	r.handlers[name] = h
}

func (r *BehaviourRegistry) Get(name string) (Handler, error) {
	h, ok := r.handlers[name]
	if !ok {
		return nil, fmt.Errorf("no handler registered for behaviour %q", name)
	}
	return h, nil
}

func (r *BehaviourRegistry) Has(name string) bool {
	_, ok := r.handlers[name]
	return ok
}
