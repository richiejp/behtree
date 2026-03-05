package behtree

import "fmt"

type FieldType string

const (
	FieldString    FieldType = "string"
	FieldNumber    FieldType = "number"
	FieldBoolean   FieldType = "boolean"
	FieldObject    FieldType = "object"
	FieldInterface FieldType = "interface"
)

type ObjectDef struct {
	Name   string               `json:"name"`
	Fields map[string]FieldType `json:"fields"`
}

type InterfaceDef struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type ParamType string

const (
	ParamObjectRef    ParamType = "object_ref"
	ParamInterfaceRef ParamType = "interface_ref"
	ParamString       ParamType = "string"
	ParamNumber       ParamType = "number"
	ParamBoolean      ParamType = "boolean"
)

type ActionDef struct {
	Name           string               `json:"name"`
	Type           NodeType             `json:"type"`
	Params         map[string]ParamType `json:"params,omitempty"`
	Async          bool                 `json:"async,omitempty"`
	Preconditions  []Condition          `json:"preconditions,omitempty"`
	Postconditions []Condition          `json:"postconditions,omitempty"`
}

// BehaviourDef is an alias for backward compatibility.
type BehaviourDef = ActionDef

type Environment struct {
	Objects    []ObjectDef    `json:"objects,omitempty"`
	Interfaces []InterfaceDef `json:"interfaces,omitempty"`
	Actions    []ActionDef    `json:"actions,omitempty"`
	Trees      []*Node        `json:"trees,omitempty"`
	Goal       []Condition    `json:"goal,omitempty"`

	// Behaviours is an alias for loading legacy JSON with "behaviours" key.
	// Merge() combines both into Actions.
	Behaviours []ActionDef `json:"behaviours,omitempty"`
}

func (e *Environment) Merge(other *Environment) {
	e.Objects = append(e.Objects, other.Objects...)
	e.Interfaces = append(e.Interfaces, other.Interfaces...)
	e.Actions = append(e.Actions, other.Actions...)
	e.Actions = append(e.Actions, other.Behaviours...)
	e.Trees = append(e.Trees, other.Trees...)
	if len(other.Goal) > 0 {
		e.Goal = other.Goal
	}
}

// ConsolidateActions moves any legacy Behaviours into Actions.
func (e *Environment) ConsolidateActions() {
	if len(e.Behaviours) > 0 {
		e.Actions = append(e.Actions, e.Behaviours...)
		e.Behaviours = nil
	}
}

func (e *Environment) FindObject(name string) *ObjectDef {
	for i := range e.Objects {
		if e.Objects[i].Name == name {
			return &e.Objects[i]
		}
	}
	return nil
}

func (e *Environment) FindInterface(name string) *InterfaceDef {
	for i := range e.Interfaces {
		if e.Interfaces[i].Name == name {
			return &e.Interfaces[i]
		}
	}
	return nil
}

func (e *Environment) FindAction(name string) *ActionDef {
	for i := range e.Actions {
		if e.Actions[i].Name == name {
			return &e.Actions[i]
		}
	}
	return nil
}

// FindBehaviour is an alias for FindAction for backward compatibility.
func (e *Environment) FindBehaviour(name string) *ActionDef {
	return e.FindAction(name)
}

func (e *Environment) ObjectNames() []string {
	names := make([]string, len(e.Objects))
	for i, o := range e.Objects {
		names[i] = o.Name
	}
	return names
}

func (e *Environment) ActionNames() []string {
	names := make([]string, len(e.Actions))
	for i, a := range e.Actions {
		names[i] = fmt.Sprintf("%s(%s)", a.Name, a.Type)
	}
	return names
}

// BehaviourNames is an alias for ActionNames for backward compatibility.
func (e *Environment) BehaviourNames() []string {
	return e.ActionNames()
}
