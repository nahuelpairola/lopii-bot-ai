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

	// AdminPath is the admin view's segment under AppPrefix. It doubles as the
	// tab key app.js matches against the path (markActiveTab), same as the
	// Tab* consts above — the admin tab reaches the tabbar by an OOB swap
	// rather than by being rendered in it, but once there it is an ordinary tab.
	AdminPath            = "admin"
	AdminInvitationsPath = AdminPath + "/invitations"


	RouteAdmin            = AppPrefix + "/" + AdminPath
	RouteAdminInvitations = AppPrefix + "/" + AdminInvitationsPath

	// Neto* are the KPI status keys shared with overview.go (status color).
	NetoGood     = "good"
	NetoCritical = "critical"
)

// Role* nombran QUÉ es cada serie de un gráfico; el color lo resuelve app.js
// leyendo el acento del tema de Telegram.
//
// Antes acá había hexes (#2a78d6, #1baf7a) y eso era la deuda que DESIGN.md
// llamó The Two Blues Rule: Chart.js no lee variables CSS, así que el color de
// los datos era un azul clavado que no seguía al usuario mientras el resto de
// la UI sí. Se paga no mandando color desde Go.
//
// Asimetría, igual que con el Neto: el gasto sigue al acento del tema, el
// ingreso se queda en su verde porque Telegram no tiene un color positivo.
//
// AccountSlotColors NO entra acá y sigue en hexes: es identidad categórica
// —el color sigue a la cuenta, así su tarjeta y su línea coinciden— y sacar
// ocho tonos distinguibles de un tema arbitrario no tiene solución.
const (
	RoleExpense = "expense"
	RoleIncome  = "income"
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
