# internal/controller/miniapp

Telegram Mini App: templ-rendered HTML views, served over HTMX.

## Auth is the Gin route group, and nothing else

`RegisterRoutes` (`controller.go:65-76`) builds two groups off the same prefix:

```go
app    := engine.Group(templates.AppPrefix)  // public — static assets only
authed := app.Group("")                      // authInitData middleware
```

Every data view hangs off `authed`. **A route registered on `app.GET(...)` instead of
`authed.GET(...)` compiles, deploys, and serves the user's financial data with no auth check
at all** — the middleware never runs for it. `registerStatic(app)` is deliberately public,
which makes "this one is public too" a precedented mistake.

No type, no naming rule and no test catches this. Adding a view means adding it to `authed`.

The admin views add a **second** gate on top: `authed.Group("", requireAdmin())`, reading the
`is_admin` flag `authInitData` stamped from the users row. Second, not alternative — an admin
route registered outside `authed` loses initData validation entirely and `requireAdmin` finds
no flag to check, so it 403s everyone. Both have to be there.

## The TabBar cannot see the user

`TabBar` is rendered by the `Shell`, which is the *unauthenticated* response — so a tab that
should only exist for some users cannot be decided there. The admin tab is therefore not
rendered in the tabbar at all: `TabBar` reserves an empty `<span id={ AdminTabSlotID } hidden>`,
and the Resumen partial — the first authenticated response — carries an `hx-swap-oob` anchor
with that same id, which htmx moves into the slot. A non-admin's Resumen simply omits it and the
slot stays empty.

**htmx discards an OOB element whose id is absent from the DOM, with no error anywhere.** Rename
the slot on one side only and the tab silently never appears. That is why the id is a const
(`AdminTabSlotID`) and why `overview_test.go` asserts both halves.

The tab also arrives *only* with Resumen. That is fine because the Telegram menu button always
opens `EntryPath` (`/app/overview`), so every app open renders it — but a view that needs its own
always-present chrome cannot get it this way.

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
