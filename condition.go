package behtree

import (
	"fmt"
	"strings"
)

// Condition is a template condition used in action pre/postconditions.
// Fields prefixed with "$" are parameter references resolved at grounding time.
type Condition struct {
	Object string `json:"object"`
	Field  string `json:"field"`
	Value  string `json:"value"`
}

// StateRef is a runtime reference to a state field, evaluated at tick-time.
type StateRef struct {
	Object string
	Field  string
}

// ResolvedCondition is a condition with parameter references resolved to
// literal object names. ValueRef (if set) is a runtime state lookup.
type ResolvedCondition struct {
	Object   string
	Field    string
	Value    string    // literal value to compare
	ValueRef *StateRef // OR: look up this state field at tick-time
}

// Name returns a human-readable name for use as a condition node name.
func (rc ResolvedCondition) Name() string {
	if rc.ValueRef != nil {
		return fmt.Sprintf("%s.%s==%s.%s", rc.Object, rc.Field, rc.ValueRef.Object, rc.ValueRef.Field)
	}
	return fmt.Sprintf("%s.%s==%s", rc.Object, rc.Field, rc.Value)
}

// Resolve grounds a template condition against action params.
// "$param" in Object or Value resolves to params[param] (literal string).
// "$param.field" in Value resolves to StateRef{Object: params[param], Field: field}.
func (c Condition) Resolve(params Params) (ResolvedCondition, error) {
	rc := ResolvedCondition{Field: c.Field}

	// Resolve object
	if strings.HasPrefix(c.Object, "$") {
		paramName := c.Object[1:]
		val, ok := params[paramName]
		if !ok {
			return rc, fmt.Errorf("unresolved param %q in object", paramName)
		}
		s, ok := val.(string)
		if !ok {
			return rc, fmt.Errorf("param %q must be string, got %T", paramName, val)
		}
		rc.Object = s
	} else {
		rc.Object = c.Object
	}

	// Resolve value
	if strings.HasPrefix(c.Value, "$") {
		ref := c.Value[1:]
		if dotIdx := strings.IndexByte(ref, '.'); dotIdx >= 0 {
			// $param.field -> StateRef
			paramName := ref[:dotIdx]
			fieldName := ref[dotIdx+1:]
			val, ok := params[paramName]
			if !ok {
				return rc, fmt.Errorf("unresolved param %q in value ref", paramName)
			}
			s, ok := val.(string)
			if !ok {
				return rc, fmt.Errorf("param %q must be string, got %T", paramName, val)
			}
			rc.ValueRef = &StateRef{Object: s, Field: fieldName}
		} else {
			// $param -> literal lookup
			val, ok := params[ref]
			if !ok {
				return rc, fmt.Errorf("unresolved param %q in value", ref)
			}
			s, ok := val.(string)
			if !ok {
				return rc, fmt.Errorf("param %q must be string, got %T", ref, val)
			}
			rc.Value = s
		}
	} else {
		rc.Value = c.Value
	}

	return rc, nil
}

// Evaluate checks whether this resolved condition holds against current state.
func (rc ResolvedCondition) Evaluate(state *State) (bool, error) {
	actual, err := state.Get(rc.Object, rc.Field)
	if err != nil {
		return false, err
	}

	var expected string
	if rc.ValueRef != nil {
		val, err := state.Get(rc.ValueRef.Object, rc.ValueRef.Field)
		if err != nil {
			return false, err
		}
		expected = fmt.Sprintf("%v", val)
	} else {
		expected = rc.Value
	}

	return fmt.Sprintf("%v", actual) == expected, nil
}

// Matches returns true if this condition targets the same object.field and
// has an equivalent value (both literal and equal, or both StateRefs to same location).
func (rc ResolvedCondition) Matches(other ResolvedCondition) bool {
	if rc.Object != other.Object || rc.Field != other.Field {
		return false
	}
	if rc.ValueRef != nil && other.ValueRef != nil {
		return rc.ValueRef.Object == other.ValueRef.Object &&
			rc.ValueRef.Field == other.ValueRef.Field
	}
	if rc.ValueRef == nil && other.ValueRef == nil {
		return rc.Value == other.Value
	}
	return false
}

// Contradicts returns true if this condition targets the same object.field
// but requires a different value.
func (rc ResolvedCondition) Contradicts(other ResolvedCondition) bool {
	if rc.Object != other.Object || rc.Field != other.Field {
		return false
	}
	return !rc.Matches(other)
}
