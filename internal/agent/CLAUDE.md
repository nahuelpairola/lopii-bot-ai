# internal/agent

The unified agent loop, consumer side: `StartLoop` is where **every** free-text message goes, and
the tool the model picks decides what happens. **Use `codegraph_explore` for structure** — this file
is only for what reading the code will not tell you.

## `record_movements` is the only tool that writes

Two rules hang off that, and both exist because of a bug that already happened.

**A turn that inserted must never be enqueued on a later 429.** `agentExecutor.wrote` is set
*before* any return from the write path (`agent_executor.go`), precisely so a 429 arriving after
the insert cannot queue the turn — the drain would replay it and register the money twice.
`start_loop` checks it. There is a test whose failure message says it outright; if you touch the
ordering, keep it.

**A CREATE that cannot complete inserts nothing at all.** All-or-nothing per batch is what stops a
two-leg transfer from splitting. When the turn can't finish it *parks* instead:

- a missing field parks an action that resumes into `movement_create`;
- an overdraft parks one that resumes into `movement_negative_confirm`.

`conversation.KeyGatePrompt` in the seed is what tells the two apart when the action is picked back
up. Same park, different meaning — read it before adding a third case.

## The loop parks, it does not route

There is no router (it was deleted in stage 5). When the loop needs an answer from the user it
parks into a flow and the wizard takes over; `pending_actions` is drained one at a time (WIP=1),
and a flow finishing is the only moment we know nothing else is open.

The wizards themselves are not here — the ones reachable from `manage_settings` live in
`internal/settings`, and every flow is built in `internal/flow`.

## Money

Anything touching amounts, signs or `account_id`: read the root `CLAUDE.md` (§ The accounting
model) **before** editing. The app owns the sign and the arithmetic; a correction arrives as a
structured diff (`field`/`op`/`value`) and the app computes the result — the model never sends a
number it worked out itself.
