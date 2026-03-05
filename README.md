# behtree

A Go library for constructing, validating, and simulating behaviour trees using the PA-BT algorithm (Colledanchise & Ogren). Given action definitions with pre/postconditions and a goal, PA-BT automatically builds provably correct, reactive behaviour trees.

The primary use case is evaluating whether LLMs can select the right actions and ground their parameters from natural-language prompts. The LLM's job is function-calling (pick actions, fill in params); the algorithm handles tree construction.

See [HISTORY.md](HISTORY.md) for the previous approach and why we pivoted.

## How It Works

1. **Define** actions with typed parameters, preconditions, and postconditions in JSON
2. **Specify** a goal as conditions that must hold when the task is complete
3. **PA-BT** builds a reactive behaviour tree by backchaining from the goal
4. **Simulate** the tree to verify correctness across all outcome permutations

The generated trees re-evaluate from the root on every tick, giving automatic recovery if the world state changes unexpectedly.

## Example: Robot Pick-and-Place

A robot must move a wrapper from a table to the bin.

### Environment (`testdata/robot_v2.json`)

```json
{
  "objects": [
    {"name": "wrapper", "fields": {"type": "string", "location": "string"}},
    {"name": "table",   "fields": {"type": "string"}},
    {"name": "bin",     "fields": {"type": "string"}},
    {"name": "robot",   "fields": {"location": "string", "holding": "string"}}
  ],
  "actions": [
    {
      "name": "NavigateTo",
      "type": "Action",
      "params": {"location": "object_ref"},
      "async": true,
      "postconditions": [
        {"object": "robot", "field": "location", "value": "$location"}
      ]
    },
    {
      "name": "PickUp",
      "type": "Action",
      "params": {"object": "object_ref"},
      "preconditions": [
        {"object": "robot", "field": "location", "value": "$object.location"},
        {"object": "robot", "field": "holding", "value": ""}
      ],
      "postconditions": [
        {"object": "robot", "field": "holding", "value": "$object"}
      ]
    },
    {
      "name": "DropIn",
      "type": "Action",
      "params": {"object": "object_ref", "container": "object_ref"},
      "preconditions": [
        {"object": "robot", "field": "holding", "value": "$object"},
        {"object": "robot", "field": "location", "value": "$container"}
      ],
      "postconditions": [
        {"object": "robot", "field": "holding", "value": ""},
        {"object": "$object", "field": "location", "value": "$container"}
      ]
    }
  ],
  "goal": [
    {"object": "wrapper", "field": "location", "value": "bin"}
  ]
}
```

### Condition Templates

Parameters use `$` prefix for references:
- `$param` resolves to a literal object name (e.g., `$location` → `"bin"`)
- `$param.field` becomes a runtime state lookup (e.g., `$object.location` looks up the object's current location at tick time)

### Generated Tree

Running `beht plan testdata/robot_v2.json` produces:

```
Sequence
└── Fallback
    ├── Condition: wrapper.location==bin
    └── Sequence
        ├── Fallback
        │   ├── Condition: robot.holding==wrapper
        │   └── Sequence
        │       ├── Condition: robot.location==wrapper.location
        │       ├── Condition: robot.holding==
        │       └── Action: PickUp(wrapper)
        ├── Fallback
        │   ├── Condition: robot.location==bin
        │   └── Action: NavigateTo(bin)
        └── Action: DropIn(wrapper, bin)
```

PA-BT constructed this by backchaining from the goal `wrapper.location==bin`:

1. To drop the wrapper in the bin, the robot needs to be holding it and be at the bin
2. To pick up the wrapper, the robot needs to be at the wrapper's location and not holding anything
3. Each precondition becomes a Fallback guard: check the condition first, only execute the action if it fails

The tree is reactive — on every tick it re-evaluates from the root, so if the robot drops the wrapper or gets bumped off course, it recovers automatically.

### Tick-by-Tick Walkthrough

| Tick | Robot state | Tree execution | Result |
|------|------------|----------------|--------|
| 1 | Not holding, not at table | `wrapper.location==bin` FAIL → `robot.holding==wrapper` FAIL → `robot.location==wrapper.location` FAIL → needs NavigateTo(table) | RUNNING |
| 2 | At table | `robot.holding==wrapper` FAIL → `robot.location==wrapper.location` OK → `robot.holding==` OK → PickUp(wrapper) OK → `robot.location==bin` FAIL → NavigateTo(bin) | RUNNING |
| 3 | Holding wrapper, at bin | `wrapper.location==bin` FAIL → `robot.holding==wrapper` OK → `robot.location==bin` OK → DropIn(wrapper, bin) OK | SUCCESS |

## Example: Desktop Voice Assistant

A voice assistant for the Sway desktop must open a URL in Firefox. This example demonstrates two-layer composition: a hand-authored outer tree handles the voice interaction loop, while PA-BT generates the task-specific inner tree.

### Inner Tree (PA-BT generated)

The inner tree actions have pre/postconditions. PA-BT builds the tree automatically from the goal.

```json
{
  "objects": [
    {"name": "sway_state", "fields": {"refreshed": "string"}},
    {"name": "firefox", "fields": {"open": "string", "active_page": "string"}}
  ],
  "actions": [
    {
      "name": "QuerySwayTree", "type": "Action",
      "postconditions": [{"object": "sway_state", "field": "refreshed", "value": "true"}]
    },
    {
      "name": "OpenApp", "type": "Action",
      "params": {"app": "object_ref"},
      "preconditions": [{"object": "sway_state", "field": "refreshed", "value": "true"}],
      "postconditions": [{"object": "$app", "field": "open", "value": "true"}]
    },
    {
      "name": "OpenURL", "type": "Action",
      "params": {"app": "object_ref", "url": "string"},
      "preconditions": [{"object": "$app", "field": "open", "value": "true"}],
      "postconditions": [{"object": "$app", "field": "active_page", "value": "$url"}]
    }
  ],
  "goal": [{"object": "firefox", "field": "active_page", "value": "https://github.com/mudler/LocalAI"}]
}
```

Given action selections `[QuerySwayTree, OpenApp(firefox), OpenURL(firefox, URL)]`, PA-BT generates:

```
Sequence
└── Fallback
    ├── Condition: firefox.active_page==https://github.com/mudler/LocalAI
    └── Sequence
        ├── Fallback
        │   ├── Condition: firefox.open==true
        │   └── Sequence
        │       ├── Fallback
        │       │   ├── Condition: sway_state.refreshed==true
        │       │   └── Action: QuerySwayTree
        │       └── Action: OpenApp(firefox)
        └── Action: OpenURL(firefox, https://github.com/mudler/LocalAI)
```

### Outer Tree (hand-authored)

The outer tree is fixed control flow — it handles voice interaction and delegates to the inner tree via `RunTaskTree`:

```
Fallback
├── Sequence [HasTaskTree → RunTaskTree]
├── Sequence [HasPendingUtterance → ProcessUtterance]
├── Sequence [UserSpeaking → WaitForSpeech]
└── WaitForSpeech
```

The inner tree is stored in state at `task_tree.tree`. When `HasTaskTree` succeeds, `RunTaskTree` executes the PA-BT-generated tree. The outer tree stays hand-authored because it's event-driven control flow, not goal-directed planning.

## LLM Integration

In the LLM pipeline, the model doesn't generate tree structure. Instead it selects actions and grounds parameters:

```json
{
  "goal": [{"object": "wrapper", "field": "location", "value": "bin"}],
  "actions": [
    {"name": "NavigateTo", "params": {"location": "table"}},
    {"name": "PickUp", "params": {"object": "wrapper"}},
    {"name": "NavigateTo", "params": {"location": "bin"}},
    {"name": "DropIn", "params": {"object": "wrapper", "container": "bin"}}
  ]
}
```

This is fed into PA-BT which builds the correct tree automatically.

## CLI

```sh
# Build a tree from an environment file
beht plan testdata/robot_v2.json

# Build from environment + explicit action selections
beht plan -actions selections.json testdata/robot_v2.json

# Run LLM benchmarks
beht benchmark -model <model> -provider <provider>

# Re-evaluate saved trees without LLM
beht eval <saved-trees-dir>

# Display environment and tree from JSON files
beht show <file.json>...

# Query trace files
beht trace <trace.jsonl>
```

## Build & Test

```sh
make all       # check-fmt + lint + metrics + test
make test      # go test -v ./...
make lint      # golangci-lint run
make fmt       # auto-format
```

Tests use Ginkgo/Gomega (BDD style).
