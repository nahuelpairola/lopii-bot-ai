# internal/controller/messaging

The Telegram surface: 52 non-test files, 15 registered flows, 68 `Data` keys. **Use `codegraph_explore`
for structure** — where something is, what calls it. This file is only for what reading the
code will not tell you: rules that compile fine and then behave wrong.

## The `Data` contract

`conversation.Data` round-trips through a JSONB column, so numbers come back as `float64` and
slices as `[]interface{}`. The full contract is in `internal/conversation/CLAUDE.md`; what
matters here is that **you never index `Data` directly**:

| Need | Use | Not |
|---|---|---|
| a string | `conversation.StringOrEmpty(data[k])` | `data[k].(string)` |
| a bool flag | `conversation.Flag(data, k)` / `conversation.SetFlag(data, k)` | `data[k] == true` |
| a `[]string` | `conversation.DecodeStringSlice` / `conversation.EncodeStringSlice` | a plain cast |
| movement rows | `movement.DecodeMovementRows` / `movement.EncodeMovementRows` | a plain cast |
| a copy | `conversation.CopyData` (nil-safe) | `maps.Clone` |

Helpers live in `internal/conversation/data.go` and `internal/movement/rows.go`. Every key is an
exported const in `internal/conversation/data.go` (`conversation.KeyX`) — one const per distinct
*meaning*, even when two strings collide. The reminder-hub keys (`keyHubHasRow` …) are the one
local exception, scoped to `reminder_setup_flow.go`. A structure of your own travels as
**one JSON string**, not nested maps (see `encodeOpenQuestions`).

## Registering a flow touches three places outside its own file

Nothing enforces any of them:

1. `server.go` — `conversationEngine.Register(NewXFlow())`
2. `controller.go` — a case in `handleFlowFinished`'s switch
3. `messages.go` — a case in `FlowResumeLabel`

Miss #1 and the server fails at startup (loud, fine). Miss #2 and the flow completes into
`msgSomethingBroke`. **Miss #3 and nothing breaks until a user goes idle for 24h**, then the
resume gate offers them "una conversación anterior" instead of real copy.

## `callback_data` is 64 bytes — send indices, not labels

Telegram truncates past 64 bytes and the callback stops matching any option, so the button
silently does nothing.

**This is violated today.** `category_picker.go:29` and `movement_create_flow.go:138,181` put
raw category and subcategory names into the callback value, and users create their own
categories. There is no guard anywhere in the repo. `movement_delete_flow.go:38` and
`ask_user_flow.go` show the correct shape: `strconv.Itoa(i)`, resolved back on the other side.

## One entry point, one reference resolver

**There is no router.** `handleFreeText` is one line: every message without an open flow goes to
`startAgentLoop`, and the loop picks one of its 7 tools. The ten-branch switch that used to live
here is gone, and so are `orchestrator.ClassifyIntent`, `ClassifyCreate` and `ResolveDelete`.

That is the whole point of stage 5, not a refactor: routing *first* meant one intent had to be
guessed before anything could be done, and on 2026-08-10 a user asked for one thing — move a
batch of movements to another category — six ways, and each phrasing landed somewhere that had
no way to do it.

The wizards did not go away; they are still the app-side UI. What changed is how you reach them:
the loop **parks** into a flow when it needs an answer, instead of a router deciding up front.

`record_movements` is the only tool that writes. Two rules hang off that:

- **A turn that inserted must never be enqueued on a later 429** (`agentExecutor.wrote`,
  checked in `startAgentLoop`) — the drain would replay it and register the money twice.
- **A CREATE that cannot complete inserts nothing at all.** Gaps park an action that resumes
  into `movement_create`; an overdraft parks one that resumes into `movement_negative_confirm`,
  told apart by `conversation.KeyGatePrompt` in the seed. All-or-nothing per batch is what keeps a two-leg
  transfer from splitting.

`resolveCandidates` (`reference_resolution.go`) is the **only** candidate-search mechanism, and
both correction paths share it. Do not add a second. Textual relevance is decided in Go
(`matchesMessage`, accent-folded), never in SQL. It also has a shortcut worth knowing: when
nothing matches textually but the user has just recorded something, it returns **exactly one**
candidate — the recent entry — rather than a picker.

## The QUERY tools have ONE text filter, and it is not the only text matcher here

`sum_movements` and `list_movements` take a single `search` (since 2026-08-14; it replaced
`category`, `subcategory` and `description`). It matches category name OR subcategory name OR
description, case- and accent-insensitively, **in SQL** via the `unaccent` extension. One
parameter, because the model cannot reliably tell which of the three fields a name lives in:
"lote" reads like a category and actually sits in the description of movements spread across
four subcategories.

**Do not confuse it with `resolveCandidates`.** That one still decides textual relevance in Go
with `foldAccents`, and the rule above — *textual relevance is decided in Go, never in SQL* —
still holds **for reference resolution**. The two answer different questions: `search` filters
an aggregate query, `resolveCandidates` works out which movement a correction refers to.

Three more things that are not obvious from the code:

- `stripLeadingIcon` is still load-bearing, now on `search` alone. The prompt tells the model to
  lead each line with the emoji, `list_categories` returns `"🍔 Alimentación"`, and the model
  copies that whole string into the filter. `unaccent` does not strip emoji.
- `MovementQuery` keeps `Category`/`Subcategory` as **exact** filters. Nothing the model touches
  sets them — the Mini App's drill does, where the name came from a row the user tapped. Exact
  is correct there: a fuzzy match would pull in rows from other subcategories and the leaf would
  stop reconciling against the total that led the user to it.
- **An empty result runs up to two probes** (`describeEmptyResult`) to tell four different facts
  apart. The reserved-category probe is not optional: `apply()` hides `Sistema` and
  `PENDING_REVIEW`, so without it a search for "transferencia" — 12 real movements — would be
  reported as not existing at all, which is worse than the mute zero it replaced.

## The 429 queue has an ordering invariant

`enqueueBehindPending` is called from **`handleConversationInput` only** (`controller.go:244`),
the webhook edge — never from `handleFreeText`. Called from there, a drained replay would
re-enqueue itself forever. The `isReplaying`/`withReplaying` ctx flag is what distinguishes a
real webhook call from a replay; `replayJob` deliberately reuses the *real* handler rather than
a copy, so the two paths can never drift.

## Money, and the rest

Anything touching amounts, signs or `account_id`: read the root `CLAUDE.md` (§ The accounting
model) **before** editing. The anti-pattern list is in `docs/ARCHITECTURE.md`.

Two smaller ones:

- In tests `b` is `nil`. Use `c.sendText(...)`, which guards; a direct `b.SendMessage` panics.
- A subcategory's description is not decorative — it feeds
  `orchestrator.TaxonomyEntry.Description`, i.e. future CREATE classification. That is why the
  description step is mandatory when creating a category.
