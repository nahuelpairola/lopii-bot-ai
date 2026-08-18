package miniapp

import (
	"net/http"
	"slices"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

// accountParam carries the drilled-into account. Query param y no segmento de
// path, por el mismo criterio que categoryParam en categories.go: el drill vive
// en el mismo handler que el índice, así la pestaña sigue marcada y no hay una
// segunda ruta que mantener en sincronía.
const accountParam = "account"

func (c *controller) handleAccounts(ctx *gin.Context) {
	userID := ctx.GetUint64(contextUserIDKey)
	// El drill cuelga del mismo handler que el índice, como el de categorías.
	// Se desvía ANTES de leer el período: la hoja usa presets distintos.
	if raw := ctx.Query(accountParam); raw != "" {
		c.handleAccountLeaf(ctx, userID, raw)
		return
	}
	p := periodFromQuery(ctx, templates.TrendPresets, templates.Preset6M)

	accounts, err := c.accounts.FindByUserID(userID)
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	// One currency at a time. ARS and USD never share an axis: a balance of
	// USD 500 next to ARS 2.000.000 flattens the line into the baseline, and
	// the two numbers mean nothing to each other anyway.
	accounts = slices.DeleteFunc(accounts, func(a account.Account) bool {
		return a.Currency != p.Currency
	})

	data := templates.AccountsData{Period: p, Empty: len(accounts) == 0}
	var allMonths []string
	seenMonths := map[string]bool{}
	var runningByAccount [][]monthBalance

	for _, a := range accounts {
		bal, err := c.movements.SumAmountForAccount(uint64(a.ID))
		if err != nil {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		data.Snapshots = append(data.Snapshots, templates.AccountSnapshot{
			Name:      a.Name,
			Balance:   templates.FormatMoney(bal, a.Currency),
			IsDefault: a.IsDefault,
			Href:      p.Query() + "&" + accountParam + "=" + strconv.FormatUint(uint64(a.ID), 10),
		})

		deltas, err := c.movements.MonthlyDeltasForAccount(uint64(a.ID))
		if err != nil {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		running := cumulativeBalances(deltas, p.Months)
		runningByAccount = append(runningByAccount, running)
		for _, m := range running {
			if !seenMonths[m.Month] {
				seenMonths[m.Month] = true
				allMonths = append(allMonths, m.Month)
			}
		}
	}

	var datasets []templates.TrendDataset
	for i, a := range accounts {
		color := templates.AccountSlotColors[i%len(templates.AccountSlotColors)]
		datasets = append(datasets, templates.TrendDataset{
			Label: a.Name, Data: valuesForRunning(runningByAccount[i], allMonths), BackgroundColor: color,
		})
	}

	data.TrendChart = templates.TrendChartData{Labels: allMonths, Datasets: datasets}

	ctx.Status(http.StatusOK)
	templates.Accounts(data).Render(ctx.Request.Context(), ctx.Writer)
}

type monthBalance struct {
	Month   string
	Balance float64
}

// cumulativeBalances runs a cumsum over ALL deltas (oldest first, as
// MonthlyDeltasForAccount returns them) and only then keeps the trailing
// `keep` months — cumsumming over the full history avoids the
// opening-balance boundary bug a pre-filtered window would hit (spec §5).
func cumulativeBalances(deltas []movement.MonthlyDelta, keep int) []monthBalance {
	var running float64
	all := make([]monthBalance, len(deltas))
	for i, d := range deltas {
		f, _ := d.Delta.Float64()
		running += f
		all[i] = monthBalance{Month: d.Month, Balance: running}
	}
	if len(all) > keep {
		return all[len(all)-keep:]
	}
	return all
}

func valuesForRunning(running []monthBalance, months []string) []float64 {
	byMonth := map[string]float64{}
	for _, r := range running {
		byMonth[r.Month] = r.Balance
	}
	out := make([]float64, len(months))
	for i, m := range months {
		out[i] = byMonth[m]
	}
	return out
}

// movementLeafLimit es el tope de filas de la hoja, y también el máximo que
// ListForAccount acepta: por encima de eso cae a su default.
const movementLeafLimit = 50

// msgUnclassified nombra en pantalla lo que internamente es PENDING_REVIEW. Esa
// jerga no llega a la vista, pero la fila sí: es plata que movió el saldo, y es
// justo el movimiento que el usuario querría corregir.
const msgUnclassified = "Sin clasificar"

// handleAccountLeaf sirve la hoja de UNA cuenta: los movimientos que explican su
// saldo, transferencias y filas reservadas incluidas.
func (c *controller) handleAccountLeaf(ctx *gin.Context, userID uint64, raw string) {
	// Período propio, no el del índice: el índice usa TrendPresets, que a
	// propósito no ofrece "Mes" (dejaría la tendencia con un solo punto), y un
	// extracto de cuenta es justo lo que se lee por mes.
	p := periodFromQuery(ctx, templates.AllPresets, templates.PresetMonth)

	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	// La cuenta se busca entre las del usuario. El id de la query nunca se usa
	// para consultar directo: es entrada del usuario y apunta a plata.
	accounts, err := c.accounts.FindByUserID(userID)
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	idx := slices.IndexFunc(accounts, func(a account.Account) bool { return uint64(a.ID) == id })
	if idx < 0 {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}
	acc := accounts[idx]

	// Recién ahora, con el id validado: los controles del período tienen que
	// volver a ESTA hoja, no al índice. Va después de la validación para no
	// reflejar en un link un id que resultó no ser del usuario.
	p = p.WithDrill("&" + accountParam + "=" + strconv.FormatUint(id, 10))
	p.HideCurrency = true

	deltas, err := c.movements.MonthlyDeltasForAccount(id)
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	movs, err := c.movements.ListForAccount(id, p.From, p.To, movementLeafLimit)
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	// Apertura y cierre salen del acumulado mensual completo, NO de las filas
	// listadas: con el tope, la suma de lo visible no cerraría.
	fromKey := p.From.Format("2006-01")
	opening := sumDeltas(deltas, func(month string) bool { return month < fromKey })
	inWindow := sumDeltas(deltas, func(month string) bool {
		return month >= fromKey && month <= p.AnchorKey()
	})

	data := templates.AccountLeafData{
		Period:       p,
		AccountName:  acc.Name,
		OpeningLabel: "Saldo al " + p.From.Format("02/01"),
		Opening:      templates.FormatMoney(opening, acc.Currency),
		ClosingLabel: "Saldo al " + p.To.Format("02/01"),
		Closing:      templates.FormatMoney(opening.Add(inWindow), acc.Currency),
		BackQuery:    p.Query(),
		Rows:         movementRows(movs, acc.Currency),
		Capped:       len(movs) == movementLeafLimit,
		Empty:        len(movs) == 0,
	}

	ctx.Status(http.StatusOK)
	templates.AccountLeaf(data).Render(ctx.Request.Context(), ctx.Writer)
}

// sumDeltas suma los deltas cuyos meses cumplen keep. Las claves son "YYYY-MM",
// así que compararlas como strings ordena igual que como fechas.
func sumDeltas(deltas []movement.MonthlyDelta, keep func(month string) bool) decimal.Decimal {
	sum := decimal.Zero
	for _, d := range deltas {
		if keep(d.Month) {
			sum = sum.Add(d.Delta)
		}
	}
	return sum
}

// movementRows arma las filas de la hoja. El monto va CON SIGNO, al revés que en
// todas las otras vistas: acá la dirección es contra esta cuenta, y es justo el
// contenido de la pantalla.
func movementRows(movs []movement.Movement, cur currency.Currency) []templates.MovementRow {
	rows := make([]templates.MovementRow, 0, len(movs))
	for _, m := range movs {
		subName, category := "", ""
		if m.Subcategory != nil {
			subName, category = m.Subcategory.Subcategory, m.Subcategory.Category
		}
		title := templates.RowTitle(m.Description, subName)
		rows = append(rows, templates.MovementRow{
			Icon:   movement.IconForType(m.Type),
			Title:  title,
			Date:   templates.RowDate(m.Date),
			Note:   taxonomyNote(category, subName, title),
			Amount: templates.FormatMoney(m.Amount, cur),
		})
	}
	return rows
}

// taxonomyNote es el "de qué fue" que acompaña a la fecha. En la hoja de una
// CUENTA cada fila cae en una categoría distinta, así que el par identifica el
// gasto tanto como su descripción — al revés que en la hoja de una
// subcategoría, donde el par sería el mismo en las 50 filas y no informa nada.
//
// Se omite cuando el título ya ES la subcategoría (un movimiento sin
// descripción), porque repetirla al lado no agrega nada.
func taxonomyNote(category, subcategory, title string) string {
	if category == constants.PendingReview {
		return msgUnclassified
	}
	if subcategory == "" || subcategory == title {
		return category
	}
	if category == "" {
		return subcategory
	}
	return category + " › " + subcategory
}
