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

It runs from `handleConversationInput` (in `controller/messaging`) after the engine reports no open
flow — never from `handleFreeText`. Called from there, a drained replay would queue itself again.

Its job is ordering: while a user still has jobs waiting, a new message goes behind them so
"no, 600" cannot be processed before "gasté 500".

## The drain's state is a single in-process value

`nextDrainAt` and its mutex are package-level. Gating is what makes ticking every 30s cheap: with no
quota there are no useless probes, and the drain **is** the probe — a 429 while draining re-gates
and cuts that user's cycle rather than retrying blindly.

Being in-process, it holds for **one instance only**, same known ceiling as `messaging.userLocks`.
A second instance needs this in Postgres.

## Never silent, and never twice

Two rules the copy depends on:

- **An enqueue always acks.** `AckForWait` picks between the short and the long wording by the size
  of the wait; there is no path where the user's message is cached without them being told.
- **A turn that already inserted must never be enqueued.** That guard lives in `internal/agent`
  (`agentExecutor.wrote`), because that is where the write happens — but this package is the thing
  that would replay it, and the money would be recorded twice. Read `agent/CLAUDE.md` before
  touching either side.

`maxJobAge` (2h) is the giving-up point: past it, a job is not rate-limited any more but permanently
broken (dead key, billing, provider down), and the user gets told rather than left waiting.

---

**Why the design is this way** — the measurements, incidents and rejected
alternatives behind these rules live in `docs/decisions.md`, section **Groq quota, the 429 queue and rate limits**.
Read it before changing a design choice: most were already argued there, with the
production numbers that settled them.
