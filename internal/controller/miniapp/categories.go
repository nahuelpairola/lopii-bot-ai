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

// categoryParam carries the drilled-into category. It rides as a query param,
// not a path segment: real category names contain "/" ("Deudas / préstamos"),
// which no amount of escaping makes safe in a Gin path param.
const categoryParam = "category"

// subcategoryParam abre el último nivel: los movimientos que forman el total de
// una subcategoría. Query param por el mismo motivo que categoryParam.
const subcategoryParam = "subcategory"

// handleCategories serves both the ranking and the subcategory drill — same
// query shape, one extra filter — so the drill keeps the tab highlighted and
// there is no second route to keep in sync.
func (c *controller) handleCategories(ctx *gin.Context) {
	userID := ctx.GetUint64(contextUserIDKey)
	p := periodFromQuery(ctx, templates.SinglePeriodScope)
	drill := ctx.Query(categoryParam)
	sub := ctx.Query(subcategoryParam)

	// Los controles del período tienen que volver al nivel en el que estamos.
	// El sufijo se arma entero de una: WithDrill NO es componible, dos llamadas
	// dejarían el primer param repetido en las flechas.
	if drill != "" {
		suffix := "&" + categoryParam + "=" + url.QueryEscape(drill)
		if sub != "" {
			suffix += "&" + subcategoryParam + "=" + url.QueryEscape(sub)
		}
		p = p.WithDrill(suffix)
	}

	// Tercer nivel: con categoría Y subcategoría, lo que sigue no es otro
	// ranking sino los movimientos que forman ese total.
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

	// No reserved-category filter here: movement.SumForUser excludes them for
	// every caller now. The old post-filter also only worked on the ranking —
	// it matched r.Label against category names, which in the drill are
	// subcategory names, so it never caught anything there.
	total := decimal.Zero
	for _, r := range rows {
		total = total.Add(r.Total)
	}

	out := templates.CategoriesData{
		Period: p,
		Empty:  len(rows) == 0,
		Total:  templates.FormatMoney(total, p.Currency),
	}
	labels := make([]string, len(rows))
	values := make([]float64, len(rows))
	for i, r := range rows {
		row := templates.CategoryRow{
			Category: r.Label,
			Total:    templates.FormatMoney(r.Total, p.Currency),
			Share:    sharePercent(r.Total, total),
		}
		// El ícono es sólo del nivel de arriba: una subcategoría hereda el del
		// padre y repetirlo no informa. El link, en cambio, existe en los dos
		// niveles — la categoría lleva a sus subcategorías, y la subcategoría a
		// los movimientos que la componen.
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

// sharePercent renders a row's slice of the period ("75%"). A zero total means
// there are no rows, so no caller reaches this with one.
func sharePercent(v, total decimal.Decimal) string {
	if total.IsZero() {
		return ""
	}
	pct := v.Mul(decimal.NewFromInt(100)).Div(total)
	return strconv.FormatInt(pct.Round(0).IntPart(), 10) + "%"
}

// handleSubcategoryLeaf sirve el último nivel del drill: los movimientos que
// forman el total de una subcategoría.
//
// El total del pie NO es la suma de las filas listadas: la lista está topeada y
// el número que trajo al usuario acá es el de la fila que tocó, así que sale de
// un SumForUser sin agrupar, sobre todas.
func (c *controller) handleSubcategoryLeaf(ctx *gin.Context, userID uint64, p templates.Period, category, sub string) {
	expenseType := constants.Expense
	q := movement.MovementQuery{
		UserID: userID, From: p.From, To: p.To, Currency: p.Currency,
		Type: &expenseType, Category: &category, Subcategory: &sub,
	}

	movs, err := c.movements.ListForUser(q, movementLeafLimit)
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
			// Sin ícono: en esta hoja todas las filas comparten categoría, así
			// que sería la misma imagen repetida. Y el monto va en positivo,
			// como en todo el resto de la app: son todos gastos.
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
