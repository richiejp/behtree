# Thesis

- We can use VLMs (or LLMs) to generate verifiable and reactive scripts using behaviour trees.
- These are a safer and simpler alternative to writing code in a full programming language
  - safe because only predefined behaviours can be used
  - simpler because the logic for a reactive system in embedded in the BT implementation
  - behaviour trees are easier to statically analyse than a full programming language
- The behaviour tree can be defined as JSON
  - The JSON itself can be described with jsonschema the same as too call parameters

# Theory

## Ticks

- Root node receives a tick from an external source
  - e.g. timer, every N simulation ticks
- Tick triggers a callback that returns SUCCESS, FAILURE, RUNNING
  - `tick() -> {SUCCESS, FAILURE, RUNNING}`
  - `tick()` can have side effects
- Nodes propagate ticks to their children from their `tick()`
  - Can choose whether to tick one or more of their children
  - Can choose the number of times to tick their children
- Every node's `tick()` is called in the same thread/process, but `tick()` may spawn long running processes
  - Unless a parallel node is used which ticks its children in parallel
- Nodes can be ticked again after returning SUCCESS or FAILURE and may return a new result

## Node types

- TreeNode
  - TrunkNode
    - Decorator
      - One child
      - e.g Alters result of child, ticks multiple times
        - Inverter
        - RetryUntilSuccessful
        - ForceSuccess, ForceFailure
    - ControlNode
      - 1..N children
      - Children are ordered
      - Memory
        - No memory: reevaluates every child
        - Memory: caches child results
      - e.g. Ticks children based on its state and the state of its siblings
        - Sequence
          - Tick each child in order until one returns FAILURE or all children have completed
        - Fallback
          - Tick each child until one returns SUCCESS
        - Parallel
          - Tick each child in parallel, return SUCCESS if child success rate above threshold
        - Utility Fallback
          - Like Fallback, but...
          - Order the children in terms of expected payoff (utility)
  - LeafNode
    - ConditionNode
      - Reads state and can be parameterized with Object/Actor references, constants and variable names 
      - Synchronously evaluates some state with no side effects
      - `tick() -> {SUCCESS, FAILURE}`: SUCCESS (true) or FAILURE (false)
    - ActionNode
      - Reads and writes state and can be parameterized with Object/Actor references and constants
      - Synchronous
        - `tick()` only returns SUCCESS or FAILURE
      - Asynchronous
        - `tick()` can return RUNNING, SUCCESS or FAILURE

## Variables

- Top level variables are all Object's described as JSON
  - defintion:
    - Name: ID of the object
    - Fields: JSON object describing the fields
  - all objects and interfaces are declared in the global scope and refered to by reference
- Types
  - object, string, number, boolean and interface
  - interface types are references to an object that complies with a particular Go interface these are opaque to the BT

## Designing BTs

- Backchaining: Start with the goal to maximise reactivity

# Examples

## Robot

The user asks a robot to move a wrapper from a table to the bin. The robot is currently stood in sight of the table, but not close enough to pick up the wrapper. It has behaviours for navigation, picking up items and dropping items.

### Available behaviours

**Conditions** (no side effects):
- `IsHolding` — is the robot holding a given object?
- `IsAt` — is the robot at a given location?

**Actions** (may have side effects):
- `NavigateTo` — move to a location (async, returns RUNNING while moving)
- `PickUp` — pick up an object at the robot's current location
- `DropIn` — drop the currently held object into a container

### Behaviour tree design

Using backchaining, we start from the goal and work backwards:

1. **Goal**: wrapper is in the bin → `DropIn(wrapper, bin)` — but only works if we're at the bin holding the wrapper
2. **Precondition for drop**: be at the bin → `NavigateTo(bin)` — but only if not already there
3. **Precondition for navigate-to-bin**: be holding the wrapper → `PickUp(wrapper)` — but only works at the table
4. **Precondition for pickup**: be at the table → `NavigateTo(table)`

This gives us a tree that is reactive: on every tick the tree re-evaluates from the root, so if the robot is bumped off course or drops the wrapper, it recovers automatically.

```
Sequence
├── Fallback
│   ├── Condition: IsHolding(wrapper)
│   └── Sequence
│       ├── Fallback
│       │   ├── Condition: IsAt(table)
│       │   └── Action: NavigateTo(table)
│       └── Action: PickUp(wrapper)
├── Fallback
│   ├── Condition: IsAt(bin)
│   └── Action: NavigateTo(bin)
└── Action: DropIn(wrapper, bin)
```

### Object definitions

```json
{
  "objects": [
    {
      "name": "wrapper",
      "fields": {
        "type": "string",
        "location": "string"
      }
    },
    {
      "name": "table",
      "fields": {
        "type": "string"
      }
    },
    {
      "name": "bin",
      "fields": {
        "type": "string"
      }
    }
  ]
}
```

### Behaviour tree JSON

```json
{
  "type": "Sequence",
  "children": [
    {
      "type": "Fallback",
      "children": [
        {
          "type": "Condition",
          "name": "IsHolding",
          "params": {
            "object": "wrapper"
          }
        },
        {
          "type": "Sequence",
          "children": [
            {
              "type": "Fallback",
              "children": [
                {
                  "type": "Condition",
                  "name": "IsAt",
                  "params": {
                    "location": "table"
                  }
                },
                {
                  "type": "Action",
                  "name": "NavigateTo",
                  "params": {
                    "location": "table"
                  }
                }
              ]
            },
            {
              "type": "Action",
              "name": "PickUp",
              "params": {
                "object": "wrapper"
              }
            }
          ]
        }
      ]
    },
    {
      "type": "Fallback",
      "children": [
        {
          "type": "Condition",
          "name": "IsAt",
          "params": {
            "location": "bin"
          }
        },
        {
          "type": "Action",
          "name": "NavigateTo",
          "params": {
            "location": "bin"
          }
        }
      ]
    },
    {
      "type": "Action",
      "name": "DropIn",
      "params": {
        "object": "wrapper",
        "container": "bin"
      }
    }
  ]
}
```

### Tick-by-tick walkthrough

| Tick | Robot state | Tree execution | Result |
|------|------------|----------------|--------|
| 1 | Not holding wrapper, not at table | `IsHolding(wrapper)` → FAILURE → `IsAt(table)` → FAILURE → `NavigateTo(table)` → RUNNING | RUNNING |
| 2 | Moving to table | `IsHolding(wrapper)` → FAILURE → `IsAt(table)` → FAILURE → `NavigateTo(table)` → RUNNING | RUNNING |
| 3 | At table | `IsHolding(wrapper)` → FAILURE → `IsAt(table)` → SUCCESS → `PickUp(wrapper)` → SUCCESS → `IsAt(bin)` → FAILURE → `NavigateTo(bin)` → RUNNING | RUNNING |
| 4 | Holding wrapper, moving to bin | `IsHolding(wrapper)` → SUCCESS → `IsAt(bin)` → FAILURE → `NavigateTo(bin)` → RUNNING | RUNNING |
| 5 | Holding wrapper, at bin | `IsHolding(wrapper)` → SUCCESS → `IsAt(bin)` → SUCCESS → `DropIn(wrapper, bin)` → SUCCESS | SUCCESS |
