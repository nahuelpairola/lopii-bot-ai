# internal/controller/miniapp

Telegram Mini App: templ-rendered HTML views, served over HTMX.

## Auth is the Gin route group, and nothing else

`RegisterRoutes` (`controller.go:87+`) builds two groups off the same prefix:

```go
app    := engine.Group(templates.AppPrefix)  // public — static assets only
authed := app.Group("")                      // authInitData middleware
```

Every data view hangs off `authed`. **A route registered on `app.GET(...)` instead of
`authed.GET(...)` compiles, deploys, and serves the user's financial data with no auth check
at all** — the middleware never runs for it. `registerStatic(app)` is deliberately public,
which makes "this one is public too" a precedented mistake.

No type, no naming rule and no test catches this. Adding a view means adding it to `authed`.

## There used to be an admin surface here, and there is not one now

Until 2026-08-25 this package carried `/app/admin` (an invitation-management view), gated by a
second `requireAdmin()` middleware layered on `authed`, reached from the tab bar by an
`hx-swap-oob` anchor the Resumen partial shipped into an empty `<span>` slot the `TabBar`
reserved (`TabBar` renders in the unauthenticated `Shell`, so it cannot itself know who is
viewing). The user asked for it removed entirely, and it is: `admin.go`, its templates, the
`requireAdmin` middleware, the `AdminPath`/`RouteAdmin` consts and the OOB slot are all gone —
not merely unreachable. `invitation.Repository` still exists and is still wired into
`messagingctrl` for `/start` code redemption in chat; only the Mini App surface for *managing*
invitations went away.

**If a future view needs to depend on the viewer, the OOB-into-a-reserved-slot trick is the
pattern that worked**, with one trap worth carrying forward: **htmx discards an OOB element
whose id is absent from the DOM, with no error anywhere.** Rename one side only and the tab
silently never appears — keep the id as a const and assert both halves in tests, the way this
package used to.

A tab delivered this way also arrives *only* with the partial that carries it. That was fine for
Resumen because the Telegram menu button always opens `EntryPath` (`/app/overview`), so every app
open renders it — but a view that needs its own always-present chrome cannot get it this way.

## The movement leaves end each drill, and only one of them may show everything

Two views end in a list of individual movements: `Cuentas → cuenta` (`AccountLeaf`) and
`Categorías → categoría → subcategoría` (`SubcategoryLeaf`). Both hang off the *existing*
handler via a query param — there is no third route.

They look alike and are deliberately not symmetric:

| | Account leaf | Subcategory leaf |
|---|---|---|
| Source | `ListForAccount` — **bypasses `apply`** | `ListForUser` — goes through `apply` |
| Shows | everything, transfers and reserved rows included | expenses only |
| Amount | **signed** (direction is the point) | `Abs()`, like the rest of the app |
| Row note | `Categoría › Subcategoría` | none — it would repeat on all 50 rows |
| Footer | opening + closing balance | the period total |

**Neither footer is the sum of the visible rows**, and that is the whole point: the list is
capped at 50, so summing what is on screen would contradict the figure the user tapped to
get here. Both come from an aggregate over *all* rows (`MonthlyDeltasForAccount` /
`SumForUser`). Deriving them from `Rows` looks simpler and is wrong.

Consequences worth knowing before touching either:

- The account leaf is the **only place in the app where a transfer or a USD purchase is
  visible at all** — `apply` filters `type = transfer` everywhere else. Routing it back
  through `apply` silently empties it of exactly what it exists to show.
- It is also the only place `PENDING_REVIEW` rows surface, rendered as "Sin clasificar".
  That string never reaches the screen raw.
- The leaf and the index share the `SinglePeriodScope` slot, so walking into a leaf keeps
  the range you were reading. The index used to sit in `TrendScope` (no "Mes") and the leaf
  inherited whatever the index had — a statement is read by month, so it now does.
- **A leaf must call `p.WithDrill(suffix)` or its own period controls throw the user out.**
  `periodQuery` encodes the preset scopes plus `m`/`c` and nothing else, so every header
  link — both arrows, every preset chip, both currency chips — used to rebuild a bare index
  URL and the `?account=`/`?category=`/`?subcategory=` param vanished. `WithDrill` appends
  the suffix to those links only; `Query()` stays bare on purpose, because that is the link
  *back out* (`BackQuery`, and the row `Href`s one level up). Two more things it will not do
  for you: it is **not composable** — the third level builds `&category=X&subcategory=Y` in
  one string, since a second call would repeat the first suffix inside
  `PrevQuery`/`NextQuery` — and it is called only **after** the drill target is validated, so
  a link never echoes an id that turned out not to belong to the user.
- The account leaf also sets `Period.HideCurrency`. An account holds one currency, so the
  chip would render an ARS leaf against a USD period — worse than the bug above. The
  subcategory leaf keeps the chips: a category legitimately spans both.

## The preset has TWO slots, and every link must carry both

`p` is the preset of every view whose window reads as "this period" — Resumen, Categorías,
Cuentas and both leaves. `pt` is Evolución's, and **Evolución is its only member**: it is the
one view that cannot render a one-month window, so it is the one view that needs a separate
memory. A `PresetScope` (`templates/period.go`) is the triple "which param + which presets +
which default", and `periodFromQuery` resolves **every** scope on every request, not only the
active one — so no unvalidated value ever travels.

They are two because `periodFromQuery` never 400s: a preset the view does not offer falls
silently to that view's default. With a single slot, Evolución's coercion `month → 6m` got
written back into `#app-state`, the tab bar carried it out on the next tap, and Resumen —which
*does* offer `6m`— accepted it as a user choice. The filter changed by itself and stayed
changed, and the accounts index leaked the same way into the account leaf via `p.Query()`.

**Every link carries every scope.** `periodQuery` writes all of `Period.Presets`; a link built
by hand with only the active param erases the other view's memory on the next tab tap, and
nothing fails — the value simply falls to a default that reads like a choice. `WithPreset`
clones the map before writing: `Period` is a value, its map is not.

**The accounts index pays for being in the single-period scope**, and `accounts.templ` is
where. Its balance cards are today's balance (`SumAmountForAccount`) and ignore the period
entirely; the period only sizes the trend chart, via `cumulativeBalances(deltas, p.Months)`.
At `Months == 1` that chart is one floating dot per account, so the template renders it only
`if data.Period.Months > 1`. A new period-driven element on that page has to decide the same
thing for itself — nothing above the template enforces it.

## Two smaller traps

- `categoryParam` is a **query param, not a path segment**, because category names contain
  slashes ("Deudas / préstamos"). A "RESTful cleanup" to `/categories/:category` silently
  breaks every category with one.
- Reserved categories are excluded **inside `movement.SumForUser`**, not per view. They used to
  be a post-filter each view had to remember, which is exactly how `overview` came to report
  balance adjustments as real spending while `categories` and `evolution` hid them. A new view
  inherits the exclusion for free; set `MovementQuery.OnlyReserved` to get *only* those rows,
  which is what the overview's "Variación de saldos" line does.

Chart math and period arithmetic are self-explanatory and tested — nothing about them here.

## The palette is not ours

**This package's code carries no comments** (2026-08-29): the code is the source of truth for
behaviour, and everything a number in `app.css` used to explain lives in
[DESIGN.md](../../../DESIGN.md) at the repo root — the type ramp, the 44px rule, the token
mapping, the measured contrasts. Read it before changing anything visual, and regenerate it with
`/impeccable document` after you do; it is derived from the shipped artifact and
`.impeccable/design.json` is its sidecar.

The rule it turns on: **the app does not own its palette — Telegram does.** `app.css` remaps 19
pico tokens onto `--tg-theme-*` across **three** selector blocks (light,
`prefers-color-scheme: dark`, `[data-theme=dark]`), because one block either loses to pico on
specificity or wins everywhere and drags one theme's fallbacks over the other's palette — which
looks wrong outside Telegram without anything failing. Every `--tg-*` reference carries a
fallback; `TestAppCSS_EveryTelegramVarHasFallback` enforces it.

Three deliberate exceptions keep a fixed colour, and the reason is what the colour *means*, not
taste: the positive green (Telegram ships `destructive-text-color` and has no positive
counterpart), `AccountSlotColors` (colour is categorical identity — an account's card and its
trend line must match, and eight distinguishable hues cannot be derived from an arbitrary theme),
and pico's own light/dark fallbacks.

Chart.js cannot read CSS variables, so Go sends a **role** and never a colour
(`RoleExpense`/`RoleIncome`); `colorForRole()` in `app.js` resolves it from the computed style.

---

**Why the design is this way** — the measurements, incidents and rejected
alternatives behind these rules live in `docs/decisions.md`, section **The money model**.
Read it before changing a design choice: most were already argued there, with the
production numbers that settled them.
