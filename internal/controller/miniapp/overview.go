package miniapp

import (
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

func (c *controller) handleOverview(ctx *gin.Context) {
	userID := ctx.GetUint64(contextUserIDKey)
	p := periodFromQuery(ctx, templates.AllPresets, templates.PresetMonth)

	expenseType := constants.Expense
	incomeType := constants.Income
	base := movement.MovementQuery{UserID: userID, From: p.From, To: p.To, Currency: p.Currency}

	expenseQ := base
	expenseQ.Type = &expenseType
	incomeQ := base
	incomeQ.Type = &incomeType

	// A one-month window plots daily bars; anything wider plots months. Both
	// shapes carry the same two series — gastos e ingresos, lado a lado.
	groupBy := movement.GroupByMonth
	bucketLabel := templates.ShortMonth
	if p.Months == 1 {
		groupBy = movement.GroupByDay
		bucketLabel = templates.ShortDay
	}

	expenseRows, err := c.movements.SumForUser(expenseQ, movement.GroupByNone)
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	incomeRows, err := c.movements.SumForUser(incomeQ, movement.GroupByNone)
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	expenseByBucket, err := c.movements.SumForUser(expenseQ, groupBy)
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	incomeByBucket, err := c.movements.SumForUser(incomeQ, groupBy)
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	// Lo que los saldos hicieron sin una transacción real detrás: ajustes de
	// saldo y rendimiento de inversión. Va aparte de Gastos/Ingresos a
	// propósito — una revaluación de CEDEARs entra hoy y sale mañana, y
	// contarla como plata ganada o gastada rompe el promedio diario. Type nil
	// descarta los transfer, y con eso caen solos los saldos iniciales y las
	// patas de transferencia, que son plomería que nadie quiere ver.
	variationQ := base
	variationQ.OnlyReserved = true
	variationRows, err := c.movements.SumForUser(variationQ, movement.GroupByType)
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	expenses := sumTotal(expenseRows)
	incomes := sumTotal(incomeRows)
	neto := incomes.Sub(expenses)
	variation := signedVariation(variationRows)

	status := templates.NetoGood
	if neto.IsNegative() {
		status = templates.NetoCritical
	}

	trend := buildTrendChart(expenseByBucket, incomeByBucket, bucketLabel)

	data := templates.OverviewData{
		Period:     p,
		Gastos:     templates.FormatMoney(expenses, p.Currency),
		Ingresos:   templates.FormatMoney(incomes, p.Currency),
		Neto:       templates.FormatMoney(neto, p.Currency),
		NetoStatus: status,
		// Un período cuyo único evento fue un ajuste no está vacío: el saldo se
		// movió. Sin la variación acá, la vista se contradecía sola —
		// mostraba "+$84.200" y justo abajo "Sin movimientos en este período".
		Empty:      expenses.IsZero() && incomes.IsZero() && variation.IsZero(),
		TrendChart: trend,
		// La fila solo existe si hubo variación: en un mes sin ajustes no
		// tiene por qué ocupar lugar ni pedir atención.
		HasVariacion: !variation.IsZero(),
		Variacion:    signedMoney(variation, p.Currency),
		IsAdmin:      ctx.GetBool(contextIsAdminKey),
	}

	ctx.Status(http.StatusOK)
	templates.Overview(data).Render(ctx.Request.Context(), ctx.Writer)
}

func sumTotal(rows []movement.CategorySum) decimal.Decimal {
	if len(rows) == 0 {
		return decimal.Zero
	}
	return rows[0].Total
}

// signedVariation folds the reserved rows, grouped by type, into one signed
// figure. SumForUser returns SUM(ABS(amount)) — the sign is a storage detail
// that never surfaces — so direction has to come back from the type label:
// income is money that appeared in an account, expense money that left it.
func signedVariation(rows []movement.CategorySum) decimal.Decimal {
	v := decimal.Zero
	for _, r := range rows {
		if r.Label == constants.Income {
			v = v.Add(r.Total)
			continue
		}
		v = v.Sub(r.Total)
	}
	return v
}

// signedMoney renders a delta rather than a balance, so a gain carries its "+"
// explicitly. FormatMoney already writes the "-" for a loss.
func signedMoney(d decimal.Decimal, cur currency.Currency) string {
	if d.IsPositive() {
		return "+" + templates.FormatMoney(d, cur)
	}
	return templates.FormatMoney(d, cur)
}

// buildTrendChart merges the two series into one Chart.js-ready shape, aligned
// on the union of bucket labels present in either. display renders a bucket key
// as its axis text; it runs last, so the alignment above works on the raw keys.
func buildTrendChart(expenses, incomes []movement.CategorySum, display func(string) string) templates.TrendChartData {
	keys := unionLabels(expenses, incomes)
	labels := make([]string, len(keys))
	for i, k := range keys {
		labels[i] = display(k)
	}
	return templates.TrendChartData{
		Labels: labels,
		Datasets: []templates.TrendDataset{
			{Label: "Gastos", Data: valuesForLabels(expenses, keys), BackgroundColor: templates.ColorExpense},
			{Label: "Ingresos", Data: valuesForLabels(incomes, keys), BackgroundColor: templates.ColorIncome},
		},
	}
}

// unionLabels returns every bucket key present in either series, oldest first.
// SumForUser orders by total DESC — fine for a category ranking, nonsense for a
// time axis — so the order is rebuilt here. Sorting the keys as strings is the
// chronological sort: "YYYY-MM" and "YYYY-MM-DD" are zero-padded fixed-width.
func unionLabels(a, b []movement.CategorySum) []string {
	seen := map[string]bool{}
	var labels []string
	for _, rows := range [][]movement.CategorySum{a, b} {
		for _, r := range rows {
			if !seen[r.Label] {
				seen[r.Label] = true
				labels = append(labels, r.Label)
			}
		}
	}
	slices.Sort(labels)
	return labels
}

func valuesForLabels(rows []movement.CategorySum, labels []string) []float64 {
	byLabel := map[string]float64{}
	for _, r := range rows {
		f, _ := r.Total.Float64()
		byLabel[r.Label] = f
	}
	out := make([]float64, len(labels))
	for i, l := range labels {
		out[i] = byLabel[l]
	}
	return out
}
