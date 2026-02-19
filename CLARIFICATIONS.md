# Clarification Questions

## 1. JSON format: single document or separate files?

The README shows object definitions and the tree as separate JSON blocks. Should the input be a single JSON document containing `objects`, `interfaces`, `behaviours`, and `tree` together? Or separate files?

Leaning toward a single document since it's what an LLM would produce in one shot.

Answer: There should be the option of using separate files or putting them all in the same file. eventually they should all be merged into the same structure but the LLM is only expected to write part of the tree. The definitions for objects, interfaces and behaviours are part of the environment. It's also possible that we'll want to compose Different behaviour trees.

## 2. Behaviour registry — where do the definitions come from?

Validation checks that leaf nodes reference behaviours that "exist." For the interpreter/simulator, we need to know:
- What conditions and actions are available
- What params each one accepts
- What side effects actions have on variables

Should the behaviour definitions (name, params schema, whether condition/action) be part of the JSON input document? Something like:

```json
"behaviours": [
  {"name": "IsHolding", "type": "Condition", "params": {"object": "object_ref"}},
  {"name": "NavigateTo", "type": "Action", "params": {"location": "object_ref"}, "async": true}
]
```

Answer: Yes these should be defined as part of the environment. You should be able to merge Multiple definitions of behaviours, objects and interfaces into a single environment for the LLM to create a behaviour tree in. In some cases part of the behaviour tree may already be defined and the LLM is filling in a leaf node within that existing tree.

## 3. Simulation semantics — how do actions change state?

For the simulation to actually run and verify "the ending state of the variables indicates the correct actions were taken," we need to know what each action *does*. Options:

- **(a) Declarative effects** — each behaviour declares its preconditions and postconditions in the JSON (e.g. `PickUp` sets `robot.holding = object`, requires `robot.location == object.location`)
- **(b) Go-native handlers** — behaviours are registered as Go functions in a `BehaviourRegistry` that the host program provides. The JSON just references names, and the Go code supplies the logic
- **(c) Both** — JSON describes effects for LLM-generated/simulated trees; Go handlers for real execution

(b) is simplest for now and most flexible; (a) would be needed for LLM-verifiable trees without Go code.

Answer: for now (b)

## 4. Async actions in simulation

Actions like `NavigateTo` return `RUNNING` for multiple ticks. In simulation, should we:

- **(a)** Model a tick counter (action returns RUNNING for N ticks, then SUCCESS)?
- **(b)** Simplify and have all actions complete in one tick for validation purposes?
- **(c)** Let the Go handler decide (it tracks internal state across ticks)?

Answer: the simulation harness should be able to request to the Go handler whether it should complete immediately with success, whether it should fail or return running for one tick. The Go handler will have to check if the global state is compatible with this request and if so it may have to update the state. If it is not compatible then it can signal this to the simulation environment and the harness can skip this scenario.

Depending on the size  of the tree we can systematically request every outcome from behaviour nodes.

## 5. "Benchmark" — what do you mean?

You said "interpreter, simulation and benchmark." Benchmark could mean:

- **(a)** Go `testing.B` benchmarks measuring parse/validate/tick performance
- **(b)** Benchmarking LLM-generated trees against expected outcomes (accuracy metrics)
- **(c)** Both

Answer: The benchmarking is of LLMs. So we test each LLM to see if it can produce trees of increasing complexity.

## 6. Decorator nodes

The README theory section describes decorators (Inverter, RetryUntilSuccessful, ForceSuccess, ForceFailure) but neither example uses them. Should we implement them now or stub the types?

Answer: stub them

## 7. Memory variants

The README mentions control nodes with/without memory. Should we implement both `Sequence`/`SequenceWithMemory` and `Fallback`/`FallbackWithMemory` now, or just the no-memory variants from the examples?

Answer: just use no-memory

## 8. RunTaskTree — dynamic subtree insertion

The desktop assistant example has `RunTaskTree` which executes a dynamically-generated subtree. Should the interpreter support this (a node that runs a tree stored in a variable), or defer this?

Answer: It should support this
