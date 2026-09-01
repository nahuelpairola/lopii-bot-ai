# internal/flow

Every conversation flow: the 15 builders `server/flows.go` registers, their steps and options, the
movement write pipeline, the finishes, and the near-duplicate gate. **Use `codegraph_explore` for
structure** — this file is only for what reading the code will not tell you.

## Registering a flow touches three places, in three packages

Nothing enforces any of them:

1. `server/flows.go` — a line in `registerFlows`
2. `controller/messaging/controller.go` — a case in `handleFlowFinished`'s switch
3. `controller/messaging/messages.go` — a case in `FlowResumeLabel`

Miss #1 and the server fails at startup (loud, fine). Miss #2 and the flow completes into
`msgSomethingBroke`. **Miss #3 and nothing breaks until a user goes idle for 24h**, then the
resume gate offers them "una conversación anterior" instead of real copy.

Two of the three live in `messaging`, not here: the flow is *built* in this package but *finished*
at the edge, because a finish needs the repos. That split is the reason the list is easy to
half-do.

## `callback_data` is 64 bytes — send indices, not labels

Telegram truncates past 64 bytes and the callback stops matching any option, so the button
silently does nothing.

**This is violated today.** `category_picker.go` and `movement_create_flow.go` put raw category and
subcategory names into the callback value, and users create their own categories. There is no
guard anywhere in the repo. `movement_delete_flow.go` and `ask_user_flow.go` show the correct
shape: `strconv.Itoa(i)`, resolved back on the other side.

## `runner` is exported on purpose

`runner` (`runner.go`) is the narrow interface this package uses to reach the DB and the outbound
bot, implemented by `messaging`'s `*controller`. Its **methods are exported although the interface
is not**: an interface with unexported methods can only be implemented from inside its own package,
and the implementation lives at the edge.

Anything needing the LLM does *not* go through `runner` directly — `flow` never imports
`orchestrator`. The two cases that need it (`StartAccountCreate`, `SuggestMergeTarget`) are runner
methods that the edge forwards to `internal/settings`.

## Two pickers, and only one of them re-searches

`ask_user_flow.go` is a `TextStep` with accelerator buttons: free text is the point, and an answer
naming none of the options makes the agent loop search again (`agent_dispatch.go`). Its budget is
spent **per round, not per question** — an answer can be useless ("no sé") and make the executor
park again with a new question — so the ceiling is frozen at park time. Otherwise a growing list
of questions raises its own ceiling.
`movement_update_flow.go`'s `movement_update_pick` and `movement_delete_flow.go` are `ChoiceStep`s:
there, free text lands on `InvalidChoiceMessage` and nothing is re-searched.

They **share the prompt constants** (`MsgPickUpdateCandidate`, `MsgPickDeleteCandidate`), which is
why `MsgCanRetypeToSearch` is appended by `park` at the `ask_user` call site rather than being
folded into either constant. Putting it inside one of them writes a promise the `ChoiceStep` path
cannot keep.

## Money

Amounts, signs or `account_id`: `AGENTS.md` § The accounting model, before editing. The write
pipeline (`movement_write.go`) is where those invariants are enforced.

**A finish re-reads its rows from the DB instead of trusting what it was handed.**
`FinishAccountAdjust` re-reads the account so the guard compares the movement's currency against
the database rather than against the flow's `Data`; a mismatch is rejected instead of writing a
movement in a currency its account does not hold, which is the error that corrupts a balance in
silence (a balance is `SUM(amount)` and never looks at each row's currency).
`ApplyNearDuplicateChoice` re-reads both rows for the same reason: a tap can arrive late, and
adding an amount to a row that already changed corrupts a balance from a stale screen.

**The one deliberate exception to the guard** is that same `ApplyNearDuplicateChoice`, which
writes without `movement.Normalize`. It can: both rows came out of the guard when they were
inserted, and `nearDuplicateCandidate` requires them to share type, currency and account — so they
share a sign, the sum can neither zero out nor invert, and neither currency nor account changes
here. Loosening the candidate rule means putting the guard back.

**A failed balance adjustment is invisible without its log line.** The finish is a void function,
so the error never reaches `traced`.

In tests `b` is nil. Use `r.SendText(...)`, which guards; a direct `b.SendMessage` panics.

Why: `docs/decisions.md`, section **Conversation engine and flows**.
