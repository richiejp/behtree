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

type BehaviourDef struct {
	Name   string               `json:"name"`
	Type   NodeType             `json:"type"`
	Params map[string]ParamType `json:"params,omitempty"`
	Async  bool                 `json:"async,omitempty"`
}

type Environment struct {
	Objects    []ObjectDef    `json:"objects,omitempty"`
	Interfaces []InterfaceDef `json:"interfaces,omitempty"`
	Behaviours []BehaviourDef `json:"behaviours,omitempty"`
	Trees      []*Node        `json:"trees,omitempty"`
}

func (e *Environment) Merge(other *Environment) {
	e.Objects = append(e.Objects, other.Objects...)
	e.Interfaces = append(e.Interfaces, other.Interfaces...)
	e.Behaviours = append(e.Behaviours, other.Behaviours...)
	e.Trees = append(e.Trees, other.Trees...)
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

func (e *Environment) FindBehaviour(name string) *BehaviourDef {
	for i := range e.Behaviours {
		if e.Behaviours[i].Name == name {
			return &e.Behaviours[i]
		}
	}
	return nil
}

func (e *Environment) ObjectNames() []string {
	names := make([]string, len(e.Objects))
	for i, o := range e.Objects {
		names[i] = o.Name
	}
	return names
}

func (e *Environment) BehaviourNames() []string {
	names := make([]string, len(e.Behaviours))
	for i, b := range e.Behaviours {
		names[i] = fmt.Sprintf("%s(%s)", b.Name, b.Type)
	}
	return names
}
