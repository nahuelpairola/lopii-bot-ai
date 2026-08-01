# internal/controller/messaging

The Telegram surface: 43 files, 15 registered flows, 69 `Data` keys. **Use `codegraph_explore`
for structure** — where something is, what calls it. This file is only for what reading the
code will not tell you: rules that compile fine and then behave wrong.

## The `Data` contract

`conversation.Data` round-trips through a JSONB column, so numbers come back as `float64` and
slices as `[]interface{}`. The full contract is in `internal/conversation/CLAUDE.md`; what
matters here is that **you never index `Data` directly**:

| Need | Use | Not |
|---|---|---|
| a string | `stringOrEmpty(data[k])` | `data[k].(string)` |
| a bool flag | `flag(data, k)` / `setFlag(data, k)` | `data[k] == true` |
| a `[]string` | `decodeStringSlice` / `encodeStringSlice` | a plain cast |
| movement rows | `decodeMovementRows` / `encodeMovementRows` | a plain cast |
| a copy | `copyData` (nil-safe) | `maps.Clone` |

Helpers live in `movement_flow.go:65-159` and `data_keys.go:120-123`. Every key is a const in
`data_keys.go` — one const per distinct *meaning*, even when two strings collide. A structure of
your own travels as **one JSON string**, not nested maps (see `encodeOpenQuestions`).

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

`handleFreeText` runs the router (Call 1) and dispatches per intent. Two exceptions that do not
open a flow: **QUERY** goes to the read-only loop in `query.go`, and **UPDATE, DELETE and
CREATE** go to the unified loop via `startAgentLoop` (stages 2 and 3 of the agent migration).
The router is still the gate that decides what reaches the loop — that is what makes each stage
bisectable. CREATE's leg is behind `controller.routeCreateToLoop`; off, it returns to
`startMovementCreate`.

`record_movements` is the only tool that writes. Two rules hang off that:

- **A turn that inserted must never be enqueued on a later 429** (`agentExecutor.wrote`,
  checked in `startAgentLoop`) — the drain would replay it and register the money twice.
- **A CREATE that cannot complete inserts nothing at all.** Gaps park an action that resumes
  into `movement_create`; an overdraft parks one that resumes into `movement_negative_confirm`,
  told apart by `keyGatePrompt` in the seed. All-or-nothing per batch is what keeps a two-leg
  transfer from splitting.

`resolveCandidates` (`reference_resolution.go`) is the **only** candidate-search mechanism, and
both correction paths share it. Do not add a second. Textual relevance is decided in Go
(`matchesMessage`, accent-folded), never in SQL. It also has a shortcut worth knowing: when
nothing matches textually but the user has just recorded something, it returns **exactly one**
candidate — the recent entry — rather than a picker.

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
