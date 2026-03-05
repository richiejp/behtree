package behtree

import "fmt"

// GroundedAction is an action definition with all parameter references resolved.
type GroundedAction struct {
	Name           string
	Params         Params
	Preconditions  []ResolvedCondition
	Postconditions []ResolvedCondition
	Async          bool
}

// BuildResult holds the tree and registry produced by BuildTree.
type BuildResult struct {
	Tree     *Node
	Registry *ActionRegistry
}

// BuildTree constructs a behaviour tree using the PA-BT algorithm
// (Colledanchise & Ogren). Given a goal (set of conditions), grounded
// actions, and initial state, it expands conditions into fallback
// subtrees until the tree succeeds or no more expansions are possible.
func BuildTree(goal []ResolvedCondition, actions []GroundedAction, state *State) (*BuildResult, error) {
	if len(goal) == 0 {
		return nil, fmt.Errorf("empty goal")
	}

	b := &builder{
		registry: NewActionRegistry(),
		condMap:  make(map[string]ResolvedCondition),
		actions:  actions,
	}

	// Build initial tree: Sequence of goal conditions
	root := &Node{Type: SequenceNode}
	for _, g := range goal {
		condName := g.Name()
		root.Children = append(root.Children, &Node{
			Type: ConditionNode,
			Name: condName,
		})
		b.registerCondition(condName, g)
	}

	const maxIterations = 100
	for range maxIterations {
		failed := b.findDeepestFailedCondition(root, state)
		if failed == nil {
			return &BuildResult{Tree: root, Registry: b.registry}, nil
		}

		action, err := b.findSatisfyingAction(failed.cond, state)
		if err != nil {
			return nil, fmt.Errorf("cannot satisfy condition %q: %w", failed.condName, err)
		}

		expansion := b.buildExpansion(action)
		replaceChild(failed.parent, failed.childIdx, expansion)
		resolveConflicts(failed.parent, failed.childIdx, action, b.condMap)
	}

	return &BuildResult{Tree: root, Registry: b.registry}, nil
}

type builder struct {
	registry *ActionRegistry
	condMap  map[string]ResolvedCondition
	actions  []GroundedAction
}

func (b *builder) registerCondition(name string, cond ResolvedCondition) {
	b.condMap[name] = cond
	cond2 := cond // capture for closure
	b.registry.Register(name, func(_ Params, state *State, _ OutcomeRequest) HandlerResult {
		ok, err := cond2.Evaluate(state)
		if err != nil {
			return HandlerResult{Status: Failure, Compatible: true}
		}
		if ok {
			return HandlerResult{Status: Success, Compatible: true}
		}
		return HandlerResult{Status: Failure, Compatible: true}
	})
}

type failedCondInfo struct {
	parent   *Node
	childIdx int
	condName string
	cond     ResolvedCondition
}

func (b *builder) findDeepestFailedCondition(root *Node, state *State) *failedCondInfo {
	type queueItem struct {
		node     *Node
		parent   *Node
		idx      int
		depth    int
		expanded bool // true if this node is first child of a Fallback (already expanded)
	}

	queue := []queueItem{{node: root, depth: 0}}
	var deepest *failedCondInfo
	deepestDepth := -1

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		// A condition that's the first child of a Fallback is already expanded
		if item.node.Type == ConditionNode && !item.expanded {
			handler, err := b.registry.Get(item.node.Name)
			if err != nil {
				continue
			}
			result := handler(item.node.Params, state, RequestSuccess)
			if result.Status == Failure && item.depth > deepestDepth {
				cond := b.condMap[item.node.Name]
				deepest = &failedCondInfo{
					parent:   item.parent,
					childIdx: item.idx,
					condName: item.node.Name,
					cond:     cond,
				}
				deepestDepth = item.depth
			}
		}

		for i, child := range item.node.Children {
			isExpanded := item.node.Type == FallbackNode && i == 0
			queue = append(queue, queueItem{
				node:     child,
				parent:   item.node,
				idx:      i,
				depth:    item.depth + 1,
				expanded: isExpanded,
			})
		}
	}

	return deepest
}

func (b *builder) findSatisfyingAction(target ResolvedCondition, state *State) (*GroundedAction, error) {
	for i := range b.actions {
		for _, post := range b.actions[i].Postconditions {
			if matchesWithState(post, target, state) {
				return &b.actions[i], nil
			}
		}
	}
	return nil, fmt.Errorf("no action has postcondition matching %s", target.Name())
}

// matchesWithState checks if a postcondition satisfies a target condition.
// If the target has a StateRef, it evaluates it against state to get the
// literal value for comparison.
func matchesWithState(post, target ResolvedCondition, state *State) bool {
	if post.Object != target.Object || post.Field != target.Field {
		return false
	}

	// Get the effective target value
	var targetVal string
	if target.ValueRef != nil {
		val, err := state.Get(target.ValueRef.Object, target.ValueRef.Field)
		if err != nil {
			return false
		}
		targetVal = fmt.Sprintf("%v", val)
	} else {
		targetVal = target.Value
	}

	// Get the effective post value
	var postVal string
	if post.ValueRef != nil {
		val, err := state.Get(post.ValueRef.Object, post.ValueRef.Field)
		if err != nil {
			return false
		}
		postVal = fmt.Sprintf("%v", val)
	} else {
		postVal = post.Value
	}

	return postVal == targetVal
}

func (b *builder) buildExpansion(action *GroundedAction) *Node {
	actionNode := &Node{
		Type:   ActionNode,
		Name:   action.Name,
		Params: action.Params,
	}

	if len(action.Preconditions) == 0 {
		return &Node{
			Type: FallbackNode,
			Children: []*Node{
				nil, // placeholder for the original condition
				actionNode,
			},
		}
	}

	seq := &Node{Type: SequenceNode}
	for _, pre := range action.Preconditions {
		condName := pre.Name()
		seq.Children = append(seq.Children, &Node{
			Type: ConditionNode,
			Name: condName,
		})
		b.registerCondition(condName, pre)
	}
	seq.Children = append(seq.Children, actionNode)

	return &Node{
		Type: FallbackNode,
		Children: []*Node{
			nil, // placeholder for the original condition
			seq,
		},
	}
}

func replaceChild(parent *Node, idx int, expansion *Node) {
	if parent == nil {
		return
	}
	original := parent.Children[idx]
	expansion.Children[0] = original
	parent.Children[idx] = expansion
}

func resolveConflicts(parent *Node, expansionIdx int, action *GroundedAction, condMap map[string]ResolvedCondition) {
	if parent == nil || parent.Type != SequenceNode {
		return
	}

	for i := 0; i < expansionIdx; i++ {
		siblingConds := extractConditions(parent.Children[i], condMap)
		for _, sc := range siblingConds {
			for _, post := range action.Postconditions {
				if post.Contradicts(sc) {
					moveChildLeft(parent, expansionIdx, i)
					return
				}
			}
		}
	}
}

func extractConditions(n *Node, condMap map[string]ResolvedCondition) []ResolvedCondition {
	if n.Type == ConditionNode {
		if cond, ok := condMap[n.Name]; ok {
			return []ResolvedCondition{cond}
		}
		return nil
	}
	if n.Type == FallbackNode && len(n.Children) > 0 {
		return extractConditions(n.Children[0], condMap)
	}
	return nil
}

func moveChildLeft(parent *Node, from, to int) {
	child := parent.Children[from]
	copy(parent.Children[to+1:from+1], parent.Children[to:from])
	parent.Children[to] = child
}

// GroundAction resolves an ActionDef's condition templates against concrete params.
func GroundAction(def *ActionDef, params Params) (*GroundedAction, error) {
	ga := &GroundedAction{
		Name:   def.Name,
		Params: params,
		Async:  def.Async,
	}

	for _, pre := range def.Preconditions {
		rc, err := pre.Resolve(params)
		if err != nil {
			return nil, fmt.Errorf("resolve precondition for %s: %w", def.Name, err)
		}
		ga.Preconditions = append(ga.Preconditions, rc)
	}

	for _, post := range def.Postconditions {
		rc, err := post.Resolve(params)
		if err != nil {
			return nil, fmt.Errorf("resolve postcondition for %s: %w", def.Name, err)
		}
		ga.Postconditions = append(ga.Postconditions, rc)
	}

	return ga, nil
}

// GroundActionSelection resolves a list of selected actions with their params.
type ActionSelection struct {
	Name   string `json:"name"`
	Params Params `json:"params"`
}

// GroundActions resolves a list of action selections against environment definitions.
func GroundActions(env *Environment, selections []ActionSelection) ([]GroundedAction, error) {
	var grounded []GroundedAction
	for _, sel := range selections {
		def := env.FindAction(sel.Name)
		if def == nil {
			return nil, fmt.Errorf("unknown action %q", sel.Name)
		}
		ga, err := GroundAction(def, sel.Params)
		if err != nil {
			return nil, err
		}
		grounded = append(grounded, *ga)
	}
	return grounded, nil
}

// ResolveGoal resolves template conditions from the environment goal.
func ResolveGoal(goal []Condition) ([]ResolvedCondition, error) {
	var resolved []ResolvedCondition
	for _, c := range goal {
		rc, err := c.Resolve(Params{})
		if err != nil {
			return nil, fmt.Errorf("resolve goal condition: %w", err)
		}
		resolved = append(resolved, rc)
	}
	return resolved, nil
}
