# internal/subcategory

Taxonomy (categories/subcategories) plus `Cache`, an in-memory copy loaded once at startup and
read by every request.

## Writes go straight to Postgres — the caller must `Reload()`

`Cache.Insert` and `Cache.Delete` (`cache.go`) delegate to the repository and do **not**
touch the in-memory copy. Skipping the follow-up `Reload()` compiles fine and serves stale
taxonomy to **every user in the process** until something else happens to reload. Correct
sequence: `messaging/subcategory_setup_finish.go`.

## `c.global` is shared by every user and never copied

One backing slice serves all users (`cache.go`). Read paths copy before handing out a
pointer (`found := s; return &found`, `cache.go`). A new lookup returning `&c.global[i]`
directly — or ranging and mutating in place — corrupts the taxonomy for the whole process. No
DB write, no error, nothing to notice.

## Same method names, different semantics, in two files

Only the cache is wired in production (`server.go` hands the repository to `NewCache` and keeps
no reference), but the repository methods are exported and easy to reach for by habit:

| Method | `repository.go` | `cache.go` |
|---|---|---|
| `DistinctCategoriesForUser` | SQL `NOT IN` — case-**sensitive** | `IsReserved` — case-**insensitive** |
| `FindByCategoryAndSubcategory` | **takes no userID** — can return another user's row | user-scoped first |

## Not enforced here

`Delete` removes a subcategory regardless of live `movements` pointing at it. The "don't delete
one that's in use" check lives in `messaging`, via `movement.CountBySubcategory`.

---

Why: `docs/decisions.md`, section **Taxonomy**.
