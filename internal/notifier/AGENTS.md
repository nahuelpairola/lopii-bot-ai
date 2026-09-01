# internal/notifier

The `Sweeper`: one in-process `time.Ticker` goroutine driving every scheduled job — daily
reminder, weekly summary, monthly summary, trace retention, USD quotes, CPI. **Use `codegraph_explore` for
structure** — this file is only for what reading the code will not tell you.

## Every outbound message is `ParseMode: HTML`, set in the adapter now

The sweeper reaches a user through `chatResolver.ChatFor` (`messenger.Chat`), and every send goes
`messenger.SendText(ctx, chat, text)` — **except the monthly summary**, which carries a WebApp
button and therefore cannot: `SendText` is the no-buttons helper by definition, so
`sweepMonthlySummary` calls `chat.Send(ctx, prompt)` with the `conversation.Prompt` the builder
returns. A new emitter that needs a button has to do the same; routing it through `SendText`
compiles and silently drops the keyboard. `ParseMode: models.ParseModeHTML` used to be set on a `send`
closure built in `NewSweeper`; it now lives in `internal/messenger/telegram` (`chat.go`,
`htmlParseMode`) and is global to every message that adapter sends — not something this package
controls or can see. The weekly summary needs it — it uses `<b>` so the numbers can be scanned —
but since the mode is global, **it applies to the daily reminder and to anything added later,
whether that copy asked for HTML or not.**

**A raw `<`, `>` or `&` in the text makes Telegram return 400 and the message is not delivered at
all.** Not degraded, not sent unstyled — nothing arrives, and the failure only shows in the logs.
An account named `Ahorro & Cía` is enough to silence a user's whole weekly summary.

So: **any new emitter hanging off the sweeper must escape everything that came from the user
before it reaches `messenger.SendText`.** The summary already does this at the four places user
data enters (`summary.go` — account names, category labels, the top movement's description) via
`html.EscapeString`. Escaping happens in the builder, not in the adapter, because the adapter
cannot tell the deliberate `<b>` from a user's stray `<`.

The reminder copy is safe by inspection rather than escaping — it is static, with no user data in
it — and `reminder/messages_test.go` guards that it stays that way. If a reminder ever
interpolates a name, that test is the thing that will not catch it: the test checks the constants,
not the rendered message.

## The monthly summary is not opt-in, and that changes where its candidates come from

`sweepWeeklySummary` reads `ListWeeklyDue`, gated on `reminders.weekly_summary_enabled`. The
monthly one is **not gated on anything**: `ListMonthlyDue` returns user ids resolved over
`users` with a `LEFT JOIN` to `reminders`, because most users have no `reminders` row and a
query over that table would silently reach only the few that do. Full reasoning and the
production numbers in `docs/decisions.md` (§ Notifications and reminders).

Two consequences worth carrying:

- **`SetLastMonthlySummaryOn` upserts, and must write exactly one column** — never through the
  general-purpose `Upsert`. When it creates the row, every opt-in flag is written false: the row
  exists to hold a date, not to enable anything.
- **Any copy that turns notifications off must say the monthly keeps coming.** `internal/flow`
  guards this with a test over `MsgReminderAllOff` / `MsgWeeklySummaryOff`; it used to promise
  "todas las notificaciones" and that became a lie the day the monthly stopped being optional.

## The monthly runs before the weekly, and that ordering IS the suppression

On a Monday the 3rd both summaries fire. `tick` calls `sweepMonthlySummary` first and hands its
return value — the set of users it **actually sent to** — to `sweepWeeklySummary` as `skip`.
Reordering the two calls, or dropping the parameter, silently double-messages those users.

The set holds only users that were sent to, never those merely considered: a user whose month
was empty gets no monthly, so their weekly must still go out.

## The ticker is in-process, like the rest of the schedulers

One goroutine, started in `server.InitServer` — the same one-process ceiling as
`messaging.userLocks` and `pendingjob`'s drain gate.

**Send-once is a read-then-write, not a claim.** `ListDue`/`ListWeeklyDue` filter on
`last_reminded_on`/`last_summary_on`, and the sweeper stamps that column *after* `send` returns.
Nothing is reserved in between, so the guarantee holds only because one goroutine walks the list
serially. A second instance would have both sweepers pass the same filter before either writes,
and the user gets the message twice. Making this multi-instance means claiming the row, not adding
another filter.

The order also means a **send that succeeds while its stamp fails re-sends on the next tick** —
deliberate: the log line is there, and a duplicate reminder beats a silent gap.

---

Why: `docs/decisions.md`, section **Notifications and reminders**.
