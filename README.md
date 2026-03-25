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
        └── Action: DropIn(bin, wrapper)
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

A voice assistant for the Sway desktop must open a URL in Firefox. This example demonstrates two-layer PA-BT composition: an outer tree manages the voice interaction loop (handle speech, run tasks, idle), while inner trees are generated per-task (e.g., open a URL in Firefox).

### Inner Tree (PA-BT generated)

The inner tree actions have pre/postconditions. PA-BT builds the tree automatically from the goal. Objects representing external state (like `firefox`) have an `observed` field — the interpreter resets all `observed` fields to `"false"` at the start of each tick, forcing re-observation. The generic `Observe` action re-synchronizes internal state with reality; its handler can internally choose cheap heuristics (Sway tree query) or expensive perception (screenshot + vision model).

Actions that depend on external state (`OpenApp`, `OpenURL`) have `observed==true` as a precondition. PA-BT backchains this into the tree, placing `Observe` before each action that needs fresh state. With `observed==true` first in the goal, the tree observes reality before checking any conditions — if the goal is already satisfied, no actions run.

```json
{
  "objects": [
    {"name": "sway_state", "fields": {"refreshed": "string"}},
    {"name": "firefox", "fields": {"open": "string", "active_page": "string", "observed": "string"}}
  ],
  "actions": [
    {
      "name": "Observe", "type": "Action",
      "params": {"target": "object_ref"},
      "postconditions": [{"object": "$target", "field": "observed", "value": "true"}]
    },
    {
      "name": "QuerySwayTree", "type": "Action",
      "postconditions": [{"object": "sway_state", "field": "refreshed", "value": "true"}]
    },
    {
      "name": "OpenApp", "type": "Action",
      "params": {"app": "object_ref"},
      "preconditions": [
        {"object": "sway_state", "field": "refreshed", "value": "true"},
        {"object": "$app", "field": "observed", "value": "true"}
      ],
      "postconditions": [
        {"object": "$app", "field": "open", "value": "true"}
      ]
    },
    {
      "name": "OpenURL", "type": "Action",
      "params": {"app": "object_ref", "url": "string"},
      "preconditions": [
        {"object": "$app", "field": "open", "value": "true"},
        {"object": "$app", "field": "observed", "value": "true"}
      ],
      "postconditions": [
        {"object": "$app", "field": "active_page", "value": "$url"}
      ]
    }
  ],
  "goal": [
    {"object": "firefox", "field": "observed", "value": "true"},
    {"object": "firefox", "field": "active_page", "value": "https://github.com/mudler/LocalAI"}
  ]
}
```

Given action selections `[Observe(firefox), QuerySwayTree, OpenApp(firefox), OpenURL(firefox, URL)]`, PA-BT generates:

```
Sequence
├── Fallback
│   ├── Condition: firefox.observed==true
│   └── Action: Observe(firefox)
└── Fallback
    ├── Condition: firefox.active_page==https://github.com/mudler/LocalAI
    └── Sequence
        ├── Fallback
        │   ├── Condition: firefox.open==true
        │   └── Sequence
        │       ├── Fallback
        │       │   ├── Condition: sway_state.refreshed==true
        │       │   └── Action: QuerySwayTree
        │       ├── Fallback
        │       │   ├── Condition: firefox.observed==true
        │       │   └── Action: Observe(firefox)
        │       └── Action: OpenApp(firefox)
        ├── Fallback
        │   ├── Condition: firefox.observed==true
        │   └── Action: Observe(firefox)
        └── Action: OpenURL(firefox, https://github.com/mudler/LocalAI)
```

The tree observes first, then checks conditions with fresh state. If Firefox is already on the right page, the top-level condition passes and no actions run. If not, `Observe` appears as a precondition guard before each action (OpenApp, OpenURL), ensuring the tree always acts on observed reality rather than stale cached state.

### Outer Tree (PA-BT generated)

The outer tree is also PA-BT generated. Its goal is `speech.observed==true` then `system.idle==true` — meaning speech has been observed (fresh state) and all work completed. Like the inner tree, `speech` has an `observed` field that resets each tick, and `Observe(speech)` is a precondition on every action that depends on speech state.

The tree must be built from worst-case initial state (`speech.active="true"`, `task_tree.pending="true"`) so PA-BT expands all fallback branches. At runtime, conditions that are already satisfied are simply checked and skipped.

```json
{
  "objects": [
    {"name": "speech", "fields": {"active": "string", "observed": "string"}},
    {"name": "task_tree", "fields": {"pending": "string", "tree": "object"}},
    {"name": "system", "fields": {"idle": "string"}}
  ],
  "actions": [
    {
      "name": "Observe", "type": "Action",
      "params": {"target": "object_ref"},
      "postconditions": [{"object": "$target", "field": "observed", "value": "true"}]
    },
    {
      "name": "HandleSpeech", "type": "Action", "async": true,
      "preconditions": [{"object": "speech", "field": "observed", "value": "true"}],
      "postconditions": [{"object": "speech", "field": "active", "value": "false"}]
    },
    {
      "name": "RunTaskTree", "type": "Action", "async": true,
      "params": {"tree_variable": "string"},
      "preconditions": [
        {"object": "speech", "field": "active", "value": "false"},
        {"object": "speech", "field": "observed", "value": "true"}
      ],
      "postconditions": [{"object": "task_tree", "field": "pending", "value": "false"}]
    },
    {
      "name": "Idle", "type": "Action",
      "preconditions": [
        {"object": "speech", "field": "active", "value": "false"},
        {"object": "task_tree", "field": "pending", "value": "false"},
        {"object": "speech", "field": "observed", "value": "true"}
      ],
      "postconditions": [{"object": "system", "field": "idle", "value": "true"}]
    }
  ],
  "goal": [
    {"object": "speech", "field": "observed", "value": "true"},
    {"object": "system", "field": "idle", "value": "true"}
  ]
}
```

Given action selections `[Observe(speech), HandleSpeech, RunTaskTree(task_tree.tree), Idle]`, PA-BT generates:

```
Sequence
├── Fallback
│   ├── Condition: speech.observed==true
│   └── Action: Observe(speech)
└── Fallback
    ├── Condition: system.idle==true
    └── Sequence
        ├── Fallback
        │   ├── Condition: speech.active==false
        │   └── Sequence
        │       ├── Fallback
        │       │   ├── Condition: speech.observed==true
        │       │   └── Action: Observe(speech)
        │       └── Action: HandleSpeech
        ├── Fallback
        │   ├── Condition: task_tree.pending==false
        │   └── Sequence
        │       ├── Fallback
        │       │   ├── Condition: speech.active==false
        │       │   └── Sequence
        │       │       ├── Fallback
        │       │       │   ├── Condition: speech.observed==true
        │       │       │   └── Action: Observe(speech)
        │       │       └── Action: HandleSpeech
        │       ├── Fallback
        │       │   ├── Condition: speech.observed==true
        │       │   └── Action: Observe(speech)
        │       └── Action: RunTaskTree(task_tree.tree)
        ├── Fallback
        │   ├── Condition: speech.observed==true
        │   └── Action: Observe(speech)
        └── Action: Idle
```

The tree observes speech first, then checks conditions with fresh state. `Observe(speech)` appears as a precondition guard before every action that depends on speech state — HandleSpeech, RunTaskTree, and Idle all require knowing whether speech is active.

The interpreter resets ephemeral fields (`observed`, `idle`) at each tick start, so both `speech.observed` and `system.idle` are always re-evaluated. The inner task tree is stored in state at `task_tree.tree` and executed by `RunTaskTree` as a dynamic subtree.

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
