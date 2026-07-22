package miniapp

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

// categoryParam carries the drilled-into category. It rides as a query param,
// not a path segment: real category names contain "/" ("Deudas / préstamos"),
// which no amount of escaping makes safe in a Gin path param.
const categoryParam = "category"

// handleCategories serves both the ranking and the subcategory drill — same
// query shape, one extra filter — so the drill keeps the tab highlighted and
// there is no second route to keep in sync.
func (c *controller) handleCategories(ctx *gin.Context) {
	userID := ctx.GetUint64(contextUserIDKey)
	p := periodFromQuery(ctx, templates.AllPresets, templates.PresetMonth)
	drill := ctx.Query(categoryParam)

	groupBy := movement.GroupByCategory
	var category *string
	if drill != "" {
		groupBy = movement.GroupBySubcategory
		category = &drill
	}

	data, err := c.buildCategoriesData(userID, p, groupBy, category)
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	data.Drill = drill

	ctx.Status(http.StatusOK)
	if drill != "" {
		templates.SubcategoryDrill(data).Render(ctx.Request.Context(), ctx.Writer)
		return
	}
	templates.Categories(data).Render(ctx.Request.Context(), ctx.Writer)
}

func (c *controller) buildCategoriesData(userID uint64, p templates.Period, groupBy string, category *string) (templates.CategoriesData, error) {
	expenseType := constants.Expense
	q := movement.MovementQuery{
		UserID: userID, From: p.From, To: p.To, Currency: p.Currency, Type: &expenseType,
	}
	if category != nil {
		q.Category = category
	}

	rows, err := c.movements.SumForUser(q, groupBy)
	if err != nil {
		return templates.CategoriesData{}, err
	}

	filtered := rows[:0]
	total := decimal.Zero
	for _, r := range rows {
		if subcategory.IsReserved(r.Label) {
			continue
		}
		filtered = append(filtered, r)
		total = total.Add(r.Total)
	}

	out := templates.CategoriesData{
		Period: p,
		Empty:  len(filtered) == 0,
		Total:  templates.FormatMoney(total, p.Currency),
	}
	labels := make([]string, len(filtered))
	values := make([]float64, len(filtered))
	for i, r := range filtered {
		row := templates.CategoryRow{
			Category: r.Label,
			Total:    templates.FormatMoney(r.Total, p.Currency),
			Share:    sharePercent(r.Total, total),
		}
		// Only the top level drills — and only categories have an icon; a
		// subcategory inherits its parent's, which would just repeat.
		if category == nil {
			row.Href = p.Query() + "&" + categoryParam + "=" + url.QueryEscape(r.Label)
			row.Icon = c.subcategories.IconForCategory(userID, r.Label)
		}
		out.Rows = append(out.Rows, row)
		labels[i] = r.Label
		f, _ := r.Total.Float64()
		values[i] = f
	}
	out.Chart = templates.BarChartData{Labels: labels, Values: values, Color: templates.ColorBar}
	return out, nil
}

// sharePercent renders a row's slice of the period ("75%"). A zero total means
// there are no rows, so no caller reaches this with one.
func sharePercent(v, total decimal.Decimal) string {
	if total.IsZero() {
		return ""
	}
	pct := v.Mul(decimal.NewFromInt(100)).Div(total)
	return strconv.FormatInt(pct.Round(0).IntPart(), 10) + "%"
}
