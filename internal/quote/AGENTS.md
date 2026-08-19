# internal/quote — traps

Owns two public series: `usd_quotes` (`Quote`) and `monthly_cpi` (`CPI`). Both are written by
`notifier`'s sweeper and, as of today, **read by nobody** — the purchasing-power feature that
will consume them is not built yet. Everything below is aimed at whoever writes that first
reader, because all four traps produce a plausible wrong number rather than an error.

The long "why" for the first three lives in the doc comments on `Quote` and `CPI`
(`model.go`) — read them before touching either type. This file is the list, not a second copy.

## The four

- **A date is resolved with `<= D ORDER BY date DESC LIMIT 1`, never `= D`.** The series is not
  strictly business days and the gaps are irregular — the source carries the last value into some
  weekends and not others. `= D` returns nothing on a perfectly normal Saturday.
- **Display the quote's own date, not the date that was asked for.** A reader that resolves
  2026-08-02 to the 2026-07-31 row and then labels it "2026-08-02" invents a price that never
  traded. That is exactly what v1 did — writer-side gap filling — and why writes here never fill.
- **`CPI.Value` is a monthly percentage change, not an index level, and it can be negative.** A
  deflator is `∏(1 + value/100)` chained across months. A ratio between two rows is meaningless.
  The current month never has a row (INDEC publishes ~2 weeks after close), so any deflation
  anchors on the **last month present in the table**, not on the period's anchor month.
- **`Bid`/`Ask` are the exchange house's side, and the spread can come inverted.** ARS→USD divides
  by `Ask`; USD→ARS multiplies by `Bid`. Normally `Bid <= Ask`, but the source publishes the odd
  row backwards (verified: mayorista 2026-01-13, `compra=1466 venta=1457`), stored as-is. Anything
  computing a spread must tolerate a negative one.

## Writes

Both inserts are idempotent by PK, but for opposite reasons — `InsertQuotes` uses `DoUpdates`
(today's live value from dolarapi is provisional and must be overwritten by argentinadatos a day
or two later), `InsertCPI` uses `DoNothing` (a published month never changes). Swapping either
one silently mixes two sources into one series. The rationale is at each method in
`repository.go`; the seeding/backfill policy is the sweeper's, in `notifier/quotes.go`.
