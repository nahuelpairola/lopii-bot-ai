# internal/controller/messaging

The Telegram edge, and only that: the webhook handlers, `/start` + invitations, the bridge files
that let every cluster reach the DB, `userLocks`, tracing, and the metric outcomes. 20 non-test
files. **Use `codegraph_explore` for structure** — this file is only for what reading the code will
not tell you.

It used to be ~10k lines and hold the whole app. What lived here now lives in `flow` (the flows),
`agent` (the unified loop), `query` (free-text reads), `settings` (the configuration wizards),
`nudges` (contextual tips), `pendingjob` (the 429 queue) and `messages` (shared copy). Each has its
own doc-comments; the traps that moved with them moved too — `flow/AGENTS.md` carries flow
registration and the `callback_data` limit, `conversation/AGENTS.md` the `Data` contract.

## The bridge pattern, and why the methods are exported

Every cluster defines a narrow interface of what it needs from the world (`flow.runner`,
`agent`'s `agentServices`, `query`'s `services`, `settings.Services`, `nudges.Services`,
`pendingjob.Services`) and `*controller` implements all of them structurally, through one-line
bridge files (`*_services.go`). The clusters never import the repos.

The methods are **exported** even though most interfaces are not: an interface with unexported
methods can only be satisfied from inside its own package, and the implementation is here.

A bridge is shared the moment two clusters want it — `RemindersFindByUserID` lives in
`nudges_services.go` and `settings` reuses it. Adding a duplicate is a compile error, which is the
cheapest possible reminder.

## One update per user at a time

`Handle` — the neutral entry point, `messenger.Handler` — takes `c.locks.lock(uid)` before anything
else. Everything downstream is keyed by `user_id` and assumes one message in flight: the single
`conversation_states` row, the `pending_actions` drain, and the "most recent pending" that closes an
`intent_event`.

`userLocks` is an in-memory mutex, so it holds for **one process only**. With a second instance the
upgrade is `pg_advisory_xact_lock(user_id)`. Documented at the mutex itself.

## Two callbacks bypass the engine, deliberately

In `dispatch` (the body `Handle` runs under the per-user lock), *before* `c.engine.Handle`:

1. `nudges.HandleCallback` — a tip's button tap.
2. `flow.HandleNearDuplicateChoice` — the near-duplicate gate's tap.

Both go early so an open flow cannot swallow the callback as if it were one of its own options.
Neither belongs to any flow, and both are safe there: the nudge query is read-only and leaves the
open flow intact.

## The queue's ordering invariant

`pendingjob.EnqueueBehindPending` runs *after* `engine.Handle` returns "no open flow", and only for
text — it is what keeps "no, 600" from being processed before "gasté 500" when the user still has
jobs waiting.

It is called from `dispatch` only, never from `handleFreeText`: called from there, a drained replay
would re-enqueue itself forever. `pendingjob`'s replay ctx flag is what tells a real webhook call
from a replay, and the drain deliberately replays through the *real* handler rather than a copy, so
the two paths cannot drift.

## There is no router

`handleFreeText` is one line: every message without an open flow goes to `agent.StartLoop`, and the
loop picks one of its tools. The ten-branch switch that used to live here is gone, and so are
orchestrator.ClassifyIntent, ClassifyCreate and ResolveDelete — the last two survive only as
methods on test fakes, which nothing in production calls.

That is the point of stage 5, not a refactor: routing *first* meant an intent had to be guessed
before anything could be done, and on 2026-08-10 a user asked for one thing — move a batch of
movements to another category — six ways, and each phrasing landed somewhere that had no way to do
it. The wizards did not go away; the loop **parks** into one when it needs an answer.

## Money, and the rest

Anything touching amounts, signs or `account_id`: read `AGENTS.md` (§ The accounting
model) **before** editing. The anti-pattern list is in `docs/ARCHITECTURE.md`.

In tests, `c.sendText`/`c.sendPrompt`/`c.startFlow`/`c.handleFreeText` and every `finish*` bridge
take a `messenger.Chat` directly — pass `&messenger.FakeChat{}`, not a Telegram `(bot, chatID)`
pair. edgeChat and newEdgeChat, the temporary wrapper that used to stand in for that pair, are
gone as of Task 6: the webhook edge now arrives with `in.Chat` already resolved by the adapter
(`internal/messenger/telegram`), so there is nothing left to wrap.

Every bridge reachable from more than one entry point (the webhook edge AND `pendingjob`'s drain —
`SendText`, `SendPrompt`, `StartFlow`, `HandleFreeText`, `FinishAnswerQuery`,
`FinishManageSettings`) must go straight through `messenger.SendText`/`chat.Send`/`agent.StartLoop`
on the `messenger.Chat` it receives — never unwrap it back to `(bot, chatID)` first. A chat that
`chatResolver.ChatFor` resolves (the sweeper, the 429 drain) is a different concrete type than one
the webhook edge hands in, so any unwrap that only recognizes one concrete type fails silently for
the other — and "silently" here means a replayed message that needs to ask the user something (a
gap-fill, a QUERY reply, a settings wizard) gets dropped with no error and no message, which is
exactly the "never silent" invariant `pendingjob/AGENTS.md` protects. This is why asTelegramPair
and pairOrLog — the unwrap helpers that used to sit in chat_bridge.go — are gone: they were the
bug, found in Task 8's first review round.

---

Why: `docs/decisions.md`, section **Groq quota, the 429 queue and rate limits** (the lock) and **Package layout, metrics and tooling**.
