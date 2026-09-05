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
)

const categoryParam = "category"

const subcategoryParam = "subcategory"

func (c *controller) handleCategories(ctx *gin.Context) {
	userID := ctx.GetUint64(contextUserIDKey)
	p := periodFromQuery(ctx, templates.SinglePeriodScope)
	drill := ctx.Query(categoryParam)
	sub := ctx.Query(subcategoryParam)

	if drill != "" {
		suffix := "&" + categoryParam + "=" + url.QueryEscape(drill)
		if sub != "" {
			suffix += "&" + subcategoryParam + "=" + url.QueryEscape(sub)
		}
		p = p.WithDrill(suffix)
	}

	if sub != "" && drill != "" {
		c.handleSubcategoryLeaf(ctx, userID, p, drill, sub)
		return
	}

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

	total := decimal.Zero
	for _, r := range rows {
		total = total.Add(r.Total)
	}

	now := nowInART()
	days := decimal.NewFromInt(int64(p.DaysElapsed(now)))

	out := templates.CategoriesData{
		Period:     p,
		Empty:      len(rows) == 0,
		Total:      templates.FormatMoney(total, p.Currency),
		PerDayNote: p.PerDayNote(now),
	}
	labels := make([]string, len(rows))
	values := make([]float64, len(rows))
	for i, r := range rows {
		row := templates.CategoryRow{
			Category: r.Label,
			Total:    templates.FormatMoney(r.Total, p.Currency),
			Share:    sharePercent(r.Total, total),
			PerDay:   templates.FormatMoney(r.Total.Div(days), p.Currency),
		}

		if category == nil {
			row.Href = p.Query() + "&" + categoryParam + "=" + url.QueryEscape(r.Label)
			row.Icon = c.subcategories.IconForCategory(userID, r.Label)
		} else {
			row.Href = p.Query() +
				"&" + categoryParam + "=" + url.QueryEscape(*category) +
				"&" + subcategoryParam + "=" + url.QueryEscape(r.Label)
		}
		out.Rows = append(out.Rows, row)
		labels[i] = r.Label
		f, _ := r.Total.Float64()
		values[i] = f
	}
	out.Chart = templates.BarChartData{Labels: labels, Values: values, Role: templates.RoleExpense}
	return out, nil
}

func sharePercent(v, total decimal.Decimal) string {
	if total.IsZero() {
		return ""
	}
	pct := v.Mul(decimal.NewFromInt(100)).Div(total)
	return strconv.FormatInt(pct.Round(0).IntPart(), 10) + "%"
}

func (c *controller) handleSubcategoryLeaf(ctx *gin.Context, userID uint64, p templates.Period, category, sub string) {
	expenseType := constants.Expense
	q := movement.MovementQuery{
		UserID: userID, From: p.From, To: p.To, Currency: p.Currency,
		Type: &expenseType, Category: &category, Subcategory: &sub,
	}

	movs, err := c.movements.ListForUser(q, movementLeafLimit, 0)
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	totals, err := c.movements.SumForUser(q, "")
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	total := decimal.Zero
	if len(totals) > 0 {
		total = totals[0].Total
	}

	rows := make([]templates.MovementRow, 0, len(movs))
	for _, m := range movs {
		subName := ""
		if m.Subcategory != nil {
			subName = m.Subcategory.Subcategory
		}
		rows = append(rows, templates.MovementRow{

			Title:  templates.RowTitle(m.Description, subName),
			Date:   templates.RowDate(m.Date),
			Amount: templates.FormatMoney(m.Amount.Abs(), p.Currency),
		})
	}

	ctx.Status(http.StatusOK)
	templates.SubcategoryLeaf(templates.SubcategoryLeafData{
		Period:      p,
		Category:    category,
		Subcategory: sub,
		BackQuery:   p.Query() + "&" + categoryParam + "=" + url.QueryEscape(category),
		Rows:        rows,
		Total:       templates.FormatMoney(total, p.Currency),
		Capped:      len(movs) == movementLeafLimit,
		Empty:       len(movs) == 0,
	}).Render(ctx.Request.Context(), ctx.Writer)
}
