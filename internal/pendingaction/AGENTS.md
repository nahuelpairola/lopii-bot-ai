# internal/pendingaction

The durable per-user queue of actions the agent loop could not finish in its turn, because a
piece of data only the user has is missing.

## It is not `pendingjob`, and confusing the two is the trap

They are sibling queues with opposite contents:

| | Holds | Written when |
|---|---|---|
| `pendingjob` | a **message** that was never processed | Groq returned a terminal 429 |
| `pendingaction` | an **action already interpreted**, missing an answer | the loop needs to ask the user |

A replay of the first re-runs the whole turn; a drain of the second only fills in a blank. Both
drain one at a time — while one is open, no second one starts.

## `Options` accelerate, they never trap

An `OpenQuestion` carries `Key` (**where** the answer goes inside the action's payload, e.g.
`row0.category`) and `Options` (buttons). **Free text is always accepted**, whatever the buttons
offer. That is the whole difference from the old picker, which could only offer what already
existed and left the user with no way out when the right answer was not on the list.

`Key` is interpreted by whichever package parked the action, not here.

`Budget` is stored, not recomputed at drain time.

Why: `docs/decisions.md`, section **Groq quota, the 429 queue and rate limits**.
