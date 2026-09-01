# internal/orchestrator

Groq tool-calling over plain `net/http`. Heavily tested — the traps below are the ones no test
covers, because they only bite on *future* edits.

## Three call types share `createModel`

Since stage 5 deleted the router, `Config` carries five model fields
(create/update/query/agent/classifier), which reads as one model per call type. It is not:

| Call site | `callType` bucket | Model actually used |
|---|---|---|
| `ResolveAccountManage` | `account_manage` | `o.createModel` |
| `ClassifyCategoryCreate` | `category_create` | `o.createModel` |
| `ClassifyOnboarding` | `onboarding` | `o.createModel` |

**Retuning `createModel` retunes three unrelated wizard paths.** The separate Grafana buckets
hide it — they suggest three independent call types. No test covers this.

## Groq's ceilings are PER MODEL, and that is the whole design

TPM, requests/day and TPD are counted per model, so a 429 on one says nothing about another.
Everything below follows from that, and none of it is arbitrary:

- **`agentRound` (`agent.go`) walks a chain** — `agentModel`, then `agentFallbacks` in order —
  **only on 429**. A 400 returns immediately: retrying a schema error on another model burns a
  second call to get the same rejection. Each attempt writes its own `llm_calls` row, which is
  what makes the chain visible in Grafana without a new column.
- **`classifierModel` is deliberately a different model** from `agentModel`, to land in a
  different bucket. `ClassifyCategories` returns no error: on 429 or timeout it degrades to
  `PENDING_REVIEW` and the picker opens. **A classifier without quota therefore looks like a
  model classifying badly**, not like a failure — `llm_calls` is the only place it shows.
- **Groq reserves `prompt + max_completion_tokens`** against the quota whether the completion
  uses it or not, so a completion ceiling is quota spent on every call. `maxAgentCompletionTokens`
  is measured against the worst real batch, never rounded up "to be safe".

## `AgentTool.Kind` is vestigial — and that is a loaded gun

orderCallsByKind and kindRank were deleted in stage 5, along with the read-before-write ordering
they enforced. **`Kind` survives as a field that nothing reads** — every tool still declares one,
and no code looks at it.

**The premise that justified deleting it expired the same day.** `a419524` removed ordering
arguing the toolbox had no read tools — true when written — and `0b3427a`, later that
afternoon, added five (`list_categories`, `sum_movements`, `list_movements`,
`account_balance`, `get_reminder`). Nobody went back. A round executes **every** call the
model emitted, in the model's own order (`agent.go`, the `range assistant.ToolCalls`), so a
`sum_movements` and a `record_movements` in one round are once again the exact situation
ordering existed for: a total computed without the rows about to be inserted. Whether the
model actually mixes them in one round is **unmeasured** — `llm_calls.tool_calls` is where
that answer lives.

If ordering does come back, the old trap has to be avoided rather than reintroduced: an unset
`Kind` sorted into the same bucket as an explicit `KindRead`, so a write tool declared without
`Kind` ran *after* the reads. Make the zero value unrepresentable.

## Shared prompt fragments must contain no `%` and no backtick

`numberFormatRule` (`number_format.go`) and the two consts in `movement_rules.go` are
concatenated into `fmt.Sprintf` templates. A literal `%` corrupts the rendered system prompt at
runtime — no panic, no error, the model just receives a garbled instruction block. Any percentage
that reaches a prompt const has to be written `%%`.

`movement_rules.go` holds what the prompt templates say **verbatim**, deduplicated after a
2026-08-08 fix had to be pasted into both by hand. It is two consts, not one, because
`REGLA DE FECHA` sits between them and genuinely differs.

## `Run` and `AnswerQuery` are two near-identical loops, kept apart on purpose

`Run` (`agent.go`) is the unified agent loop and, since stage 5, the **only** path a free-text
message takes. `AnswerQuery` is the read-only QUERY loop, reached through the `answer_query`
tool. They differ in completion ceiling (1500 vs 1024), in whether content alongside
`tool_calls` ends the turn (`Run` only), and in the model fallback chain (`Run` only).
**Fixing a bug in one and porting it to the other by habit is a live risk.** Same shape, two
types too: `toolSchema` for single-shot calls, `AgentTool` for loop calls.

## Door 2 is incomplete by construction, and its prompt must say so

`AnswerQuery` has two exits. **Door 1** is the healthy one: the model stops requesting tools and
its own content is the answer. **Door 2** is the forced narration after the round cap ran out —
which means it is reached *only* when the model still wanted more tools. Its data is therefore
incomplete **as a matter of control flow**, and Go knows that without parsing the question.

**Say it in the prompt, or the model fills the hole by denying** — it has reported "no records"
over real spending it never queried, on a question naming three entities. The rule that governs
amounts (a fact about the world needs a tool behind it) extends to **absences**.

**The guarantee lives in the prompt, not in Go** — a deliberate choice, so the compensating
control is the `multi_entidad_no_niega_lo_que_no_consulto` eval, which *measures* compliance
instead of assuming it. Its blocklist of denial phrases is inherently incomplete: the first
version passed green while the model denied three entities with a phrasing the list lacked.

## Copies that nothing keeps in sync

The read-tool schemas in `agent_tools.go` are hand-copied **verbatim** from `internal/query`'s
`Tools`, because query's executor parses those exact argument names (`queryToolArgs`).

A comment used to be the only thing enforcing it; since 2026-08-14 a test does —
`TestQueryTools_ParametersMatchAgentTools` (`query/query_tools_parity_test.go`) compares
the two `Parameters` blocks and fails on any divergence. It exists because the failure mode is
**silent and wide**: change one file and not the other, and the agent offers the model a
parameter the executor no longer parses. `json.Unmarshal` drops the unknown field (nothing sets
`additionalProperties`, so Groq does not 400 on it), the filter is never applied, and the query
answers over the whole range. No error, no log, and no other test sees it.

`recorder.go`'s `LLMCall` is deliberately *not* the `metric`
package's GORM model — `server` maps between them by hand, so a new field needs both.

## The restriction goes in the SCHEMA, not the prompt

Stage 5 relearned this twice at a cost. Groq validates arguments server-side and 400s before any
Go code sees the payload, so a rule the prompt states but the schema does not declare is a rule
the model breaks for free. `correct_movement`'s `op` is nullable in the schema for exactly this
reason: it was required, the model omitted it, and every such turn died as a hard 400 with the
user's correction lost.

Why: `docs/decisions.md`, section **The agent loop and QUERY** and **Groq quota, the 429 queue and rate limits**.
