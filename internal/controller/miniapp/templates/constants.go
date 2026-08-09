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
	TabEvolution     = "evolution"

	// Route* are the full link targets (AppPrefix + "/" + tab key).
	RouteOverview   = AppPrefix + "/" + TabOverview
	RouteCategories = AppPrefix + "/" + TabCategories
	RouteAccounts   = AppPrefix + "/" + TabAccounts
	RouteEvolution     = AppPrefix + "/" + TabEvolution

	// AdminPath is the unlisted admin view's segment under AppPrefix. It is
	// deliberately NOT a Tab* const: tab keys exist to be matched against the
	// tabbar, and this route is not in it.
	AdminPath            = "admin"
	AdminInvitationsPath = AdminPath + "/invitations"

	RouteAdmin            = AppPrefix + "/" + AdminPath
	RouteAdminInvitations = AppPrefix + "/" + AdminInvitationsPath

	// Neto* are the KPI status keys shared with overview.go (status color).
	NetoGood     = "good"
	NetoCritical = "critical"
)

// Chart palette (dataviz skill — categorical slots + status hues). Hex for
// Chart.js datasets.
const (
	ColorExpense = "#2a78d6" // primary (also categorical slot 0)
	ColorIncome  = "#1baf7a"
	ColorBar     = "#2a78d6"
)

// Cell* are the evolution shading steps, as CSS classes rather than an inline
// rgba(): the shade means "above this row's own average", which is a two-step
// scale, not a continuous ramp.
const (
	CellMild = "cell-mild"
	CellHigh = "cell-high"
)

// AccountSlotColors gives each account a stable color by position ("color
// follows the entity" — its snapshot and trend line always match).
var AccountSlotColors = []string{"#2a78d6", "#1baf7a", "#eda100", "#008300", "#4a3aa7", "#e34948", "#e87ba4", "#eb6834"}
