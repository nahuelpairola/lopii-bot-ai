# internal/messenger

The neutral surface between lopii and whatever channel it talks over. `internal/messenger/telegram`
is the one adapter today; nothing outside this tree may know `*bot.Bot` exists (bootstrap and the
Mini App are the two carved-out exceptions — see root `AGENTS.md`).

## `SendText` is a package function, not a `Chat` method

`Chat` has two methods: `Send` and `Typing`. `SendText` (`messenger.go`) wraps `Send` with an
empty-button `Prompt` — it is a convenience on top of the interface, not part of it. Putting it on
`Chat` looks harmless and is not: every future adapter (WhatsApp, whatever) would have to
implement the same three-line wrapper again instead of getting it for free. If you're tempted to
add a `SendText` method to the interface because a call site wants one fewer argument, don't —
call the package function.

## The Telegram adapter acks before the per-user lock, and nothing downstream may ack

`Transport.Serve` (`telegram/transport.go`) answers a callback query — stops the button's loading
spinner — **inside its own handler**, before `h(ctx, in)` is ever called. That is deliberate: Telegram
owns the concept of a callback ack, lopii's core does not, and the per-user lock the old code
acked *after* taking added latency to every tap for no reason. Nothing past `toIncoming` may call
`AnswerCallbackQuery` — there is exactly one ack site, and it is this one. A second one (a new
handler, a retry path) double-acks or acks a query the first ack already answered.

## `Incoming` has no `Kind` and no RawText on purpose

Both are derivable from `Input` (`conversation.Input`): a callback carries `CallbackData`, a
message carries `Text`. Adding either field back duplicates state that can drift from `Input` —
compute `Kind`/raw text where they're used (traced), not on the struct.

## Onboarding does not go through `Handler`

`/start CODE` is a Telegram deep-link convention, not something a neutral `messenger.Incoming` can
represent — a WhatsApp equivalent would look nothing like it. It is wired separately via
`Transport.RegisterCommand("/start", messagingController.HandleStart)`, which still takes
`(*bot.Bot, *models.Update)` — the one place outside the adapter that legitimately imports
`go-telegram/bot`. Do not try to fold `/start` into `messenger.Handler`; it does not fit the
abstraction and was never meant to.

## Passing `Channel` around is fine; branching on it is the bug

`Incoming.Channel` and `Chat` travel freely through the core (`user.ChannelTelegram` is stored,
compared for equality against a repo lookup, and so on). What must never happen outside this tree
is an `if`/`switch` that changes *behavior* based on which channel a message came from — that is
exactly the coupling this refactor removed. `grep -rn "Channel ==\|Channel !=" internal/` outside
`internal/messenger/` must stay empty.

## Editing the tapped message is an optional capability, and only a callback chat has it

`EditSource` / `ClearSourceButtons` (`messenger.go`) act on the message whose button was tapped.
They are package functions over an optional `SourceEditor`, not `Chat` methods, for the same reason
as `SendText`: a channel that cannot edit (WhatsApp) falls back to sending a new message, and
clearing becomes a no-op. In Telegram only the `chat` built by `toIncoming` for a callback knows
which message was tapped; a chat from `ChatFor` (sweeper, 429 drain) has none and falls back too.
An edit must send an **explicit empty keyboard** to drop the buttons — leaving the reply markup unset
is not the same.

## `ReplaceTapped` must forward every optional capability, or it hides them

`dispatch` wraps every callback's chat in `ReplaceTapped`, and the optional capabilities above are
found by type assertion — on the **wrapper**, not on what it wraps. So the wrapper re-implements
`SourceEditor` by delegating. Drop it and nothing fails to compile: `EditSource` quietly becomes a
`Send`. A new optional capability needs the same forwarding.

`ClearSourceButtons` does **not** spend the turn's one edit; `EditSource` does. A handler that
clears and then sends gets its send turned into the edit, overwriting the whole card.

Why: `docs/decisions.md`, section **Conversation engine and flows** (one wizard is one card).
