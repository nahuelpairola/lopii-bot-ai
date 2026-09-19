# internal/agent

The unified agent loop, consumer side: `StartLoop` is where **every** free-text message goes.
Use `codegraph_explore` for structure; this file is only what the code will not tell you.

## `record_movements` is the only tool that writes

Two rules hang off that, both from a bug that already happened.

**A turn that inserted must never be enqueued on a later 429.** `agentExecutor.wrote` is set
*before* any return from the write path (`agent_executor.go`), so a 429 arriving after the insert
cannot queue the turn — the drain would replay it and register the money twice. `start_loop`
checks it; a test's failure message says this outright, so keep the ordering.

**A CREATE that cannot complete inserts nothing at all.** All-or-nothing per batch is what stops a
two-leg transfer from splitting. When the turn can't finish it *parks* instead:

- a missing field parks an action that resumes into `movement_create`;
- an overdraft parks one that resumes into `movement_negative_confirm`.

`conversation.KeyGatePrompt` in the seed is what tells the two apart when the action is picked back
up. Same park, different meaning — read it before adding a third case.

## The account of an expense is resolved from the MESSAGE, never from the model

`account_id` in a `record_movements` draft is honored **only on `transfer` legs**
(`seed.go`, `buildCreateSeed`). On `expense`/`income` it is dropped and the account is
resolved by `accountNamedInMessage` against the user's raw text.

This looks like distrust of a field that works. It is not: the model fills `account_id` on most
expenses whether or not the user named one, picking by position in the prompt's account block —
ordered by id, with no default marked. A user whose first ARS row is their default sees no bug;
a user whose first row is some other account gets their expenses written to it.

**Two rules hang off this and both are load-bearing:**

- The match requires coverage of **1.0** — every token of the account name present in the message.
  "Some token" matches `Mercado Pago` against `pago de monotributo`, the most common message shape.
- `matchNamedAccount` is called **only** from the `transfer` branch. In the other branch it is
  unreachable by construction: it requires an exact name match, so anything it could resolve
  `accountNamedInMessage` already found. Calling it there is dead code that reads as live.

`account_name_guess` survives for one job only — naming an account that does **not exist yet**
("pagué el curso con Brubank") so the gap can offer to create it; ignored unless the message backs
it up. `userText` is the **turn's** message, not the row's: naming one account assigns it to every
row of that turn — right for "pagué luz 5000 y gas 3000 con mercado pago", wrong if the user mixes
accounts in one message, chosen knowingly with no measured case of the second. UPDATE ignores all
of this.

## One reference resolver, and it has two windows

`resolveCandidates` (`reference_resolution.go`) is the **only** candidate-search mechanism; both
correction paths share it — do not add a second. Textual relevance is **scored and ranked** in Go
(`scoreGroup`, accent-folded), never in SQL: a candidate's score is the FRACTION of its own
description tokens the message names, plus a capped tie-break for date proximity, and the cut to
five is by score, not recency. A score of 0 means "no match" and the group never enters — that
keeps the recency fallback alive; a change letting the date term alone produce a candidate would
silently delete it. Shortcut: when nothing matches textually but the user just recorded something,
it returns **exactly one** candidate — the recent entry — rather than a picker.

It has **two windows, and picking the wrong one is the whole bug class.** With no date it
searches by `created_at` ("what did I just enter"); with a date it searches by *business* date. A
correction naming a date entered days later — "el débito del 4 de agosto", loaded on the 8th — is
invisible to the first window and obvious to the second, which is why `correct_movement` carries
`date_from`/`date_to` as a **locator** (they never change the movement; a date correction travels
in `changes` with `field: "date"`). One lone date closes the window on *both* sides: an open
`until` does not narrow anything, and a lone `date_to` used to invert the window outright.

Consequence: a relative reference must arrive with **no date at all** — the model cannot turn a
weekday into one, so `date_from`'s description asks for a date **only** when the message spells out
day and month. Why `TestAgentDateAnchorEval` keeps "el lunes" red: `docs/decisions.md` (§ The
agent loop and QUERY).

When the picker's answer is free text that names none of the options, the loop **searches again**
with the original message plus what the user just typed, instead of re-asking. That is what
`SearchText`/`DateFrom`/`DateTo` are doing in the parked payload: `park` would otherwise drop all
three, leaving nothing to search with. `SearchText` is `e.userText`, never `Change` — the model's
paraphrase — and `applyAnswers` concatenates the user's answers onto it.

## Draining an action: three rules that are not in the code's shape

- **`budgetSlack` is 2 grace rounds** over the number of open questions: one for an answer that
  did not help, one for the retry — a third would burn the user's time on something the bot is
  not going to understand.
- **A candidate is validated BEFORE the action is deleted.** A corrupt payload is not dropped in
  silence — it stays queued and fails loudly.
- **When the field came from a button and the value was just typed, the app has the whole
  correction and does NOT call the model.** The change is built in Go; amounts never come through
  here, they are typed straight, with no button.
- Two accounts sharing a name **and** a currency do exist in production; picking one is
  non-deterministic, so the resolver asks.

## The loop parks, it does not route

There is no router. When the loop needs an answer from the user it parks into a flow and the
wizard takes over; `pending_actions` is drained one at a time (WIP=1), and a flow finishing is
the only moment we know nothing else is open. The wizards themselves are not here — the ones
reachable from `manage_settings` live in `internal/settings`, every flow in `internal/flow`.

## A replayed turn claims its job before any effect

Claim mechanics: `pendingjob/AGENTS.md`. Three claim sites: `startAgentLoop`, right after `Run`
returns; `record`, before `ResolveAndInsertMovements` (that write happens *inside* `Run`); and
`proceedToUpdateConfirm`, before each branch's first effect.

A lost claim writes nothing and **says nothing**. `record` cannot return the claim error to the
model — `Run` feeds tool errors back as text and the model would narrate it — so it parks the
error on the executor and ends the turn with `ErrAgentTurnDone`.

A new effect on a replayable path with no claim compiles, passes every non-replay test, and
doubles when two instances drain the same job during a deploy overlap.

## Money

Amounts, signs or `account_id`: `AGENTS.md` § The accounting model, before editing. Why (for the
rest of this file): `docs/decisions.md`, section **The agent loop and QUERY**.

## What is not this package's business

`agent` decides **what** to do; `orchestrator` knows **how to talk to Groq**. Model choice,
fallback chains, TPM ceilings and tool schemas are not decisions this package makes — see
`orchestrator/AGENTS.md` before changing any of them here.
