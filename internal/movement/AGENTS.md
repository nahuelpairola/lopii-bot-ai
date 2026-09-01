# internal/movement

The money guard lives here. The accounting model itself is in `AGENTS.md` § The accounting
model; below is only what that does not say.

## The guard protects the INSERT path, by caller convention

Nothing in `repository.go` calls `Normalize`. `InsertBatch`, `InsertAccountsWithOpenings` and
`ReplaceMovements` accept a `[]Movement` and trust it has already been through the guard —
which is a pure function the *caller* must invoke (see `flow/movement_write.go`).
A new write path that skips it compiles and inserts unnormalized money.

## `ReassignAccount` upholds a guard invariant in raw SQL

`ReassignAccount` (`repository.go`) is not a plain `UPDATE`. Internal transfers between the
two accounts are soft-deleted **whole**, because re-pointing a single leg would produce a
"transfer from `to` to `to`" — which `validateTransferGroups` rejects at insert time and nothing
re-checks here. That SQL is hand-maintained correctness: editing it can produce data the guard
would have refused.

## `apply`'s reserved filter has THREE branches, and the third is subcategory-precise

Reserved categories are excluded by default and returned alone under `OnlyReserved`. The third
branch is `Type == transfer`: every own-account transfer lives under `Sistema | Transferencia`,
so without it a transfer query selected exactly the rows the exclusion deleted.

**The exemption names the SUBCATEGORY, not the category, and that is load-bearing.**
`Sistema | Saldo inicial` is also `type=transfer`. Widening the exemption to all of `Sistema`
compiles, passes every test, and starts counting each account's opening balance as a transfer —
trading a silent undercount for a silent overcount.

`Sistema | Ajuste de saldo` rows are `expense`/`income`, never `transfer`, so they are unreachable
from this branch and stay out of every P&L query.

## `GroupByDirection` exists so a transfer total is not double

A transfer is two rows and `SumForUser` is `SUM(ABS(amount))`, so any total over both legs is
exactly 2×. `GroupByDirection` splits them into `out` (amount < 0) and `in`. Whoever consumes it
must never add the two rows — they are the same money seen twice.

## Two adjacent finders need opposite time-binding styles

| Function | Column | Bind |
|---|---|---|
| `FindSimilarForUser` | `date` — plain `DATE` | **string** `"2006-01-02"` |
| `FindRecentlyCreatedForUser` | `created_at` — `timestamptz` | `time.Time` directly |

Binding a `time.Time` against `date` makes Postgres cast the column using the *session's*
timezone (UTC), not the ART offset the parameter carries. Every "today" row then falls before an
ART-anchored `since` and **vanishes from the result for part of the day**. It compiles, it runs,
it loses rows.

`FindSimilarForUser`'s `query string` parameter is dead: it stays in the signature only because
removing it ripples through the interface and three test mocks.

## `ListForAccount` breaks two house rules on purpose

It is the one read that does **not** go through `MovementQuery.apply`, and the one preload
that runs `Unscoped`. Both look like oversights and are load-bearing.

**No `apply`.** Its caller is the Mini App's account leaf, which has to reconcile against the
account's balance. `apply` drops `type = transfer` and every reserved category — precisely
the rows that move a balance without being a user-facing expense. Routed through `apply`, the
list silently stops adding up to the number printed above it. Adding a third state to
`apply`'s filters would have served this one view at the risk of its four other consumers;
`SumAmountForAccount` and `MonthlyDeltasForAccount` already sidestep it for the same reason.

**`Unscoped` preload.** The taxonomy was reseeded, so live movements point at soft-deleted
`subcategories` rows — a plain `Preload` returns `nil` for them and the row loses its name in
the UI (it renders as the generic fallback). This is display of an already-assigned
historical value, which is **not** the case the "live rows only" rule governs: that rule is
about *choosing* a subcategory (pickers, taxonomy resolution), where offering a deleted one
is a real bug. Callers must still nil-check `Subcategory`; `Unscoped` shrinks the case, it
does not remove it.

Why: `docs/decisions.md`, section **The money model** and **Movements: mutation and reference resolution**.
