package templates

const (
	AppName = "Balance"

	AppPrefix = "/app"

	TabOverview   = "overview"
	TabCategories = "categories"
	TabAccounts   = "accounts"
	TabEvolution  = "evolution"

	RouteOverview   = AppPrefix + "/" + TabOverview
	RouteCategories = AppPrefix + "/" + TabCategories
	RouteAccounts   = AppPrefix + "/" + TabAccounts
	RouteEvolution  = AppPrefix + "/" + TabEvolution

	NetoGood     = "good"
	NetoCritical = "critical"
)

const (
	RoleExpense = "expense"
	RoleIncome  = "income"
)

const (
	CellMild = "cell-mild"
	CellHigh = "cell-high"
)

var AccountSlotColors = []string{"#2a78d6", "#1baf7a", "#eda100", "#008300", "#4a3aa7", "#e34948", "#e87ba4", "#eb6834"}
