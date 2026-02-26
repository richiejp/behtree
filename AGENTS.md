# AGENTS.md

## Project Overview

**behtree** is a Go library for defining, validating, interpreting, and simulating behaviour trees. Its primary purpose is evaluating whether LLMs can generate correct behaviour trees from natural-language prompts. Trees and their environments (objects, interfaces, behaviours) are defined in JSON and executed by a tree-walking interpreter backed by Go handler functions.

## Architecture

```
document.go          Parse/load JSON documents
environment.go       Type system: objects, interfaces, behaviour definitions
  ├─ registry.go     Runtime handler registry (Go functions per behaviour)
  ├─ state.go        Runtime state (object → field → value)
  └─ validate.go     Static validation of environment + tree structure
behtree.go           Core types: Node, NodeType, Status, Params
interpreter.go       Tree-walking tick interpreter
  └─ runtasktree.go  Dynamic subtree execution (RunTaskTree)
harness.go           Exhaustive simulation (all outcome combinations)
benchmark.go         LLM benchmarking framework
print.go             ASCII tree pretty-printer
```

### Key types

| Type | File | Purpose |
|------|------|---------|
| `Node` | behtree.go | Tree node with type, name, params, children |
| `Environment` | environment.go | Merged definitions: objects, interfaces, behaviours, tree |
| `Document` | document.go | Single JSON file; merges into Environment |
| `BehaviourRegistry` | registry.go | Maps behaviour names → Go handler functions |
| `State` | state.go | Runtime variable store (object.field = value) |
| `Interpreter` | interpreter.go | Ticks a tree against registry + state |
| `SimulationHarness` | harness.go | Runs all outcome permutations for a tree |
| `BenchmarkSuite` | benchmark.go | Evaluates LLM tree generation quality |

### Node types

- **Control**: `Sequence` (all-must-succeed), `Fallback` (first-success-wins)
- **Leaf**: `Condition` (read-only, no RUNNING), `Action` (side effects, may RUNNING)
- **Decorator** (stubbed): `Inverter`, `ForceSuccess`, `ForceFailure`, `RetryUntilSuccessful`

No memory variants — nodes always re-evaluate from the start on each tick.

## Build & Test

```sh
make test          # go test -v ./...
make lint          # golangci-lint run
make check-fmt     # verify gofmt
make all           # check-fmt + lint + metrics + test
make fmt           # auto-format
```

Tests use **Ginkgo/Gomega** (BDD style). Run a single test with:

```sh
go test -v -run "TestBehtree" ./...
```

## Code Conventions

- **Go 1.25**, single-package `behtree` (no sub-packages)
- JSON is the primary serialization format; `Node` has custom `UnmarshalJSON`
- Handler signature: `func(Params, *State, OutcomeRequest) HandlerResult`
  - `OutcomeRequest` enables simulation: handlers check if a requested outcome is compatible with current state
- `Environment.Merge()` combines multiple document definitions additively
- Validation is static and separate from interpretation — call `Validate(env)` before ticking
- State is a flat two-level map: `object name → field name → value`
- `RunTaskTree` is a special action name that delegates to a subtree stored in state

## Linting

Uses `golangci-lint` v2 with `.golangci.yml` config:

- `gocyclo`: max complexity 100
- `gocognit`: max complexity 150
- `dupl`: threshold 100 tokens
- `funlen`: 80 lines / 50 statements
- `maintidx`: minimum 20
- `goconst`: 3+ occurrences, 3+ chars

## Test Data

Example environments in `testdata/`:

- `robot.json` — robot pick-and-place scenario (simple)
- `desktop_env.json` — desktop assistant behaviour/object definitions
- `desktop_outer.json` — fixed outer voice-interaction tree
- `desktop_inner.json` — LLM-generated inner task tree

## Design Decisions

- **No memory nodes** — all control nodes re-evaluate every child on each tick
- **Decorator nodes are stubbed** — types exist but interpreter panics on them
- **Behaviours are Go-native** — no declarative precondition/postcondition system; handlers are registered as Go functions
- **Simulation uses outcome requests** — the harness asks handlers to produce specific outcomes (success/failure/running); handlers signal incompatibility if current state doesn't allow it
- **Dynamic subtrees** — `RunTaskTree` enables LLM-generated inner trees composed within fixed outer trees
