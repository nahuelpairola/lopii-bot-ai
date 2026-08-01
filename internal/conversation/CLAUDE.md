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
be written twice — `Data.UserID()` (`flow.go:150-159`) and `retryCount` (`engine.go:247-256`),
both ending in `default: return 0`. A third one written for `int` only will compile, pass a
same-turn test, and return 0 in production.

## Reserved keys are protected by an underscore and nothing else

`_user_id`, `_retry_count`, `_pending_error`, `_resume_cancelled` (`engine.go:36,46,69`,
`flow.go:166`). A `Step` whose `DataKey` collides with one of these silently corrupts engine
internals — the resume-gate retry counter, for instance — and no test can anticipate a future
collision.

## `NewFlow`'s graph validation is narrower than it looks

`PossibleNextSteps()` is a **static, self-reported declaration**, not a derived fact
(`flow.go:77-80`). `NewFlow` validates only what each `Step` chose to list. In particular,
`TextStep.EscapeOptionsFunc` buttons **must** target `NextStep` or `Finish`
(`text_step.go:39-41`) — `PossibleNextSteps()` never inspects them, so a violating button jumps
to an undeclared step with startup validation having given false confidence.

## Three hooks, three names, one idea

`ChoiceStep.OnChoice`, `TextStep.OnEscape` (buttons) and `TextStep.OnText` (accepted text) all
mean "transform `Data` on this transition". Pick the wrong name and the compiler stays quiet —
the field simply never fires.

`Engine.resumeLabel` is injected rather than imported because this package cannot import
`messaging` (`engine.go:216-220`); a flow with no case in the resolver silently gets a generic
label.
