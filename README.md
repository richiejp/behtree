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

## Desktop assistant

The user asks a Sway Desktop (the tiling window manager on Linux) voice assistant to go to LocalAI's github page. 

The desktop assistant uses the OpenAI realtime API to accept voice commands. There are two parts to the behaviour tree, an overall fixed piece of control flow and a potentially dynamic part. The fixed piece is that if the user is speaking or has spoken, then the tree should be blocked waiting for a response.

The response can contain the second part of the behaviour tree that runs to complete the specified task of going to the github page. The assistant has behaviours to find what windows are active on which desktop (i.e. it can use the output of `swaymsg -t get_tree` with a prompt to another LLM to find if FireFox is already active somewhere or is even open at the requested page). It has a behaviour to switch to a given workspace and window and one to open applications like FireFox. Finally it has a behaviour to ensure a page is open in FireFox.

Some of these behaviours might be expensive to execute, but for now we can just assume it's OK to keep executing them on each tick to make sure they have been done.

### Available behaviours

**Conditions** (no side effects):
- `UserSpeaking` — is the user currently speaking into the microphone?
- `HasPendingUtterance` — has the user finished speaking and we have an unprocessed utterance?
- `HasTaskTree` — has the LLM responded with a task behaviour tree to execute?
- `IsAppOpen` — is a given application open anywhere in the Sway tree? (queries `swaymsg -t get_tree`)
- `IsPageOpen` — is a given URL open in a browser tab? (queries Sway tree + browser state)
- `IsFocused` — is a given window currently focused?

**Actions** (may have side effects):
- `WaitForSpeech` — block until user speech begins or a timeout elapses (async, returns RUNNING while waiting)
- `ProcessUtterance` — send the transcribed utterance to an LLM and receive a task behaviour tree as the response (async, returns RUNNING while waiting for the LLM)
- `QuerySwayTree` — run `swaymsg -t get_tree` and parse the result into a window list, storing it in the blackboard
- `FocusWindow` — switch to the workspace and focus the window matching a given app/title
- `OpenApp` — launch an application by name (e.g. `firefox`)
- `OpenURL` — open a URL in the browser (e.g. via `firefox --new-tab <url>` or focusing an existing tab)

### Behaviour tree design

There are two layers: a **fixed outer tree** that handles the voice interaction loop, and a **dynamic inner tree** that is generated by the LLM to fulfil a specific request.

#### Fixed outer tree

The outer tree runs on every tick. Its job is:

1. If the user is speaking, wait for them to finish.
2. If there is a pending utterance, send it to the LLM and wait for a task tree.
3. If there is a task tree, execute it.
4. Otherwise, idle — wait for the next speech event.

Using backchaining from the goal "execute the user's request":

1. **Goal**: execute the task tree → `RunTaskTree()` — but only if we have one
2. **Precondition for task tree**: process the utterance → `ProcessUtterance()` — but only if there is a pending utterance
3. **Precondition for utterance**: wait for speech → `WaitForSpeech()`

```
Fallback
├── Sequence
│   ├── Condition: HasTaskTree
│   └── Action: RunTaskTree
├── Sequence
│   ├── Condition: HasPendingUtterance
│   └── Action: ProcessUtterance
├── Sequence
│   ├── Condition: UserSpeaking
│   └── Action: WaitForSpeech
└── Action: WaitForSpeech
```

The Fallback tries each branch in order: if we already have a task tree, run it. Otherwise, if we have a pending utterance, process it. Otherwise, if the user is speaking, wait for them to finish. Finally, if nothing else is happening, idle waiting for speech. This makes the tree reactive — if the user starts speaking while a task is running, the task tree is discarded on the next tick (since `HasTaskTree` would be cleared by the new utterance flow).

#### Dynamic inner tree (generated by the LLM)

For the request "go to LocalAI's github page", the LLM generates a task tree. Using backchaining:

1. **Goal**: LocalAI's github page is open and focused → `OpenURL("https://github.com/mudler/LocalAI")` — but only if Firefox is running
2. **Precondition for OpenURL**: Firefox must be open → `OpenApp("firefox")`
3. **Precondition for focus**: query the current Sway state → `QuerySwayTree()`

```
Sequence
├── Action: QuerySwayTree
├── Fallback
│   ├── Condition: IsAppOpen("firefox")
│   └── Action: OpenApp("firefox")
├── Fallback
│   ├── Sequence
│   │   ├── Condition: IsPageOpen("https://github.com/mudler/LocalAI")
│   │   └── Action: FocusWindow("firefox", "LocalAI")
│   └── Action: OpenURL("https://github.com/mudler/LocalAI")
└── Fallback
    ├── Condition: IsFocused("firefox", "LocalAI")
    └── Action: FocusWindow("firefox", "LocalAI")
```

The tree first refreshes the window state, ensures Firefox is open, ensures the page is open (or opens it), and finally ensures the correct tab is focused. Because there are no memory nodes, every tick re-evaluates from the root, so if Firefox is closed externally, the tree will reopen it.

### Object definitions

```json
{
  "objects": [
    {
      "name": "utterance",
      "fields": {
        "text": "string",
        "timestamp": "number",
        "processed": "boolean"
      }
    },
    {
      "name": "task_tree",
      "fields": {
        "tree": "object",
        "source_utterance": "string"
      }
    },
    {
      "name": "sway_state",
      "fields": {
        "windows": "object"
      }
    },
    {
      "name": "target_url",
      "fields": {
        "url": "string",
        "title": "string"
      }
    }
  ],
  "interfaces": [
    {
      "name": "RealtimeAPI",
      "description": "OpenAI realtime voice API connection"
    },
    {
      "name": "SwayIPC",
      "description": "Sway IPC socket for querying and controlling windows"
    },
    {
      "name": "LLM",
      "description": "LLM endpoint used to convert utterances into task behaviour trees"
    }
  ]
}
```

### Behaviour tree JSON

#### Fixed outer tree

```json
{
  "type": "Fallback",
  "children": [
    {
      "type": "Sequence",
      "children": [
        {
          "type": "Condition",
          "name": "HasTaskTree"
        },
        {
          "type": "Action",
          "name": "RunTaskTree"
        }
      ]
    },
    {
      "type": "Sequence",
      "children": [
        {
          "type": "Condition",
          "name": "HasPendingUtterance"
        },
        {
          "type": "Action",
          "name": "ProcessUtterance"
        }
      ]
    },
    {
      "type": "Sequence",
      "children": [
        {
          "type": "Condition",
          "name": "UserSpeaking"
        },
        {
          "type": "Action",
          "name": "WaitForSpeech"
        }
      ]
    },
    {
      "type": "Action",
      "name": "WaitForSpeech"
    }
  ]
}
```

#### Dynamic inner tree (LLM-generated for "go to LocalAI's github page")

```json
{
  "type": "Sequence",
  "children": [
    {
      "type": "Action",
      "name": "QuerySwayTree"
    },
    {
      "type": "Fallback",
      "children": [
        {
          "type": "Condition",
          "name": "IsAppOpen",
          "params": {
            "app": "firefox"
          }
        },
        {
          "type": "Action",
          "name": "OpenApp",
          "params": {
            "app": "firefox"
          }
        }
      ]
    },
    {
      "type": "Fallback",
      "children": [
        {
          "type": "Sequence",
          "children": [
            {
              "type": "Condition",
              "name": "IsPageOpen",
              "params": {
                "url": "https://github.com/mudler/LocalAI"
              }
            },
            {
              "type": "Action",
              "name": "FocusWindow",
              "params": {
                "app": "firefox",
                "title": "LocalAI"
              }
            }
          ]
        },
        {
          "type": "Action",
          "name": "OpenURL",
          "params": {
            "url": "https://github.com/mudler/LocalAI"
          }
        }
      ]
    },
    {
      "type": "Fallback",
      "children": [
        {
          "type": "Condition",
          "name": "IsFocused",
          "params": {
            "app": "firefox",
            "title": "LocalAI"
          }
        },
        {
          "type": "Action",
          "name": "FocusWindow",
          "params": {
            "app": "firefox",
            "title": "LocalAI"
          }
        }
      ]
    }
  ]
}
```

### Tick-by-tick walkthrough

Starting state: the user is idle, no task tree exists, Firefox is open on workspace 2 but showing a different page, and the user is currently on workspace 1.

| Tick | System state | Tree execution | Result |
|------|-------------|----------------|--------|
| 1 | Idle, no speech | `HasTaskTree` → FAILURE → `HasPendingUtterance` → FAILURE → `UserSpeaking` → FAILURE → `WaitForSpeech` → RUNNING | RUNNING |
| 2 | User starts speaking | `HasTaskTree` → FAILURE → `HasPendingUtterance` → FAILURE → `UserSpeaking` → SUCCESS → `WaitForSpeech` → RUNNING | RUNNING |
| 3 | User still speaking | `HasTaskTree` → FAILURE → `HasPendingUtterance` → FAILURE → `UserSpeaking` → SUCCESS → `WaitForSpeech` → RUNNING | RUNNING |
| 4 | User finished: "go to LocalAI's github page" | `HasTaskTree` → FAILURE → `HasPendingUtterance` → SUCCESS → `ProcessUtterance` → RUNNING | RUNNING |
| 5 | LLM returns task tree | `HasTaskTree` → SUCCESS → `RunTaskTree` → (enters dynamic tree) → `QuerySwayTree` → SUCCESS → `IsAppOpen("firefox")` → SUCCESS → `IsPageOpen(url)` → FAILURE → `OpenURL(url)` → RUNNING | RUNNING |
| 6 | Page loading in Firefox | `HasTaskTree` → SUCCESS → `RunTaskTree` → `QuerySwayTree` → SUCCESS → `IsAppOpen("firefox")` → SUCCESS → `IsPageOpen(url)` → FAILURE → `OpenURL(url)` → RUNNING | RUNNING |
| 7 | Page loaded, wrong workspace | `HasTaskTree` → SUCCESS → `RunTaskTree` → `QuerySwayTree` → SUCCESS → `IsAppOpen("firefox")` → SUCCESS → `IsPageOpen(url)` → SUCCESS → `FocusWindow("firefox", "LocalAI")` → SUCCESS → `IsFocused("firefox", "LocalAI")` → SUCCESS | SUCCESS |

After tick 7, the task tree is complete. On the next tick, `HasTaskTree` is cleared and the assistant returns to idle, waiting for the next voice command.

Note the reactivity: if the user had spoken again during ticks 5–6 (e.g. "never mind"), the new utterance would clear the task tree. On the next tick `HasTaskTree` would return FAILURE, and the outer tree would fall through to `HasPendingUtterance` → SUCCESS → `ProcessUtterance`, starting the new request.
