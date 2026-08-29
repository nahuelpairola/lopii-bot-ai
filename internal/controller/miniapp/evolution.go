package miniapp

import (
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

const expandParam = "expand"

var (
	mildRatio = decimal.RequireFromString("1.15")
	highRatio = decimal.RequireFromString("1.40")
)

func (c *controller) handleEvolution(ctx *gin.Context) {
	userID := ctx.GetUint64(contextUserIDKey)
	p := periodFromQuery(ctx, templates.TrendScope)
	expand := ctx.Query(expandParam)

	months := p.MonthKeys()
	expenseType := constants.Expense

	byCategory := map[string][]decimal.Decimal{}
	var order []string

	bySubcategory := map[string][]decimal.Decimal{}
	var subOrder []string

	for i := range months {
		from := p.From.AddDate(0, i, 0)
		q := movement.MovementQuery{
			UserID:   userID,
			From:     from,
			To:       from.AddDate(0, 1, 0).Add(-time.Nanosecond),
			Currency: p.Currency,
			Type:     &expenseType,
		}

		rows, err := c.movements.SumForUser(q, movement.GroupByCategory)
		if err != nil {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		collectMonth(rows, i, len(months), byCategory, &order)

		if expand != "" {
			subQ := q
			subQ.Category = &expand
			subRows, err := c.movements.SumForUser(subQ, movement.GroupBySubcategory)
			if err != nil {
				ctx.AbortWithStatus(http.StatusInternalServerError)
				return
			}
			collectMonth(subRows, i, len(months), bySubcategory, &subOrder)
		}
	}

	data := templates.EvolutionData{
		Period:   p,
		Months:   months,
		Expanded: expand,
		Empty:    len(order) == 0,
		Totals:   monthTotals(order, byCategory, len(months), p.Currency),
	}
	for _, row := range buildEvolutionRows(order, byCategory, p.Currency) {
		row.Icon = c.subcategories.IconForCategory(userID, row.Label)
		if row.Label == expand {
			row.Href = p.Query()
			data.Rows = append(data.Rows, row)
			for _, sub := range buildEvolutionRows(subOrder, bySubcategory, p.Currency) {
				sub.Sub = true
				data.Rows = append(data.Rows, sub)
			}
			continue
		}
		row.Href = p.Query() + "&" + expandParam + "=" + url.QueryEscape(row.Label)
		data.Rows = append(data.Rows, row)
	}

	ctx.Status(http.StatusOK)
	templates.Evolution(data).Render(ctx.Request.Context(), ctx.Writer)
}

func collectMonth(rows []movement.CategorySum, month, months int, into map[string][]decimal.Decimal, order *[]string) {
	for _, r := range rows {
		if into[r.Label] == nil {
			cells := make([]decimal.Decimal, months)
			for i := range cells {
				cells[i] = decimal.Zero
			}
			into[r.Label] = cells
			*order = append(*order, r.Label)
		}
		into[r.Label][month] = r.Total
	}
}

func buildEvolutionRows(labels []string, byLabel map[string][]decimal.Decimal, cur currency.Currency) []templates.EvolutionRow {
	rows := make([]templates.EvolutionRow, 0, len(labels))
	for _, label := range labels {
		cells := byLabel[label]
		row := templates.EvolutionRow{Label: label, Cells: make([]templates.EvolutionCell, len(cells))}
		for i, v := range cells {
			row.Cells[i] = templates.EvolutionCell{
				Value:     templates.FormatCompact(v, cur),
				Intensity: cellIntensity(cells, i),
			}
		}
		rows = append(rows, row)
	}
	slices.SortStableFunc(rows, func(a, b templates.EvolutionRow) int {
		return rowTotal(byLabel[b.Label]).Compare(rowTotal(byLabel[a.Label]))
	})
	return rows
}

func cellIntensity(cells []decimal.Decimal, i int) string {
	sum := decimal.Zero
	n := 0
	for _, c := range cells {
		if c.IsPositive() {
			sum = sum.Add(c)
			n++
		}
	}
	if n < 2 || !cells[i].IsPositive() {
		return ""
	}
	ratio := cells[i].Div(sum.Div(decimal.NewFromInt(int64(n))))
	switch {
	case ratio.GreaterThan(highRatio):
		return templates.CellHigh
	case ratio.GreaterThan(mildRatio):
		return templates.CellMild
	}
	return ""
}

func rowTotal(cells []decimal.Decimal) decimal.Decimal {
	t := decimal.Zero
	for _, c := range cells {
		t = t.Add(c)
	}
	return t
}

func monthTotals(labels []string, byLabel map[string][]decimal.Decimal, months int, cur currency.Currency) []templates.EvolutionCell {
	out := make([]templates.EvolutionCell, months)
	for i := 0; i < months; i++ {
		t := decimal.Zero
		for _, label := range labels {
			t = t.Add(byLabel[label][i])
		}
		out[i] = templates.EvolutionCell{Value: templates.FormatCompact(t, cur)}
	}
	return out
}
