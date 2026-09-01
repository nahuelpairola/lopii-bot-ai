# internal/pendingjob

The 429 queue: storage **and** behaviour. When a Groq call dies on a terminal rate limit the user's
message is cached in `pending_llm_jobs` instead of being dropped, and a ticker replays it once the
quota frees up. **Use `codegraph_explore` for structure** — this file is only for what reading the
code will not tell you.

## The replay flag is set once, at the call site

`drainUser` wraps the ctx with `WithReplaying` *before* calling `replayJob`, not inside each `case`
of its switch. That is deliberate and worth keeping: a new `case` that forgot to mark it would
re-enqueue the very job it is draining, forever. Setting it once, outside, makes forgetting
impossible rather than merely unlikely.

`IsReplaying` is what tells a real webhook call from a replay, and the drain deliberately replays
through the **real** handler rather than a copy, so the two paths cannot drift.

## `EnqueueBehindPending` belongs to the webhook edge only

It runs from the controller's `dispatch` (in `controller/messaging`) after the engine reports no
open flow — never from `handleFreeText`. Called from there, a drained replay would queue itself
again.

Its job is ordering: while a user still has jobs waiting, a new message goes behind them so
"no, 600" cannot be processed before "gasté 500".

## A replayed job is its own unit of work, in the traces too

`drainUser` stamps a **fresh `trace_id`** on each replay (`traced()`): the drain's ctx comes from
the server's ticker and carries none, so without it every Groq call of the replay writes an empty
`trace_id` and the message vanishes from all three observability layers. It also writes
`update_type = replay`, distinct from text/callback/command — otherwise a replay reads in
`request_traces` as if the user had typed again, and the queue's latency mixes into the real
messages'. A job whose payload no longer parses is a non-429, so it is **deleted**: a corrupt row
must never block the queue behind it.

## The drain's state is a single in-process value

`nextDrainAt` and its mutex are package-level. Gating is what makes ticking every 30s cheap: with no
quota there are no useless probes, and the drain **is** the probe — a 429 while draining re-gates
and cuts that user's cycle rather than retrying blindly.

Being in-process, it holds for **one instance only**, same known ceiling as `messaging.userLocks`.
A second instance needs this in Postgres.

## Never silent, and never twice

Two rules the copy depends on:

- **An enqueue always acks.** `AckForWait` picks the wording by the size of the wait — under
  `ackShortWaitThreshold` the wait is TPM (seconds), over it TPD (rare) and the copy carries an
  ETA. Only the copy branches; there is no silent path.
- **A turn that already inserted must never be enqueued.** That guard lives in `internal/agent`
  (`agentExecutor.wrote`); this package is what would replay it, and the money would be recorded
  twice. Read `agent/AGENTS.md` before touching either side.

`MaxJobAge` (2h) is the giving-up point: past it, a job is not rate-limited any more but permanently
broken (dead key, billing, provider down), and the user gets told rather than left waiting.

Why: `docs/decisions.md`, section **Groq quota, the 429 queue and rate limits**.
