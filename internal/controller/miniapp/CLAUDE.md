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
