package summary

const (
	msgMonthlyHeader    = "📅 <b>Así cerró %s</b>\n\n"
	msgMonthlyInAndOut  = "Entró <b>%s</b>, gastaste <b>%s</b> y te quedaron <b>%s</b>.\n"
	msgMonthlyOnlySpent = "Gastaste <b>%s</b>.\n"
	msgMonthlyOverspent = "Entró <b>%s</b> y gastaste <b>%s</b>: se te fueron <b>%s</b> de más.\n"
	msgMonthlySpentLess = "<i>Gastaste %s menos que en %s.</i>\n"
	msgMonthlySpentMore = "<i>Gastaste %s más que en %s.</i>\n"
	msgMonthlyPerDay    = "<b>VIVIR TE SALIÓ %s POR DÍA</b>\n"
)

const (
	msgMonthlyAccountsHeader = "\n🏦 <b>Tus cuentas al %d de %s</b> <i>(contra el cierre de %s)</i>\n"
	msgMonthlyAccountUp      = "%s <b>%s</b> ↗ %s más\n"
	msgMonthlyAccountDown    = "%s <b>%s</b> ↘ %s menos\n"
	msgMonthlyVariation      = "<i>De esa suba, %s son intereses y ajustes, no plata que entró.</i>\n"
)

const (
	rateTypeMEP  = "bolsa"
	runwayMonths = 3

	maxSpokenTimes  = 10
	maxSpokenMonths = 12

	msgMonthlyInDollars = "<i>Al dólar MEP del %d de %s (%s), el mes te salió %s.</i>\n"
	msgMonthlyUSDHeld   = "\n💵 En dólares tenés <b>%s</b> — %s más que el mes pasado.\n"
	msgMonthlyUSDHeldDn = "\n💵 En dólares tenés <b>%s</b> — %s menos que el mes pasado.\n"
	msgMonthlyUSDFlat   = "\n💵 En dólares tenés <b>%s</b>, igual que el mes pasado.\n"
	msgMonthlyRunway    = "\nCon lo que tenés en las cuentas y gastando así, te alcanza para <b>%s</b>.\n"
)

const (
	monthlyRankingRows = 5

	msgMonthlyJump      = "\n⚠️ <b>%s</b> se te fue para arriba: %s, %s.\n"
	msgMonthlyJumpNew   = "\n⚠️ Apareció <b>%s</b>: %s. En %s no habías gastado nada ahí.\n"
	msgMonthlyFoldOpen  = "\n<blockquote expandable><b>En qué se fue</b>\n"
	msgMonthlyFoldRow   = "%s %s\n"
	msgMonthlyFoldShare = "%s %s — %s de todo lo que gastaste\n"
	msgMonthlyFoldTop   = "Lo más caro del mes: %s, %s."
	msgMonthlyFoldClose = "</blockquote>"
	msgMonthlyButton    = "📈 Ver %s en la app"

	monthlyButtonQuery = "?p=month&m="
)
