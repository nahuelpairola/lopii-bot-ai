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
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

// expandParam names the category whose subcategories show as sub-rows. One at
// a time: the param holds a single name.
const expandParam = "expand"

// mildRatio/highRatio are how far above its own row average a cell has to be
// to earn a shade. Below average gets nothing — overspending is the signal,
// and underspending needs no visual channel of its own.
var (
	mildRatio = decimal.RequireFromString("1.15")
	highRatio = decimal.RequireFromString("1.40")
)

func (c *controller) handleMatrix(ctx *gin.Context) {
	userID := ctx.GetUint64(contextUserIDKey)
	p := periodFromQuery(ctx, templates.TrendPresets, templates.Preset6M)
	expand := ctx.Query(expandParam)

	months := p.MonthKeys()
	expenseType := constants.Expense

	// byCategory[label][monthIndex]. Categories, not subcategories: the global
	// taxonomy has ~65 subcategories and nobody scans that on a phone.
	byCategory := map[string][]decimal.Decimal{}
	var order []string
	// bySubcategory holds the expanded category's breakdown, same shape.
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

	data := templates.MatrixData{
		Period:   p,
		Months:   months,
		Expanded: expand,
		Empty:    len(order) == 0,
		Totals:   monthTotals(order, byCategory, len(months)),
	}
	for _, row := range buildMatrixRows(order, byCategory) {
		row.Icon = c.subcategories.IconForCategory(userID, row.Label)
		if row.Label == expand {
			row.Href = p.Query() // tapping the open row collapses it
			data.Rows = append(data.Rows, row)
			for _, sub := range buildMatrixRows(subOrder, bySubcategory) {
				sub.Sub = true
				data.Rows = append(data.Rows, sub)
			}
			continue
		}
		row.Href = p.Query() + "&" + expandParam + "=" + url.QueryEscape(row.Label)
		data.Rows = append(data.Rows, row)
	}

	ctx.Status(http.StatusOK)
	templates.Matrix(data).Render(ctx.Request.Context(), ctx.Writer)
}

// collectMonth folds one month's grouped rows into the pivot, skipping
// reserved categories and allocating a full-width row the first time a label
// shows up.
func collectMonth(rows []movement.CategorySum, month, months int, into map[string][]decimal.Decimal, order *[]string) {
	for _, r := range rows {
		if subcategory.IsReserved(r.Label) {
			continue
		}
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

// buildMatrixRows turns the pivot into render rows: sorted by period total
// descending (a Go map iterates in random order, so without this the rows
// shuffled between loads) and each cell graded against its own row.
func buildMatrixRows(labels []string, byLabel map[string][]decimal.Decimal) []templates.MatrixRow {
	rows := make([]templates.MatrixRow, 0, len(labels))
	for _, label := range labels {
		cells := byLabel[label]
		row := templates.MatrixRow{Label: label, Cells: make([]templates.MatrixCell, len(cells))}
		for i, v := range cells {
			row.Cells[i] = templates.MatrixCell{
				Value:     templates.FormatCompact(v),
				Intensity: cellIntensity(cells, i),
			}
		}
		rows = append(rows, row)
	}
	slices.SortStableFunc(rows, func(a, b templates.MatrixRow) int {
		return rowTotal(byLabel[b.Label]).Compare(rowTotal(byLabel[a.Label]))
	})
	return rows
}

// cellIntensity grades a cell against its row's average, counting only the
// months that line actually had movement — averaging over empty months would
// read "above average" for anything that started mid-window.
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
		return "" // no "normal" to compare against
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

// monthTotals is the "Total" row: what the whole period cost each month.
func monthTotals(labels []string, byLabel map[string][]decimal.Decimal, months int) []templates.MatrixCell {
	out := make([]templates.MatrixCell, months)
	for i := 0; i < months; i++ {
		t := decimal.Zero
		for _, label := range labels {
			t = t.Add(byLabel[label][i])
		}
		out[i] = templates.MatrixCell{Value: templates.FormatCompact(t)}
	}
	return out
}
