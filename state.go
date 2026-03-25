package behtree

import "fmt"

type State struct {
	Objects         map[string]map[string]any
	EphemeralFields []string
}

func NewState() *State {
	return &State{
		Objects: make(map[string]map[string]any),
	}
}

func NewStateFromEnvironment(env *Environment) *State {
	s := NewState()
	seen := make(map[string]bool)
	for _, obj := range env.Objects {
		fields := make(map[string]any)
		for name, ftype := range obj.Fields {
			switch ftype {
			case FieldString:
				fields[name] = ""
			case FieldNumber:
				fields[name] = 0.0
			case FieldBoolean:
				fields[name] = false
			case FieldObject:
				fields[name] = map[string]any{}
			case FieldInterface:
				fields[name] = nil
			}
		}
		s.Objects[obj.Name] = fields
	}
	// Auto-detect ephemeral fields from action postconditions:
	// any field that an action sets and is also used as a precondition
	// with the same field name across multiple actions likely needs
	// per-tick reset. For now, detect fields named in postconditions
	// that exist on objects — callers can override EphemeralFields.
	for _, obj := range env.Objects {
		for name := range obj.Fields {
			if !seen[name] && isEphemeralFieldName(name) {
				s.EphemeralFields = append(s.EphemeralFields, name)
				seen[name] = true
			}
		}
	}
	return s
}

// knownEphemeralFields lists field names that are treated as ephemeral
// (reset to "false" at each tick start). Add new names here as needed.
var knownEphemeralFields = map[string]bool{
	"observed": true,
	"idle":     true,
	"scanned":  true,
}

func isEphemeralFieldName(name string) bool {
	return knownEphemeralFields[name]
}

func (s *State) Get(object, field string) (any, error) {
	obj, ok := s.Objects[object]
	if !ok {
		return nil, fmt.Errorf("unknown object %q", object)
	}
	val, ok := obj[field]
	if !ok {
		return nil, fmt.Errorf("unknown field %q on object %q", field, object)
	}
	return val, nil
}

func (s *State) Set(object, field string, value any) error {
	obj, ok := s.Objects[object]
	if !ok {
		return fmt.Errorf("unknown object %q", object)
	}
	obj[field] = value
	return nil
}

// ResetEphemeral resets all ephemeral fields to "false" across all objects.
// Ephemeral fields are those that must be re-established each tick
// (e.g. "observed" forces re-observation, "idle" forces re-evaluation).
// The field names are configured via State.EphemeralFields.
func (s *State) ResetEphemeral() {
	for _, fields := range s.Objects {
		for _, name := range s.EphemeralFields {
			if _, ok := fields[name]; ok {
				fields[name] = "false"
			}
		}
	}
}

func (s *State) Clone() *State {
	clone := NewState()
	clone.EphemeralFields = make([]string, len(s.EphemeralFields))
	copy(clone.EphemeralFields, s.EphemeralFields)
	for name, fields := range s.Objects {
		clonedFields := make(map[string]any)
		for k, v := range fields {
			clonedFields[k] = v
		}
		clone.Objects[name] = clonedFields
	}
	return clone
}
