package behtree

import "fmt"

type State struct {
	Objects map[string]map[string]any
}

func NewState() *State {
	return &State{
		Objects: make(map[string]map[string]any),
	}
}

func NewStateFromEnvironment(env *Environment) *State {
	s := NewState()
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
	return s
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

func (s *State) Clone() *State {
	clone := NewState()
	for name, fields := range s.Objects {
		clonedFields := make(map[string]any)
		for k, v := range fields {
			clonedFields[k] = v
		}
		clone.Objects[name] = clonedFields
	}
	return clone
}
