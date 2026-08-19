# internal/query

The QUERY loop: free-text questions answered read-only, through its own agent loop and its own
model. **Use `codegraph_explore` for structure** — this file is only for what reading the code will
not tell you.

It runs on a separate model **on purpose**: Groq's ceilings are per model, so a question costs the
unified loop's bucket nothing. Folding it back in would move ~800 tokens of read-tool schemas into
exactly the bucket that saturates.

Nothing here writes. There is no `run_sql`, and adding a capability means a new tool plus an
executor case, never a flow.

## The tools have ONE text filter, and it is not the only text matcher in the repo

`sum_movements` and `list_movements` take a single `search` (since 2026-08-14; it replaced
`category`, `subcategory` and `description`). It matches category name OR subcategory name OR
description, case- and accent-insensitively, **in SQL** via the `unaccent` extension. One
parameter, because the model cannot reliably tell which of the three fields a name lives in:
"lote" reads like a category and actually sits in the description of movements spread across
four subcategories.

**Do not confuse it with `resolveCandidates`.** That one still decides textual relevance in Go
with `foldAccents`, and the rule above — *textual relevance is decided in Go, never in SQL* —
still holds **for reference resolution**. The two answer different questions: `search` filters
an aggregate query, `resolveCandidates` works out which movement a correction refers to.

Three more things that are not obvious from the code:

- `stripLeadingIcon` is still load-bearing, now on `search` alone. The prompt tells the model to
  lead each line with the emoji, `list_categories` returns `"🍔 Alimentación"`, and the model
  copies that whole string into the filter. `unaccent` does not strip emoji.
- `MovementQuery` keeps `Category`/`Subcategory` as **exact** filters. Nothing the model touches
  sets them — the Mini App's drill does, where the name came from a row the user tapped. Exact
  is correct there: a fuzzy match would pull in rows from other subcategories and the leaf would
  stop reconciling against the total that led the user to it.
- **An empty result runs up to two probes** (`describeEmptyResult`) to tell four different facts
  apart. The reserved-category probe is not optional: `apply()` hides `Sistema` and
  `PENDING_REVIEW`, so without it a search for "transferencia" — 12 real movements — would be
  reported as not existing at all, which is worse than the mute zero it replaced.
- **`Run` post-processes the model's answer, and that is deliberate.** Two app-owned
  facts are re-attached after narration: the reserved-category verdict
  (`reinstateAppVerdict` — the model once inverted it, telling the user nothing matched while
  the app had said the opposite) and the consulted date window
  (`appendConsultedRange`). Both exist because the model reliably *drops or contradicts* a fact
  the app established with certainty. Do not move either into the prompt: a prompt rule is a
  request, and these two already failed as requests in production.
- **The app sums grouped rows, the model never does** (`groupedTotalLine`). `group_by=type` is
  the one grouping with no total line: `CategorySum.Total` is `SUM(ABS(amount))`, so adding the
  expense row to the income row yields a number that is neither, and the whole point of the line
  is that the model quotes it without checking.

## Money

Amounts come back from the DB signed; everything shown to the model and to the user goes through
`Abs()`, with direction carried by the movement type. See `AGENTS.md` (§ The accounting
model) before touching anything that reads `amount`.

---

**Why the design is this way** — the measurements, incidents and rejected
alternatives behind these rules live in `docs/decisions.md`, section **The agent loop and QUERY**.
Read it before changing a design choice: most were already argued there, with the
production numbers that settled them.
