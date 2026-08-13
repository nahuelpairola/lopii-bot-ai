# internal/orchestrator

Groq tool-calling over plain `net/http`. Heavily tested — the traps below are the ones no test
covers, because they only bite on *future* edits.

## Four call types share `createModel`

`Config` has six model fields (router/create/update/delete/query/agent), which reads as one
model per call type. It is not:

| Call site | `callType` bucket | Model actually used |
|---|---|---|
| `create.go:83` | `create` | `o.createModel` |
| `account_manage.go:39` | `account_manage` | `o.createModel` |
| `category_create.go:55` | `category_create` | `o.createModel` |
| `onboarding.go:44` | `onboarding` | `o.createModel` |

**Retuning `createModel` retunes four unrelated paths.** The separate Grafana buckets hide it —
they suggest four independent call types. No test covers this.

## `AgentTool.Kind` has a dangerous zero value

`kindRank` (`agent.go:32-43`) sorts an unset `Kind` into the same bucket as an explicit
`KindRead`. A future write tool added to `AgentTools()` without `Kind: KindWrite` compiles and
runs **after** the reads in its round — precisely the "the total excludes the rows I am about to
insert" bug that `orderCallsByKind`'s own comment warns about. The existing test pins today's
single write tool, not a future mis-declared one.

## Shared prompt fragments must contain no `%` and no backtick

`numberFormatRule` (`number_format.go`) and the two consts in `movement_rules.go` are
concatenated into `fmt.Sprintf` templates. A literal `%` corrupts the rendered system prompt at
runtime — no panic, no error, the model just receives a garbled instruction block. Escape it as
`%%`, as the "90%%" in `taxonomyAndAmountRules` does.

`movement_rules.go` holds what `createSystemPromptTemplate` and `agentSystemPromptTemplate` say
**verbatim**, deduplicated after a 2026-08-08 fix had to be pasted into both by hand. It is two
consts, not one, because `REGLA DE FECHA` sits between them and genuinely differs — the loop
adds the timezone and a date-range rule for its query tools. **That block stays duplicated on
purpose**; unifying it would push a query instruction into `create`, which has no query tool.

## `Run` and `AnswerQuery` are two near-identical loops, kept apart on purpose

`Run` (`agent.go`) is the unified agent loop; `AnswerQuery` is the read-only QUERY loop, frozen
until stage 4 of the migration. They differ in iteration cap (5 vs 3), in whether content
alongside `tool_calls` ends the turn (`Run` only), in class-ordered execution (`Run` only) and
in completion-token ceiling. **Fixing a bug in one and porting it to the other by habit is a
live risk** — they are separate so each stage can be bisected. Same shape, two types too:
`toolSchema` for single-shot calls, `AgentTool` for loop calls.

## Copies that nothing keeps in sync

The read-tool schemas in `agent_tools.go` are hand-copied **verbatim** from `messaging/query.go`'s
`queryTools`, because messaging's executor parses those exact argument names (`queryToolArgs`).
Only a comment enforces it. `recorder.go`'s `LLMCall` is deliberately *not* the `metric`
package's GORM model — `server` maps between them by hand, so a new field needs both.
