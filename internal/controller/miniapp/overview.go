package miniapp

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/controller/miniapp/templates"
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

	// A one-month window plots daily bars; anything wider plots months.
	// Grouping 31 days into two series would be 62 bars on a phone.
	groupBy := movement.GroupByMonth
	if p.Months == 1 {
		groupBy = movement.GroupByDay
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

	expenses := sumTotal(expenseRows)
	incomes := sumTotal(incomeRows)
	neto := incomes.Sub(expenses)

	status := templates.NetoGood
	if neto.IsNegative() {
		status = templates.NetoCritical
	}

	trend := buildSingleTrend(expenseByBucket)
	if p.Months > 1 {
		incomeByBucket, err := c.movements.SumForUser(incomeQ, groupBy)
		if err != nil {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		trend = buildTrendChart(expenseByBucket, incomeByBucket)
	}

	data := templates.OverviewData{
		Period:     p,
		Gastos:     templates.FormatMoney(expenses, p.Currency),
		Ingresos:   templates.FormatMoney(incomes, p.Currency),
		Neto:       templates.FormatMoney(neto, p.Currency),
		NetoStatus: status,
		Empty:      expenses.IsZero() && incomes.IsZero(),
		TrendChart: trend,
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

// buildSingleTrend is the one-month shape: daily expense bars only. Income in
// a month is one or two events — it belongs in the KPI, not in a daily series.
func buildSingleTrend(expenses []movement.CategorySum) templates.TrendChartData {
	labels := unionLabels(expenses, nil)
	return templates.TrendChartData{
		Labels: labels,
		Datasets: []templates.TrendDataset{
			{Label: "Gastos", Data: valuesForLabels(expenses, labels), BackgroundColor: templates.ColorExpense},
		},
	}
}

// buildTrendChart merges the two series into one Chart.js-ready shape, aligned
// on the union of bucket labels present in either.
func buildTrendChart(expenses, incomes []movement.CategorySum) templates.TrendChartData {
	labels := unionLabels(expenses, incomes)
	return templates.TrendChartData{
		Labels: labels,
		Datasets: []templates.TrendDataset{
			{Label: "Gastos", Data: valuesForLabels(expenses, labels), BackgroundColor: templates.ColorExpense},
			{Label: "Ingresos", Data: valuesForLabels(incomes, labels), BackgroundColor: templates.ColorIncome},
		},
	}
}

func unionLabels(a, b []movement.CategorySum) []string {
	seen := map[string]bool{}
	var labels []string
	for _, r := range a {
		if !seen[r.Label] {
			seen[r.Label] = true
			labels = append(labels, r.Label)
		}
	}
	for _, r := range b {
		if !seen[r.Label] {
			seen[r.Label] = true
			labels = append(labels, r.Label)
		}
	}
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
