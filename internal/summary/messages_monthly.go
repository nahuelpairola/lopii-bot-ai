package summary

const (
	msgMonthlyHeader    = "📅 <b>%s cerró</b>\n\n"
	msgMonthlyInAndOut  = "Entró <b>%s</b> y gastaste <b>%s</b>.\n"
	msgMonthlyOnlySpent = "Gastaste <b>%s</b>.\n"
	msgMonthlyLeftover  = "Te quedó <b>%s</b>.\n"
	msgMonthlyOverspent = "Se te fueron <b>%s</b> más de los que entraron.\n"
	msgMonthlySpentLess = "<i>Gastaste %s menos que en %s.</i>\n"
	msgMonthlySpentMore = "<i>Gastaste %s más que en %s.</i>\n"
)

const (
	msgMonthlyAccountsHeader = "\n🏦 <b>Cómo terminaron tus cuentas el %d</b>\n"
	msgMonthlyAccountUp      = "%s cerró en <b>%s</b> ↗ %s más que a fin de %s\n"
	msgMonthlyAccountDown    = "%s cerró en <b>%s</b> ↘ %s menos\n"
	msgMonthlyVariation      = "<i>De ese movimiento, %s no es plata que entró: son intereses y correcciones de saldo.</i>\n"
)

const (
	rateTypeMEP  = "bolsa"
	runwayMonths = 3

	msgMonthlyInDollars = "<i>Todo el mes te salió %s, al dólar MEP del %d de %s.</i>\n"
	msgMonthlyUSDHeld   = "\n💵 En dólares tenés <b>%s</b>, %s más que hace un mes.\n"
	msgMonthlyUSDHeldDn = "\n💵 En dólares tenés <b>%s</b>, %s menos que hace un mes.\n"
	msgMonthlyUSDFlat   = "\n💵 En dólares tenés <b>%s</b>.\n"
	msgMonthlyRunway    = "\nEntre todas tus cuentas tenés para <b>%s</b> gastando como venís gastando.\n"
)

const (
	monthlyRankingRows = 5

	msgMonthlyJump      = "\n⚠️ Lo que más cambió: <b>%s</b> te costó <b>%s</b>, %s que en %s.\n"
	msgMonthlyJumpNew   = "\n⚠️ Lo que más cambió: <b>%s</b> te costó <b>%s</b>, y en %s no habías gastado nada ahí.\n"
	msgMonthlyFoldOpen  = "\n<blockquote expandable><b>En qué se fue</b>\n"
	msgMonthlyFoldRow   = "%s %s\n"
	msgMonthlyFoldShare = "%s %s — %s de todo lo que gastaste\n"
	msgMonthlyFoldTop   = "El gasto más caro del mes: %s en %s."
	msgMonthlyFoldClose = "</blockquote>\n"
	msgMonthlyNote      = "\n<blockquote>📩 Va cada día 3, además del resumen de los lunes. Para cortar los dos, escribime «no quiero más los resúmenes».</blockquote>"
	msgMonthlyButton    = "📈 Ver %s en la app"

	monthlyButtonQuery = "?p=month&m="
)
