package behtree

import "fmt"

type ValidationError struct {
	Path    string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

func Validate(env *Environment) []ValidationError {
	var errs []ValidationError

	seen := map[string]bool{}
	for _, o := range env.Objects {
		if seen[o.Name] {
			errs = append(errs, ValidationError{"objects", fmt.Sprintf("duplicate object %q", o.Name)})
		}
		seen[o.Name] = true
	}

	seenIface := map[string]bool{}
	for _, i := range env.Interfaces {
		if seenIface[i.Name] {
			errs = append(errs, ValidationError{"interfaces", fmt.Sprintf("duplicate interface %q", i.Name)})
		}
		seenIface[i.Name] = true
	}

	seenBeh := map[string]bool{}
	for _, b := range env.Behaviours {
		if seenBeh[b.Name] {
			errs = append(errs, ValidationError{"behaviours", fmt.Sprintf("duplicate behaviour %q", b.Name)})
		}
		seenBeh[b.Name] = true

		if !b.Type.IsLeaf() {
			errs = append(errs, ValidationError{
				fmt.Sprintf("behaviours.%s", b.Name),
				fmt.Sprintf("behaviour type must be Condition or Action, got %q", b.Type),
			})
		}
		if b.Async && b.Type == ConditionNode {
			errs = append(errs, ValidationError{
				fmt.Sprintf("behaviours.%s", b.Name),
				"conditions cannot be async",
			})
		}
	}

	for i, tree := range env.Trees {
		path := fmt.Sprintf("trees[%d]", i)
		errs = append(errs, validateNode(env, tree, path)...)
	}

	return errs
}

func validateNode(env *Environment, n *Node, path string) []ValidationError {
	if n == nil {
		return []ValidationError{{path, "nil node"}}
	}

	var errs []ValidationError

	switch {
	case n.Type.IsControl():
		if len(n.Children) == 0 {
			errs = append(errs, ValidationError{path, fmt.Sprintf("%s node must have at least one child", n.Type)})
		}
		if n.Name != "" {
			errs = append(errs, ValidationError{path, fmt.Sprintf("%s node should not have a name", n.Type)})
		}
		for i, child := range n.Children {
			childPath := fmt.Sprintf("%s.children[%d]", path, i)
			errs = append(errs, validateNode(env, child, childPath)...)
		}

	case n.Type.IsDecorator():
		if len(n.Children) != 1 {
			errs = append(errs, ValidationError{path, fmt.Sprintf("%s decorator must have exactly one child, got %d", n.Type, len(n.Children))})
		}
		for i, child := range n.Children {
			childPath := fmt.Sprintf("%s.children[%d]", path, i)
			errs = append(errs, validateNode(env, child, childPath)...)
		}

	case n.Type.IsLeaf():
		if n.Name == "" {
			errs = append(errs, ValidationError{path, fmt.Sprintf("%s node must have a name", n.Type)})
		}
		if len(n.Children) > 0 {
			errs = append(errs, ValidationError{path, fmt.Sprintf("%s node must not have children", n.Type)})
		}

		beh := env.FindBehaviour(n.Name)
		if beh == nil {
			errs = append(errs, ValidationError{path, fmt.Sprintf("unknown behaviour %q", n.Name)})
		} else {
			if beh.Type != n.Type {
				errs = append(errs, ValidationError{path, fmt.Sprintf("behaviour %q is %s but used as %s", n.Name, beh.Type, n.Type)})
			}
			errs = append(errs, validateParams(env, beh, n.Params, path)...)
		}

	default:
		errs = append(errs, ValidationError{path, fmt.Sprintf("unknown node type %q", n.Type)})
	}

	return errs
}

func validateParams(env *Environment, beh *BehaviourDef, params Params, path string) []ValidationError {
	var errs []ValidationError

	for name := range params {
		if _, ok := beh.Params[name]; !ok {
			errs = append(errs, ValidationError{
				fmt.Sprintf("%s.params.%s", path, name),
				fmt.Sprintf("unexpected parameter %q for behaviour %q", name, beh.Name),
			})
		}
	}

	for name, ptype := range beh.Params {
		val, ok := params[name]
		if !ok {
			errs = append(errs, ValidationError{
				fmt.Sprintf("%s.params.%s", path, name),
				fmt.Sprintf("missing required parameter %q for behaviour %q", name, beh.Name),
			})
			continue
		}

		switch ptype {
		case ParamObjectRef:
			s, ok := val.(string)
			if !ok {
				errs = append(errs, ValidationError{
					fmt.Sprintf("%s.params.%s", path, name),
					fmt.Sprintf("parameter %q must be a string (object reference)", name),
				})
			} else if env.FindObject(s) == nil {
				errs = append(errs, ValidationError{
					fmt.Sprintf("%s.params.%s", path, name),
					fmt.Sprintf("parameter %q references unknown object %q", name, s),
				})
			}
		case ParamInterfaceRef:
			s, ok := val.(string)
			if !ok {
				errs = append(errs, ValidationError{
					fmt.Sprintf("%s.params.%s", path, name),
					fmt.Sprintf("parameter %q must be a string (interface reference)", name),
				})
			} else if env.FindInterface(s) == nil {
				errs = append(errs, ValidationError{
					fmt.Sprintf("%s.params.%s", path, name),
					fmt.Sprintf("parameter %q references unknown interface %q", name, s),
				})
			}
		case ParamString:
			if _, ok := val.(string); !ok {
				errs = append(errs, ValidationError{
					fmt.Sprintf("%s.params.%s", path, name),
					fmt.Sprintf("parameter %q must be a string", name),
				})
			}
		case ParamNumber:
			if _, ok := val.(float64); !ok {
				errs = append(errs, ValidationError{
					fmt.Sprintf("%s.params.%s", path, name),
					fmt.Sprintf("parameter %q must be a number", name),
				})
			}
		case ParamBoolean:
			if _, ok := val.(bool); !ok {
				errs = append(errs, ValidationError{
					fmt.Sprintf("%s.params.%s", path, name),
					fmt.Sprintf("parameter %q must be a boolean", name),
				})
			}
		}
	}

	return errs
}
