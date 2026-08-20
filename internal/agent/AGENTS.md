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

That rule has a consequence, and **measurement settled how to handle it** (2026-08-19, real
model, `TestAgentDateAnchorEval`): a relative reference must arrive with **no date at all**, so
the search falls back to the `created_at` window where the text match finds it. The model cannot
compute a weekday into a date — given "hoy es miércoles 2026-08-19" it dated "del lunes" (the
17th) as the 15th, then the 14th, and still the 14th with `lunes 2026-08-17` written out in the
prompt. `date_from`'s description now asks for a date **only** when the message spells out day
and month, which it transcribes correctly.

That works for "la semana pasada" and leaves **"el lunes" still sending a wrong date** — the one
red case in the eval, kept red on purpose. Do not widen the window in Go to compensate: that
would undo the 24h margin the 04/08 case needed.

## The loop parks, it does not route

There is no router (it was deleted in stage 5). When the loop needs an answer from the user it
parks into a flow and the wizard takes over; `pending_actions` is drained one at a time (WIP=1),
and a flow finishing is the only moment we know nothing else is open.

The wizards themselves are not here — the ones reachable from `manage_settings` live in
`internal/settings`, and every flow is built in `internal/flow`.

## Money

Anything touching amounts, signs or `account_id`: read `AGENTS.md` (§ The accounting
model) **before** editing. The app owns the sign and the arithmetic; a correction arrives as a
structured diff (`field`/`op`/`value`) and the app computes the result — the model never sends a
number it worked out itself.

---

**Why the design is this way** — the measurements, incidents and rejected
alternatives behind these rules live in `docs/decisions.md`, section **The agent loop and QUERY**.
Read it before changing a design choice: most were already argued there, with the
production numbers that settled them.
