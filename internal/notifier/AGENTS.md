# internal/notifier

The `Sweeper`: one in-process `time.Ticker` goroutine driving every scheduled job — daily
reminder, weekly summary, trace retention, USD quotes, CPI. **Use `codegraph_explore` for
structure** — this file is only for what reading the code will not tell you.

## Every outbound message goes through ONE `send`, and it is `ParseMode: HTML`

`NewSweeper` builds a single `send` closure (`sweeper.go`) with `ParseMode: models.ParseModeHTML`,
and every job shares it. The weekly summary needs it — it uses `<b>` so the numbers can be scanned
— but the mode is set on the closure, so **it applies to the daily reminder and to anything added
later, whether that copy asked for HTML or not.**

**A raw `<`, `>` or `&` in the text makes Telegram return 400 and the message is not delivered at
all.** Not degraded, not sent unstyled — nothing arrives, and the failure only shows in the logs.
An account named `Ahorro & Cía` is enough to silence a user's whole weekly summary.

So: **any new emitter hanging off the sweeper must escape everything that came from the user
before it reaches `send`.** The summary already does this at the four places user data enters
(`summary.go` — account names, category labels, the top movement's description) via
`html.EscapeString`. Escaping happens in the builder, not in `send`, because `send` cannot tell
the deliberate `<b>` from a user's stray `<`.

The reminder copy is safe by inspection rather than escaping — it is static, with no user data in
it — and `reminder/messages_test.go` guards that it stays that way. If a reminder ever
interpolates a name, that test is the thing that will not catch it: the test checks the constants,
not the rendered message.

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

**Why the design is this way** — the measurements, incidents and rejected
alternatives behind these rules live in `docs/decisions.md`, section **Notifications and reminders**.
Read it before changing a design choice: most were already argued there, with the
production numbers that settled them.
