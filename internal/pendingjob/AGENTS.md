# internal/pendingjob

The 429 queue: storage **and** behaviour. When a Groq call dies on a terminal rate limit the user's
message is cached in `pending_llm_jobs` instead of being dropped, and a ticker replays it once the
quota frees up. **Use `codegraph_explore` for structure** — this file is only for what reading the
code will not tell you.

## The replay ctx is built once, at the call site

`drainJob` builds it in one place: `context.WithoutCancel` over the shutdown ctx, the claim, then
`WithReplaying` inside `Traced`. Keep all three there — a new `case` that forgot the flag would
re-enqueue the job it is draining, forever. `WithoutCancel` matters: SIGTERM cancels the shutdown
ctx on every deploy, and a replay running on it loses its Groq calls mid-flight with nobody told
(`TestDrain_ReplayContextSurvivesShutdown`). `IsReplaying` is what tells a real webhook call from
a replay, and the drain deliberately replays through the **real** handler rather than a copy, so
the two paths cannot drift.

## The job is claimed at the first write, by the agent, not by the drain

The drain never deletes before replaying and never deletes blindly after. The agent calls
`ClaimReplay` the moment the turn stops talking to Groq and starts having effects; it deletes the
row and says whether this instance got it: not claimed → write and send nothing; claim failed →
job stays for the next tick; never claimed (no effects) → the drain deletes it afterwards; claimed
then a 429 → re-inserted with the original `CreatedAt` so `MaxJobAge` still bounds the retries.
Moving the delete either side reopens a hole: money twice after, silent loss before, on a crash.

## `EnqueueBehindPending` belongs to the webhook edge only

It runs from the controller's `dispatch` (in `controller/messaging`) after the engine reports no
open flow, never from `handleFreeText` (a drained replay would queue itself again). Its job is
ordering: while a user still has jobs waiting, a new message goes behind them so "no, 600" cannot
be processed before "gasté 500".

## A replayed job is its own unit of work, in the traces too

`drainUser` stamps a **fresh `trace_id`** on each replay (`traced()`): the drain's ctx carries none,
so without it every Groq call writes an empty `trace_id` and vanishes from all three observability
layers. It writes `update_type = replay`, distinct from text/callback/command, or it reads as the
user typing again. A payload that no longer parses is a decode **error**, logged and deleted.

## The drain's state is a single in-process value

`nextDrainAt` and its mutex are package-level. Gating is what makes ticking every 30s cheap: with no
quota there are no useless probes, and the drain **is** the probe — a 429 while draining re-gates
and cuts that user's cycle rather than retrying blindly. Being in-process, it holds for **one
instance only**, same known ceiling as `messaging.userLocks` — a second instance needs this in
Postgres, and starts at zero, draining on its first tick while the old one is still alive during a
deploy; the claim makes that overlap safe.

## Never silent, and never twice

- **An enqueue always acks.** `AckForWait` picks the wording by the size of the wait — under
  `ackShortWaitThreshold` the wait is TPM (seconds), over it TPD (rare, with an ETA). No silent path.
- **A turn that already inserted must never be enqueued.** That guard lives in `internal/agent`
  (`agentExecutor.wrote`); this package would replay it and double the money. Read `agent/AGENTS.md`.

`MaxJobAge` (2h) is the giving-up point: past it, a job is not rate-limited any more but permanently
broken (dead key, billing, provider down), and the user gets told rather than left waiting.

Why: `docs/decisions.md`, section **Groq quota, the 429 queue and rate limits**.
