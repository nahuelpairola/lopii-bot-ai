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

	RouteStatusID = "route-status"

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

const (
	msgPerDayNote          = "Por día = total ÷ %d días corridos, del %d %s al %d %s."
	msgPerDayNoteSameMonth = "Por día = total ÷ %d días corridos, del %d al %d %s."
)

const AssetVersionParam = "v"

var AssetVersion = "dev"

func StaticURL(name string) string {
	return AppPrefix + "/static/" + name + "?" + AssetVersionParam + "=" + AssetVersion
}
