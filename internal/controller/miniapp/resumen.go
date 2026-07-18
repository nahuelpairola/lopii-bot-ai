package miniapp

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

const trendMonths = 6

func (c *controller) handleResumen(ctx *gin.Context) {
	userID := ctx.GetUint64(contextUserIDKey)
	cur := currency.ARS // Phase 1: ARS fixed; currency toggle lands in a later task

	now := time.Now()
	from := now.AddDate(0, -trendMonths, 0)

	expenseType := "expense"
	incomeType := "income"

	gastosRows, err := c.movements.SumForUser(movement.MovementQuery{UserID: userID, From: from, To: now, Currency: cur, Type: &expenseType}, "")
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	ingresosRows, err := c.movements.SumForUser(movement.MovementQuery{UserID: userID, From: from, To: now, Currency: cur, Type: &incomeType}, "")
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	gastosByMonth, err := c.movements.SumForUser(movement.MovementQuery{UserID: userID, From: from, To: now, Currency: cur, Type: &expenseType}, "month")
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	ingresosByMonth, err := c.movements.SumForUser(movement.MovementQuery{UserID: userID, From: from, To: now, Currency: cur, Type: &incomeType}, "month")
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	gastos := sumTotal(gastosRows)
	ingresos := sumTotal(ingresosRows)
	neto := ingresos.Sub(gastos)

	status := "good"
	if neto.IsNegative() {
		status = "critical"
	}

	data := templates.ResumenData{
		Gastos:     gastos.StringFixed(2),
		Ingresos:   ingresos.StringFixed(2),
		Neto:       neto.StringFixed(2),
		NetoStatus: status,
		Empty:      gastos.IsZero() && ingresos.IsZero(),
		TrendChart: buildTrendChart(gastosByMonth, ingresosByMonth),
	}

	ctx.Status(http.StatusOK)
	templates.Resumen(data).Render(ctx.Request.Context(), ctx.Writer)
}

func sumTotal(rows []movement.CategorySum) decimal.Decimal {
	if len(rows) == 0 {
		return decimal.Zero
	}
	return rows[0].Total
}

// buildTrendChart merges the two by-month series into one Chart.js-ready
// shape, aligned on the union of month labels present in either series.
func buildTrendChart(gastos, ingresos []movement.CategorySum) templates.TrendChartData {
	labels := unionMonthLabels(gastos, ingresos)
	return templates.TrendChartData{
		Labels: labels,
		Datasets: []templates.TrendDataset{
			{Label: "Gastos", Data: valuesForLabels(gastos, labels), BackgroundColor: "#2a78d6"},
			{Label: "Ingresos", Data: valuesForLabels(ingresos, labels), BackgroundColor: "#1baf7a"},
		},
	}
}

func unionMonthLabels(a, b []movement.CategorySum) []string {
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
