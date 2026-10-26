# internal/controller/messaging

The Telegram edge, and only that: the webhook handlers, `/start` + invitations, the bridge files
that let every cluster reach the DB, `userLocks`, tracing, and the metric outcomes. 20 non-test
files. **Use `codegraph_explore` for structure** — this file is only for what reading the code will
not tell you.

It used to be ~10k lines and hold the whole app. What lived here now lives in `flow` (the flows),
`agent` (the unified loop), `query` (free-text reads), `settings` (the configuration wizards),
`nudges` (contextual tips) and `pendingjob` (the 429 queue). Each has its own doc-comments; the
traps that moved with them moved too — `flow/AGENTS.md` carries flow registration and the
`callback_data` limit, `conversation/AGENTS.md` the `Data` contract.

## Consumers get the repositories, not the controller — do not add a bridge

`c.queries()`, `c.wizards()`, `c.tips()`, `c.finisher()` and `c.loop()` call each package's
`New(…)` with the repositories themselves; callbacks into the controller (`c.finishAnswerQuery`,
`c.handleGroqError`, `c.resolveMetric`, …) go in as funcs. Each is built **per call on purpose** —
most tests here build `&controller{…}` literals, and a stored pointer would be nil in every one of
them. Why: `docs/decisions.md`, § Package layout.

The controller implements one interface: `pendingjob.Replayer` (`HandleFreeText`,
`ProceedToUpdateConfirm`, `Traced`), because the drain replays through the real handler.

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

The chat they receive is already wrapped in `messenger.ReplaceTapped` — the wrap happens at the top
of `dispatch`, before them, so whatever they send first **rewrites the tapped message**. A new early
callback inherits that; one that wants a separate message must send twice or run before the wrap.

## The queue's ordering invariant

`pendingjob.EnqueueBehindPending` runs *after* `engine.Handle` returns "no open flow", and only for
text — it is what keeps "no, 600" from being processed before "gasté 500" when the user still has
jobs waiting.

It is called from `dispatch` only, never from `HandleFreeText`: called from there, a drained replay
would re-enqueue itself forever. `pendingjob`'s replay ctx flag is what tells a real webhook call
from a replay, and the drain deliberately replays through the *real* handler rather than a copy, so
the two paths cannot drift.

## There is no router

`HandleFreeText` is one line: every message without an open flow goes to the agent's `StartLoop`, and the
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

In tests, `c.HandleFreeText` and every `c.finisher().Finish*`
take a `messenger.Chat` directly — pass `&messenger.FakeChat{}`, not a Telegram `(bot, chatID)`
pair. edgeChat and newEdgeChat, the temporary wrapper that used to stand in for that pair, are
gone as of Task 6: the webhook edge now arrives with `in.Chat` already resolved by the adapter
(`internal/messenger/telegram`), so there is nothing left to wrap.

Everything reachable from more than one entry point (the webhook edge AND `pendingjob`'s drain —
`SendText`, `HandleFreeText`, `ProceedToUpdateConfirm`, and the funcs `c.loop()` hands the agent)
must go straight through `flow.SendText`/`chat.Send`/`StartLoop` on the `messenger.Chat` it
receives — never unwrap it back to `(bot, chatID)` first. A chat that
`chatResolver.ChatFor` resolves (the sweeper, the 429 drain) is a different concrete type than one
the webhook edge hands in, so any unwrap that only recognizes one concrete type fails silently for
the other — and "silently" here means a replayed message that needs to ask the user something (a
gap-fill, a QUERY reply, a settings wizard) gets dropped with no error and no message, which is
exactly the "never silent" invariant `pendingjob/AGENTS.md` protects. This is why asTelegramPair
and pairOrLog — the unwrap helpers that used to sit in chat_bridge.go — are gone: they were the
bug, found in Task 8's first review round.

Why: `docs/decisions.md`, section **Groq quota, the 429 queue and rate limits** (the lock) and **Package layout, metrics and tooling**.
