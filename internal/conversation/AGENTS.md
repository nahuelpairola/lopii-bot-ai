# internal/conversation

The flow engine. Owns `Data`, the state bag every flow reads and writes. Other packages point
here for its contract rather than restating it.

## `Data` round-trips through a JSONB column, and that changes the types

`Data` is `map[string]any` persisted to `conversation_states`. After one save/load cycle:

- numbers come back as **`float64`**, whatever they went in as
- slices come back as **`[]interface{}`**, maps as **`map[string]interface{}`**
- a `nil` `Data` is representable (`data: null` deserializes without error)

**Every numeric field read out of `Data` needs a dual `int`/`float64` type switch**, or it
silently degrades to zero for any value that has survived a round trip. This has already had to
be written twice — `Data.UserID()` (`flow.go`) and `retryCount` (`engine.go`),
both ending in `default: return 0`. A third one written for `int` only will compile, pass a
same-turn test, and return 0 in production.

## Reserved keys are protected by an underscore and nothing else

`UserIDKey`, `retryCountKey` and `ResumeCancelledKey` (`engine.go`) plus `pendingErrorKey`
(`flow.go`) — the four underscore-prefixed keys `_user_id`, `_retry_count`, `_resume_cancelled`
and `_pending_error`. A `Step` whose `DataKey` collides with one of these silently corrupts engine
internals — the resume-gate retry counter, for instance — and no test can anticipate a future
collision.

## `NewFlow`'s graph validation is narrower than it looks

`PossibleNextSteps()` is a **static, self-reported declaration**, not a derived fact
(`flow.go`). `NewFlow` validates only what each `Step` chose to list. In particular,
`TextStep.EscapeOptionsFunc` buttons **must** target `NextStep` or `Finish`
(`text_step.go`) — `PossibleNextSteps()` never inspects them, so a violating button jumps
to an undeclared step with startup validation having given false confidence.

## A `Button` carries either `Data` or `WebAppPath`, never both

Telegram rejects an inline button that sets both `callback_data` and `web_app` with a 400, and
the message never arrives. `Button` has no type or validation stopping you: the adapter
(`internal/messenger/telegram`) branches on `WebAppPath != ""` and drops `Data` when it wins,
so a button carrying both loses its callback silently at render time rather than failing here.
Two tests in that package assert the exclusivity in both directions.

`WebAppPath` is a **path**, not a URL — the adapter prefixes the configured `baseHost`. That is
why `telegram.New` takes a host; a path shipped as a full URL renders as garbage.

## Three hooks, three names, one idea

`ChoiceStep.OnChoice`, `TextStep.OnEscape` (buttons) and `TextStep.OnText` (accepted text) all
mean "transform `Data` on this transition". Pick the wrong name and the compiler stays quiet —
the field simply never fires.

`Engine.resumeLabel` is injected rather than imported because this package cannot import
`messaging` (`engine.go`); a flow with no case in the resolver silently gets a generic
label.

Why: `docs/decisions.md`, section **Conversation engine and flows**.
