# History

## Epoch 1: LLM-Generated Behaviour Trees (up to commit 6d21d02)

### Thesis

The original approach asked LLMs to generate complete behaviour trees from
natural-language task descriptions. The generated JSON trees were validated,
interpreted, and exhaustively simulated to check correctness.

### What Was Built

- JSON-based behaviour tree format with Sequence, Fallback, Condition, Action nodes
- Document/Environment system for loading and merging definitions from multiple files
- Static validation of tree structure against environment definitions
- Tree-walking interpreter with outcome-request simulation support
- Exhaustive simulation harness (all outcome permutations)
- Recording tracer with span-tree output and JSONL persistence
- LLM generation pipeline with GBNF grammar constraining and multi-provider support
- Benchmark framework for evaluating LLM tree generation quality
- CLI tool (`beht`) with benchmark, eval, show, and trace subcommands
- Two example scenarios: robot pick-and-place, desktop assistant

### What We Learned

LLMs struggle with the structural correctness required for behaviour trees:
ordering, reactivity patterns, and backchaining. Even with grammar constraining,
generated trees frequently had logical errors (wrong child ordering, missing
fallback guards, incorrect precondition checks).

### The Pivot

The PA-BT algorithm (Colledanchise & Ogren) can construct provably correct
trees automatically from action definitions with pre/postconditions. The LLM's
role simplifies to selecting relevant actions and grounding their parameters --
essentially function-calling, which LLMs are already good at.

### Components Removed in Epoch 2

- **Decorator node stubs** (`InverterNode`, `ForceSuccessNode`, `ForceFailureNode`,
  `RetryUntilSuccessfulNode`): types existed but interpreter panicked on them
- **Condition behaviour definitions**: users no longer define condition behaviours
  in JSON; PA-BT generates condition nodes internally from action pre/postconditions
- **GBNF grammar for tree generation**: replaced by action-selection format
- **Tree generation system prompt**: replaced by action grounding prompt
