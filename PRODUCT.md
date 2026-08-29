# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Primary user: the author and a small circle of invited real users (family/friends), all
Argentine, tracking personal finances in ARS and USD. They are on their phone, inside
Telegram, in the seconds right after spending money — "pagué el súper 24.500 con Mercado
Pago" typed one-handed, mid-day, without opening an app or a spreadsheet.

Two distinct jobs, on two different surfaces:

- **Record** (bot chat): get the movement in, correctly, with as little friction as typing
  a message. Ambiguity is resolved by asking, not by a form.
- **Read** (Mini App): answer "how am I doing" — balances per account, spend by category,
  evolution over time — as a statement they can trust and reconcile against.

A second audience exists only as a role, not a segment: `users.is_admin` unlocks admin views
inside the same Mini App.

## Product Purpose

Turn free-text Spanish messages into correctly signed, correctly attributed accounting rows,
and give the user back a readable picture of their money. Success is that a movement gets
recorded right the first time, without the user thinking about accounts, signs, or
categories — and that the numbers in the Mini App reconcile with the real account balances.

## Positioning

**Free text in, a correct accounting row out.** The differentiator is the mechanism behind
that, not the chat surface itself: an LLM agent loop proposes, and the app — never the model —
owns the sign, the arithmetic, and the account attribution. Every movement is signed and
attributed to a real account; corrections arrive as structured diffs (`field`/`op`/`value`)
that the app computes. A competitor can put an LLM behind a chat box; it cannot honestly claim
this without rebuilding the guard, the account-resolution rules, and the all-or-nothing write
path that keeps a two-leg transfer from splitting.

The Argentine context (ARS/USD, USD quotes, monthly CPI, deflated purchasing power) is a real
part of the product, but it is supporting material, not the position.

## Operating Context

- Everything happens inside Telegram: a chat with the bot, plus a Mini App opened from the
  menu button (always at `/app/overview`).
- Mobile webview, one hand, small screen, Spanish (Argentine), often poor connectivity.
- Money moves in two currencies at once; a user commonly holds ARS accounts and USD savings.
- Recording is bursty and conversational: a message may name several movements, may omit the
  account, and may be corrected two messages later ("no, eran 600").
- Reading is periodic: a look at the month, a drill into an account or a category, a check
  of the evolution chart.
- Weekly summaries and nudges arrive as pushed Telegram messages the user did not ask for in
  that moment.

## Capabilities and Constraints

- **Surfaces that matter for design work:** the Telegram Mini App (`internal/controller/miniapp`)
  and the bot's Telegram messages (copy, inline buttons, HTML formatting). A browser web app
  and the Grafana dashboard are explicitly out of scope for design attention.
- **Mini App stack:** Go + Gin, `templ`-rendered HTML, HTMX 2.0.4, Pico CSS, Chart.js — all
  vendored under `internal/controller/miniapp/static`, no CDN, no build step, no JS framework.
- **The shell is unauthenticated.** Telegram hands `initData` to client JS only, so the first
  HTML response carries no user data; `#content` self-loads over HTMX with the
  `X-Telegram-Init-Data` header. Chrome that must exist on every view cannot be decided in the
  shell.
- **Bot message constraint:** the notification sweeper sends with `ParseMode: HTML`, so every
  emitter must be HTML-safe. Inline `callback_data` is capped at 64 bytes.
- **Money:** `shopspring/decimal` always, never `float64`. Amounts are stored signed; the sign
  never escapes storage — user and model both see `amount.Abs()` and direction comes from the
  movement type.
- **Conversation state** lives as JSONB and is only reachable through `conversation.Engine`.
- Groq rate limits (per model, TPM/TPD) are a live product constraint: a rate-limited turn is
  queued and replayed, and the user is always told.
- Undecided: whether this ever opens beyond the invited circle.

## Brand Commitments

- **Name:** Lopii.
- **Language split, non-negotiable:** Telegram UI strings and code comments in Argentine
  Spanish; all code identifiers in English; all documentation (this file included) in English.
- **Voice:** "Lopii, a buen entendedor pocas palabras" — short, plain, Argentine, no
  corporate padding, teaches instead of scolding when it rejects input.
- **Error copy is per-meaning, not generic:** distinct messages for invalid choice, something
  broke, could-not-save, could-not-load, could-not-delete. A single generic error string was
  deliberately removed.

## Evidence on Hand

- Real production data: live users, real movements, `intent_events` / `llm_calls` /
  `request_traces` telemetry, and a Grafana dashboard over it. Product decisions here are
  routinely settled by querying production, not by argument.
- **Metrics tables are for measurement only** — never read `intent_events`, `llm_calls` or
  `request_traces` for feature logic.
- `docs/decisions.md` holds the measurements, incidents and rejected alternatives behind the
  current design; `docs/business-rules.md` holds the full accounting model.
- No testimonials, no pricing, no customer logos, no benchmarks, no public launch. Do not
  invent any.

## Product Principles

1. **The app owns the accounting, the model owns the language.** Sign, arithmetic and account
   attribution are computed in Go from the user's own words — never accepted from the LLM.
2. **A wrong number is worse than a missing one.** When a turn cannot complete, it parks and
   asks; it never writes a half-transfer or guesses an amount.
3. **Never silent.** A queued message, a rate limit, a failed save — the user is told, in
   their own terms, every time.
4. **Recording costs a sentence.** Any design that adds a form, a menu step, or a decision
   before the money is recorded is moving the wrong way.
5. **The read surface must reconcile.** A figure in the Mini App has to add up against the
   real account; totals come from aggregates over all rows, never from what fits on screen.

## Accessibility & Inclusion

No formal standard has been set. Three product-specific requirements hold and must survive:
color is never the only channel for meaning (state colors are reinforcement, glyphs and labels
carry it), money renders with tabular numerals so columns of figures stay comparable, and
interactive elements meet a 44×44 minimum. The surface is a mobile webview at ~360px wide —
that, not desktop, is the design target.

**The screen-reader exemption is lifted, 2026-08-29.** It was recorded on 2026-08-25 on audience
grounds — no current or expected user uses one — and it named exactly two gaps: the four
`<canvas>` charts and the missing `aria-live` region for htmx navigations. Both are closed.

**The audience did not change; the cost-benefit did.** Each fix turned out to pay for a sighted
user too, which is what the 08-25 costing missed:

- The live region was built because htmx swaps announced nothing — and the same work is what
  makes an error audible. Errors reach `#content` through direct `innerHTML`, which never fires
  `htmx:afterSwap`, so the failure path was the one place with no feedback at all. That is a
  "Never silent" violation independent of assistive tech.
- Only two of the four canvases needed a text alternative. The two in Categorías repeat the table
  under them and are `aria-hidden`. The other two are the only copy of their data, and their
  alternative is Spanish prose built by `TrendChartAlt` — text any user can be shown, not an
  assistive-tech-only artifact.

**What is committed and what is not.** Committed: every chart is either marked decorative or
carries a text alternative; anything replacing `#content` announces itself; tables carry
accessible names and row headers; a background tint derives from the text colour so contrast
survives an arbitrary Telegram theme. **Not committed: nothing here has been exercised with an
actual screen reader.** The affordances are covered by Go tests over rendered HTML
(`templates/a11y_test.go`); the JavaScript half — `announce`, `showError` — has no test at all,
because this repo has no JS runner. Treat the screen-reader experience as plausible, not
verified. The formal-standard line above still stands: lifting the exemption committed the app
to specific affordances, not to a conformance target.

The three requirements above were never part of the exemption and never depended on it.
