package templates

// App-level string constants shared by the templ views and the miniapp
// controller. This is the leaf package (imported by miniapp, importing
// nothing of ours), so routes/keys live here to stay reachable from both the
// templates and the Go handlers without an import cycle.
const (
	// AppName is the page <title> and the Telegram menu-button label (≤16 chars).
	AppName = "Balance"

	AppPrefix = "/app"

	// Tab* are the per-view keys the tabbar highlights against (derived from
	// the request path by the middleware's activeFromPath).
	TabOverview   = "overview"
	TabCategories = "categories"
	TabAccounts   = "accounts"
	TabMatrix     = "matrix"

	// Route* are the full link targets (AppPrefix + "/" + tab key).
	RouteOverview   = AppPrefix + "/" + TabOverview
	RouteCategories = AppPrefix + "/" + TabCategories
	RouteAccounts   = AppPrefix + "/" + TabAccounts
	RouteMatrix     = AppPrefix + "/" + TabMatrix

	// Neto* are the KPI status keys shared with overview.go (status color).
	NetoGood     = "good"
	NetoCritical = "critical"
)
