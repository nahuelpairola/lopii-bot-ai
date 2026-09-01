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

## The account of an expense is resolved from the MESSAGE, never from the model

`account_id` in a `record_movements` draft is honored **only on `transfer` legs**
(`seed.go`, `buildCreateSeed`). On `expense`/`income` it is dropped and the account is
resolved by `accountNamedInMessage` against the user's raw text.

This looks like distrust of a field that works. It is not: the model fills `account_id` on
most expenses whether or not the user named an account, and it picks by position in the
prompt's account block — which is ordered by id and does not mark the default. A user whose
first ARS row happens to be their default sees no bug; a user whose first row is some other
account gets their expenses written to it.

**Two rules hang off this and both are load-bearing:**

- The match requires coverage of **1.0** — every token of the account name present in the
  message. "Some token" matches the account `Mercado Pago` against the message
  `pago de monotributo`, which is the single most common shape of message there is.
- `matchNamedAccount` is called **only** from the `transfer` branch. In the other branch it
  is unreachable by construction: it requires an exact name match, so anything it could
  resolve `accountNamedInMessage` already found. Calling it there is dead code that reads
  as live.

`account_name_guess` survives for one job only — naming an account that does **not exist
yet** ("pagué el curso con Brubank") so the gap can offer to create it. It no longer picks
between accounts that already exist, and it is ignored unless the message backs it up.

`userText` is the **turn's** message, not the row's: a message naming one account assigns it
to every row of that turn. Correct for "pagué luz 5000 y gas 3000 con mercado pago", wrong if
the user mixes accounts in one message. No measured case; the UPDATE path
(`movement_update_flow.go`, `accountGapsFor`) does not use any of this.

## One reference resolver, and it has two windows

`resolveCandidates` (`reference_resolution.go`) is the **only** candidate-search mechanism, and
both correction paths share it. Do not add a second. Textual relevance is **scored and ranked** in
Go (`scoreGroup`, accent-folded), never in SQL: a candidate's score is the FRACTION of its own
description tokens the message names, plus a capped tie-break for date proximity. A score of 0
means "no match" and the group never enters — that is what keeps the recency fallback alive, and
a change that lets the date term alone produce a candidate silently deletes it. The cut to five is
by score, not by recency. It also has a shortcut worth knowing: when nothing matches textually but
the user has just recorded something, it returns **exactly one** candidate — the recent entry —
rather than a picker.

It has **two windows, and picking the wrong one is the whole bug class.** With no date it
searches by `created_at` ("what did I just enter"); with a date it searches by *business*
date. A correction naming a date the user entered days later — "el débito del 4 de agosto",
loaded on the 8th — is invisible to the first window and obvious to the second, which is why
`correct_movement` carries `date_from`/`date_to` as a **locator** (they never change the
movement; a date correction travels in `changes` with `field: "date"`). One lone date closes
the window on *both* sides: an open `until` does not narrow anything, and a lone `date_to`
used to invert the window outright.

That rule has a consequence: a relative reference must arrive with **no date at all**, because the
model cannot turn a weekday into a date — `date_from`'s description now asks for a date **only**
when the message spells out day and month. Full measurement, and why `TestAgentDateAnchorEval`
keeps "el lunes" red on purpose, in `docs/decisions.md` (§ The agent loop and QUERY).

When the picker's answer is free text that names none of the options, the loop **searches again**
with the original message plus what the user just typed, instead of re-asking the same question.
That is what `SearchText`/`DateFrom`/`DateTo` are doing in the parked payload: `park` would
otherwise drop all three and there would be nothing to search with. `SearchText` is `e.userText`
and never `Change` — `Change` is the model's paraphrase, and `applyAnswers` concatenates the
user's answers onto it.

## The loop parks, it does not route

There is no router (it was deleted in stage 5). When the loop needs an answer from the user it
parks into a flow and the wizard takes over; `pending_actions` is drained one at a time (WIP=1),
and a flow finishing is the only moment we know nothing else is open.

The wizards themselves are not here — the ones reachable from `manage_settings` live in
`internal/settings`, and every flow is built in `internal/flow`.

## Money

Amounts, signs or `account_id`: `AGENTS.md` § The accounting model, before editing.

Why: `docs/decisions.md`, section **The agent loop and QUERY**.
