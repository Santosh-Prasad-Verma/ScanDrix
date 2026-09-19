# agentharness

> `Agent = Model + Harness`. This package is the **engine** half — the **single agent loop** + control + observability that transforms a foundation model into a production agent.
> It is **domain-agnostic**: code review, security analysis, conversational assistants, and custom rule compilation are *applications* built on top of this harness, not part of it.

## Inviolable Dependency Rule

```
Domain Layer (internal/review, internal/agents, internal/rules, ...)
                      │
                      ▼ depends on
         @internal/agentharness
                      │
                   CANNOT depend on
                      ▼
                 Domain Layer
```

`internal/agentharness/contracts/` imports NO domain types. All domain capabilities enter via **injection**:
- `ToolContext.Services`: Opaque handles (e.g. repository sandboxes, ast graphs).
- `AgentSpec`: Declarative role configuration (system prompt, tools, policies).
- `Verifier[T]`: Objective rubric evaluation (doer != checker pattern).

## The Primitives (`contracts/`)

| Primitive | Role | Summary |
|---|---|---|
| `AgentSpec` | The agent as **data** | System prompt + tools + policies + maxSteps + resultToolName |
| `AgentRunner` | The **single loop** | `Run(ctx, spec, input, toolCtx) -> (*RunState, error)` |
| `RunState` | **Observable by construction** | Steps, token usage, status, stopReason, artifacts, append-only trace |
| `AgentTool` / `ToolRegistry` | **Capability** & collection | What the model calls in the loop. Errors are returned as values (`ToolResult.IsError`), never unhandled panics |
| `AgentPolicy` | **In-loop control seam** | `PrepareStep` (steer tools/notes/messages) + `ShouldStop` (gate termination) |
| `Verifier[T]` (+ `Verdict`) | **Post-run evaluator** | The Splits pattern: generator != evaluator (fail-open) |
| `Compressor` | **Context compaction port** | Prevents context window exhaustion under heavy tool calls |
| `ProgressLedger` | **Target coverage port** | Tracks required diff hunks / endpoints before finalizing |
| `ConversationStore` | **Continuity persistence port** | Persists conversation history across multi-turn sessions |

## Implemented Infrastructure (`infrastructure/`)

- `policies/`:
  - `BudgetPolicy`: Escalating guidance (`free` → `encourage` → `urgent`) with cache-friendly band transitions.
  - `CompletionGatePolicy`: Gating on critical pending targets in `ProgressLedger` and progress debt injection.
  - `ForceFinalizePolicy`: Restricts active tools to only `doneToolName` in final steps to compel result submission.
  - `CompressionPolicy`: Intercepts `PrepareStep` to compact message history when tokens cross thresholds.
- `tools/`:
  - `InMemoryToolRegistry`: Thread-safe tool collection.
  - `CachingTool`: Run-scoped canonical JSON memoization decorator (`ToolCallCache`) that re-serves full output bodies.
- `verify/`:
  - `SubmitVerdictTool`, `BuildVerifierAgentSpec`, and fail-open `ExtractVerdict`.
- `orchestration/`:
  - `DefaultSubAgentFactory`: Sub-agent-as-tool pattern for hierarchical agents and replicas.
  - `RunVerificationPass[T]`: Bounded-concurrency verification driver.
- `compression/`:
  - `TokenEstimator`: Real token estimation for dense code and provider wire overhead.
  - `ContextCompressor`: Head preservation, tail truncation, round eviction, and 3-tier hard budget clamp.
  - `ContextWindowCompressor`: 2-tier context window compressor implementing `contracts.Compressor`.
- `runner/`:
  - `GoAgentRunner`: Production `AgentRunner` engine with policy lifecycle hooks, directive merging, tool execution (`IsError` as value), result tool artifact materialization, and error tracing.
