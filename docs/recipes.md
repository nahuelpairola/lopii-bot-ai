# Recipes — lopii-finance-bot

> Step-by-step for the common extension points. Conventions they assume live in `CLAUDE.md`.

### Recipe 1: Add a DB migration

File name: `migrations/YYYYMMDDHHMMSS_<descriptive_name>.sql`

```sql
-- +goose Up
ALTER TABLE accounts ADD COLUMN alias TEXT;

-- +goose Down
ALTER TABLE accounts DROP COLUMN alias;
```

Create with:
```bash
goose create <descriptive_name> sql -dir ./migrations
```

Migrations run automatically at startup when `runMigrations = true` in the TOML. To run manually:
```bash
goose -dir ./migrations postgres "<connection_string>" up
```

### Recipe 2: Add a conversation flow

A flow is a graph of steps that persists state in `conversation_states`. The graph is validated statically at construction — if a step references a non-existent next step, the server fails to start.

**Steps:**

1. Define step name constants in the target package:
```go
const (
    stepAskName     = "ask_name"
    stepAskCurrency = "ask_currency"
    stepConfirm     = "confirm"
)
```

2. Build the Flow:
```go
func NewAccountSetupFlow(repo accountRepository) *conversation.Flow {
    steps := map[string]conversation.Step{
        stepAskName:     conversation.NewTextStep(...),
        stepAskCurrency: conversation.NewChoiceStep(...),
        stepConfirm:     conversation.NewChoiceStep(...),
    }
    flow, err := conversation.NewFlow("account_setup", stepAskName, steps)
    if err != nil {
        panic(err) // flow graph validation failed at startup
    }
    return flow
}
```

3. Register in `server.go`:
```go
conversationEngine.Register(NewAccountSetupFlow(accountRepo))
```

4. Start from a Telegram handler:
```go
engine.Start(userID, "account_setup")
```

5. Handle the result inside `handleConversationInput` when `result.Finished == true`:
```go
switch result.FlowName {
case "account_setup":
    name := result.Data["account_name"].(string)
    // INSERT into DB
}
```

**Cancelar/Atrás on a free-text step:** `conversation.TextStep` has `EscapeOptions []ChoiceOption` + `OnEscape func(value string, data Data) Data` — buttons rendered alongside the free-text prompt, checked before text validation. This is the existing mechanism, not something to reinvent per flow; see `account_create_flow.go`'s `onAccountCreateEscape` (shared across an entire flow's steps) and `subcategory_setup_flow.go` for reference implementations.

### Recipe 3: Add an LLM intent

Intents (`internal/orchestrator/types.go`): `CREATE | UPDATE | DELETE | QUERY | ACCOUNT_CREATE | CREATE_CATEGORY | REMINDER_SET`
- Tool calling: the LLM constructs action parameters, not just the intent type
- `UPDATE` = atomic `DELETE + INSERT` in a single SQL transaction
- Implicit references ("actually it was 1200") resolve via `resolveCandidates` (in-Go token/amount match over a DB window: recency of entry `created_at`/48h by default, a mentioned date anchors on business `date`), not an in-memory store
- `CREATE_CATEGORY`: the message asks to create a category/subcategory, not to register/correct/delete a movement. No Call 2 — the flow itself (`subcategory_setup`) asks everything it needs via `ChoiceStep`/`TextStep`, unlike CREATE/UPDATE/DELETE which extract structured data from the message via a second LLM call.
- `REMINDER_SET`: the message creates, edits, or turns off the daily expense-logging reminder. Like `CREATE_CATEGORY`, no Call 2 — `reminder_setup` captures the window entirely via `ChoiceStep` presets/custom-text (`parseWindow`, deterministic, no LLM). Consulting the reminder ("¿a qué hora me recordás?") is QUERY, not REMINDER_SET — see `get_reminder` in Recipe: Add a scheduled notification below.

**Tool-schema field contract (read before adding or changing a Call-2 field).** Every `toolSchema.Parameters` is JSON Schema that Groq validates the model's tool-call against. Three coupled rules — `internal/orchestrator/schema_test.go` locks #1 and will fail CI on a violation; run `go test ./internal/orchestrator/`:

1. **Optional field ⇒ null-union type.** Any property *not* in that object's `required` array must be `["<type>", "null"]` (e.g. `["string", "null"]`). The model emits `null` for an absent optional; a bare scalar 400s on it. All movement schemas (`record_movements`, `resolve_and_correct`), `describe_accounts`, `classify_intent`, and the QUERY tools already follow this.
2. **CREATE and UPDATE share one decode struct.** Both `record_movements` (create.go) and `resolve_and_correct` (update.go) unmarshal into `MovementDraft` (`types.go`). Their movement-item schemas must stay identical field-for-field — change one, change the other, or they drift (that drift is exactly how `merchant`/`account_id`/`group` ended up 400-prone in UPDATE while CREATE was fine).
3. **Promoting a field to `required` (or a DB column to NOT NULL) couples four places — coordinate all of them:** (a) the tool schema's `required` array, (b) the decode struct in `types.go`, (c) the app-side guard `normalizeMovements` that would now assume the field present, and (d) any sibling schema sharing the struct (rule #2). Note for `account_id` specifically: making the schema field `required` is *not* how its money invariant is enforced — the LLM's id is never trusted; `normalizeMovements` resolves/validates the account app-side ([never insert without account_id](ARCHITECTURE.md#anti-patterns--what-not-to-do)). So promoting a field is a guard + struct + sibling change, not a one-line schema edit.

### Recipe: Add a scheduled notification

A "scheduled notification" is any proactive system→user Telegram push not triggered by the user's message (e.g. the expense reminder). All of them share one engine: `internal/notifier.Sweeper`, a `time.Ticker` goroutine (`sweeper.Run`) whose `tick` calls one `sweepX` function per notifier.

1. Add your own candidate query + fire condition + guard as a `sweepX(ctx, now)` method on `Sweeper` (see `sweepReminders` in `internal/notifier/sweeper.go`) — this is *not* shared with other notifiers, don't generalize it.
2. Call it from `tick()`, alongside the existing `s.sweepReminders(ctx, now)`.
3. Reuse `s.send(ctx, chatID, text)` to actually push — never call `bot.SendMessage` directly; `send` is the one injected/reachable asset every notifier (and any future admin broadcast) shares.
4. Do not add a shared data table, a notification-type registry, or a templating engine — each notifier owns its own table/columns (or a couple of fields on `users`) and its own message copy.

### Recipe 4: Add an admin command

1. Register the handler in `controller/messaging/controller.go` with a prefix match:
```go
b.RegisterHandler(bot.HandlerTypeMessageText, "/new-invite", bot.MatchTypePrefix, handleNewInvite)
```

2. For HTTP admin endpoints, use the middleware:
```go
r.POST("/invitations", middleware.RequireAdmin(adminID), invitationController.Create)
```

3. Telegram deep-links: `https://t.me/<bot_username>?start=<CODE>`

### Recipe 5: Wire a new Groq-calling site into the pending-jobs queue

Any `internal/controller/messaging` site that calls the orchestrator (a Call 1 or Call 2) can hit a terminal Groq 429 (`orchestrator.RateLimitedError`). It must route the error through the queue instead of showing the generic error copy — otherwise a rate-limited message is silently dropped.

**For a site that replays as free text** (the common case — most Call 2 sites re-run the router path on replay):

```go
res, err := c.orchestrator.SomeCall(ctx, text, ...)
if err != nil {
    if handled, oerr := c.handleGroqError(ctx, b, chatID, userID, text, err); handled {
        return oerr
    }
    c.sendText(ctx, b, chatID, msgSomethingBroke) // unchanged fallback for non-429 errors
    return fmt.Errorf("...: %w", err)
}
```

`handleGroqError` (`internal/controller/messaging/pending_jobs.go`) is context-aware: on the live webhook path it enqueues a `kindFreeText` job + acks (`ackForWait`, never silent); under drain replay (`isReplaying(ctx)`) it propagates the error so the drain re-gates `nextDrainAt` and leaves the job in place, instead of re-enqueuing it.

**For a site that must preserve more than raw text** (today: `finishMovementUpdatePickFlow`'s `update_pick`, which needs the chosen candidate's IDs/rows, not just the message) — add a dedicated payload struct + a dedicated `enqueueXIfRateLimited` helper (see `updatePickPayload`/`enqueueUpdatePickIfRateLimited`), and a `case kindX:` branch in `job_drain.go`'s `replayJob` that unmarshals the payload and calls the same function the webhook would have called.

**Never:**
- Skip `handleGroqError`/the dedicated helper and fall straight to `msgSomethingBroke` on a Groq call — that's the anti-pattern this recipe exists to prevent (see [ARCHITECTURE.md](ARCHITECTURE.md#anti-patterns--what-not-to-do)).
- Add the ordering-invariant guard (`enqueueBehindPending`) anywhere other than `handleConversationInput` — it must never be reachable from the drain's replay path (see [decisions.md](decisions.md)).
