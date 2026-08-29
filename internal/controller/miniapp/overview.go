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
	p := periodFromQuery(ctx, templates.SinglePeriodScope)

	expenseType := constants.Expense
	incomeType := constants.Income
	base := movement.MovementQuery{UserID: userID, From: p.From, To: p.To, Currency: p.Currency}

	expenseQ := base
	expenseQ.Type = &expenseType
	incomeQ := base
	incomeQ.Type = &incomeType

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

		Empty:      expenses.IsZero() && incomes.IsZero() && variation.IsZero(),
		TrendChart: trend,

		HasVariacion: !variation.IsZero(),
		Variacion:    signedMoney(variation, p.Currency),
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

func signedMoney(d decimal.Decimal, cur currency.Currency) string {
	if d.IsPositive() {
		return "+" + templates.FormatMoney(d, cur)
	}
	return templates.FormatMoney(d, cur)
}

func buildTrendChart(expenses, incomes []movement.CategorySum, display func(string) string) templates.TrendChartData {
	keys := unionLabels(expenses, incomes)
	labels := make([]string, len(keys))
	for i, k := range keys {
		labels[i] = display(k)
	}
	return templates.TrendChartData{
		Labels: labels,
		Datasets: []templates.TrendDataset{
			{Label: "Gastos", Data: valuesForLabels(expenses, keys), Role: templates.RoleExpense},
			{Label: "Ingresos", Data: valuesForLabels(incomes, keys), Role: templates.RoleIncome},
		},
	}
}

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
