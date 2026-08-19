# Recipes — lopii-finance-bot

> Step-by-step for the common extension points. Conventions they assume live in `AGENTS.md`.

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

A flow is a graph of steps persisting state in `conversation_states`. The graph is validated at
construction — a step referencing a non-existent next step fails the server at startup.

> Read `internal/conversation/AGENTS.md` first for the `Data` contract, and
> `internal/controller/messaging/AGENTS.md` for the local helpers. The traps there are the ones
> that compile.

**1. Step names as constants**, in the target package:
```go
const (
    accountSetupFlowName = "account_setup"
    stepAskName          = "ask_name"
    stepAskCurrency      = "ask_currency"
)
```

**2. Build the Flow.** Steps are **struct literals**, not constructors — there is no
`NewTextStep`/`NewChoiceStep`:
```go
func NewAccountSetupFlow() *conversation.Flow {
    steps := map[string]conversation.Step{
        stepAskName: conversation.TextStep{
            PromptText:    func(conversation.Data) string { return msgAskAccountName },
            DataKey:       keyAccountName,
            NextStep:      stepAskCurrency,
            EscapeOptions: []conversation.ChoiceOption{cancelOption},
        },
        stepAskCurrency: conversation.ChoiceStep{
            PromptText: func(conversation.Data) string { return msgAskCurrency },
            Options: []conversation.ChoiceOption{
                {Label: "ARS", Value: string(currency.ARS), Finish: true},
                {Label: "USD", Value: string(currency.USD), Finish: true},
            },
            OnChoice: func(value string, data conversation.Data) conversation.Data {
                next := copyData(data)
                next[keyAccountCurrency] = value
                return next
            },
        },
    }
    flow, err := conversation.NewFlow(accountSetupFlowName, stepAskName, steps)
    if err != nil {
        panic(err) // graph validation — fails loudly at startup, by design
    }
    return flow
}
```

**3. Wire it in three places.** Nothing enforces any of them:

| Where | What | If you forget |
|---|---|---|
| `server/flows.go` | a line in `registerFlows`: `engine.Register(flow.NewAccountSetupFlow())` | server fails at startup — loud, fine |
| `controller.go` → `handleFlowFinished` | a `case accountSetupFlowName:` | the flow completes into `msgSomethingBroke` |
| `messages.go` → `FlowResumeLabel` | a `case accountSetupFlowName:` | **silent** — broken copy appears only after a user idles 24h |

**4. Start it** with `c.startFlow(...)`, or `engine.StartWithData(userID, name, seed)` when you
have pre-resolved data and want the `SkipIf` walk to land on the first real gap.

**5. Read the result** in your `handleFlowFinished` case — always through the helpers, because
`Data` round-trips through JSONB:
```go
name := stringOrEmpty(data[keyAccountName])   // never data[k].(string)
```

**Cancelar/Atrás on a free-text step:** `TextStep` has `EscapeOptions` + `OnEscape` — buttons
rendered alongside the free-text prompt, checked before text validation. Don't reinvent it per
flow; see `account_create_flow.go`'s `onAccountCreateEscape` and `subcategory_setup_flow.go`.
`TextStep.OnText` is its symmetric hook for accepted *text* (added for the self-looping
`ask_user` step).

### Recipe 3: Add an LLM intent

Intents (`internal/orchestrator/types.go`, all ten): `CREATE | UPDATE | DELETE | QUERY |
ACCOUNT_MANAGE | CREATE_CATEGORY | CATEGORY_MANAGE | REMINDER_SET | HELP | UNCLEAR`

- Tool calling: the LLM constructs action parameters, not just the intent type
- `UPDATE` = atomic `DELETE + INSERT` in a single SQL transaction
- Implicit references ("actually it was 1200") resolve via `resolveCandidates` (in-Go token/amount match over a DB window: recency of entry `created_at`/48h by default, a mentioned date anchors on business `date`), not an in-memory store
- **Migration in progress:** `UPDATE`, `DELETE` and `CREATE` no longer open a flow from the
  router — they go through the unified agent loop (`orchestrator.Run` → `startAgentLoop`),
  stages 2 and 3 of 5. The router still gates what reaches the loop, which is what keeps each
  stage bisectable. The other seven intents are unchanged.
- `CREATE` through the loop is behind **`config.Agent.RouteCreateToLoop`** (`[agent]
  routeCreateToLoop` in the TOMLs). Stages 2 and 3 ship together, so flipping it off is the only
  way left to attribute a `create_inserted` drop to one of the two: off, `CREATE` returns to
  `startMovementCreate` with stage 2 still live. What happens *after* the loop is unchanged —
  a CREATE with gaps parks an action that resumes into the same `movement_create` flow, and an
  overdraft still goes through `movement_negative_confirm`.
- `CREATE_CATEGORY`: the message asks to create a category/subcategory, not to register/correct/delete a movement. No Call 2 — the flow itself (`subcategory_setup`) asks everything it needs via `ChoiceStep`/`TextStep`, unlike CREATE/UPDATE/DELETE which extract structured data from the message via a second LLM call.
- `REMINDER_SET`: the message creates, edits, or turns off the daily expense-logging reminder. Like `CREATE_CATEGORY`, no Call 2 — `reminder_setup` captures the window entirely via `ChoiceStep` presets/custom-text (`parseWindow`, deterministic, no LLM). Consulting the reminder ("¿a qué hora me recordás?") is QUERY, not REMINDER_SET — see `get_reminder` in Recipe: Add a scheduled notification below.

**Tool-schema field contract (read before adding or changing a Call-2 field).** Every `toolSchema.Parameters` is JSON Schema that Groq validates the model's tool-call against. Three coupled rules — `internal/orchestrator/schema_test.go` locks #1 and will fail CI on a violation; run `go test ./internal/orchestrator/`:

1. **Optional field ⇒ null-union type.** Any property *not* in that object's `required` array must be `["<type>", "null"]` (e.g. `["string", "null"]`). The model emits `null` for an absent optional; a bare scalar 400s on it. All movement schemas (`record_movements`, `resolve_and_correct`), `describe_accounts`, `classify_intent`, and the QUERY tools already follow this.
2. **CREATE and UPDATE share one decode struct.** Both `record_movements` (create.go) and `resolve_and_correct` (update.go) unmarshal into `MovementDraft` (`types.go`). Their movement-item schemas must stay identical field-for-field — change one, change the other, or they drift (that drift is exactly how `account_id`/`group` ended up 400-prone in UPDATE while CREATE was fine).
3. **Promoting a field to `required` (or a DB column to NOT NULL) couples four places — coordinate all of them:** (a) the tool schema's `required` array, (b) the decode struct in `types.go`, (c) the app-side guard `movement.Normalize` that would now assume the field present, and (d) any sibling schema sharing the struct (rule #2). Note for `account_id` specifically: making the schema field `required` is *not* how its money invariant is enforced — the LLM's id is never trusted; `movement.Normalize` resolves/validates the account app-side ([never insert without account_id](ARCHITECTURE.md#anti-patterns--what-not-to-do)). So promoting a field is a guard + struct + sibling change, not a one-line schema edit.

### Recipe: Add a scheduled notification

A "scheduled notification" is any proactive system→user Telegram push not triggered by the user's message (e.g. the expense reminder). All of them share one engine: `internal/notifier.Sweeper`, a `time.Ticker` goroutine (`sweeper.Run`) whose `tick` calls one `sweepX` function per notifier.

1. Add your own candidate query + fire condition + guard as a `sweepX(ctx, now)` method on `Sweeper` (see `sweepReminders` in `internal/notifier/sweeper.go`) — this is *not* shared with other notifiers, don't generalize it.
2. Call it from `tick()`, alongside the existing `s.sweepReminders(ctx, now)`.
3. Reuse `s.send(ctx, chatID, text)` to actually push — never call `bot.SendMessage` directly; `send` is the one injected/reachable asset every notifier (and any future admin broadcast) shares.
4. Do not add a shared data table, a notification-type registry, or a templating engine — each notifier owns its own table/columns (or a couple of fields on `users`) and its own message copy.

**Not every sweeper tenant is a notification.** `sweepQuotes`/`sweepCPI` (`internal/notifier/quotes.go`) ride the same ticker but push nothing to anyone — they ingest a public series. A tenant like that skips step 3 entirely and logs its failures with `slog.Error` instead. It also needs a *schedule*, which a notification gets for free from the user's own reminder window: use the `dailyRun` helper — one run at boot, then one per day from `ingestFireMin`, retried on the `retryEvery` floor until it succeeds. Never gate a job on elapsed-time-since-last-attempt alone; that anchors the schedule to process start, so every Render deploy silently moves the hour.

### Recipe 4: Add an admin command

1. Register the handler in `controller/messaging/controller.go` with a prefix match:
```go
b.RegisterHandler(bot.HandlerTypeMessageText, "/new-invite", bot.MatchTypePrefix, handleNewInvite)
```

2. For an admin surface with real auth, add it to the Mini App rather than as a bare HTTP endpoint. Register the route on the `authed` group with `requireAdmin()` (`internal/controller/miniapp/controller.go`), which gates on the `is_admin` flag `authInitData` stamps from the users row:
```go
adminRoutes := authed.Group("", requireAdmin())
adminRoutes.GET("/"+templates.AdminPath, c.handleAdmin)
```
`/app/admin` (the invitations view) is the worked example. Getting the user *to* it is the other half: a webview has no address bar, and the `TabBar` lives in the `Shell`, which renders before auth and so cannot know who is looking. The tab therefore ships from the first authenticated partial (Resumen) as an `hx-swap-oob` element that lands in the empty `AdminTabSlotID` slot the `TabBar` reserves — see `internal/controller/miniapp/AGENTS.md`.

3. **Do not copy `middleware.RequireAdmin`.** It authenticates nothing — it sets `user_id = 1` and calls `Next()`. Its one caller (`POST /admin/users/:telegramID/reset`) is technical debt, not a pattern (`AGENTS.md` § Technical debt).

4. Telegram deep-links: `https://t.me/<bot_username>?start=<CODE>`

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

`pendingjob.HandleGroqError` (`internal/pendingjob/enqueue.go`) is context-aware: on the live webhook path it enqueues a `KindFreeText` job + acks (`AckForWait`, never silent); under drain replay (`pendingjob.IsReplaying(ctx)`) it propagates the error so the drain re-gates `nextDrainAt` and leaves the job in place, instead of re-enqueuing it.

**For a site that must preserve more than raw text** (today: `finishMovementUpdatePickFlow`'s `update_pick`, which needs the chosen candidate's IDs/rows, not just the message) — add a dedicated payload struct + a dedicated `enqueueXIfRateLimited` helper (see `pendingjob.UpdatePickPayload`/`EnqueueUpdatePick`), and a `case KindX:` branch in `pendingjob/drain.go`'s `replayJob` that unmarshals the payload and calls the same function the webhook would have called.

**Never:**
- Skip `pendingjob.HandleGroqError`/the dedicated helper and fall straight to `msgSomethingBroke` on a Groq call — that's the anti-pattern this recipe exists to prevent (see [ARCHITECTURE.md](ARCHITECTURE.md#anti-patterns--what-not-to-do)).
- Add the ordering-invariant guard (`pendingjob.EnqueueBehindPending`) anywhere other than `handleConversationInput` — it must never be reachable from the drain's replay path (see [decisions.md](decisions.md)).
