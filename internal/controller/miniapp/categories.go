package miniapp

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

func (c *controller) handleCategories(ctx *gin.Context) {
	userID := ctx.GetUint64(contextUserIDKey)
	data, err := c.buildCategoriesData(userID, movement.GroupByCategory, nil)
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	ctx.Status(http.StatusOK)
	templates.Categories(data).Render(ctx.Request.Context(), ctx.Writer)
}

func (c *controller) handleCategoryDrill(ctx *gin.Context) {
	category := ctx.Param("category")
	userID := ctx.GetUint64(contextUserIDKey)
	data, err := c.buildCategoriesData(userID, movement.GroupBySubcategory, &category)
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	ctx.Status(http.StatusOK)
	templates.SubcategoryDrill(category, data).Render(ctx.Request.Context(), ctx.Writer)
}

func (c *controller) buildCategoriesData(userID uint64, groupBy string, category *string) (templates.CategoriesData, error) {
	expenseType := constants.Expense
	now := time.Now()
	from := now.AddDate(0, -trendMonths, 0)

	q := movement.MovementQuery{UserID: userID, From: from, To: now, Currency: currency.ARS, Type: &expenseType}
	if category != nil {
		q.Category = category
	}

	rows, err := c.movements.SumForUser(q, groupBy)
	if err != nil {
		return templates.CategoriesData{}, err
	}

	filtered := rows[:0]
	for _, r := range rows {
		if subcategory.IsReserved(r.Label) {
			continue
		}
		filtered = append(filtered, r)
	}

	out := templates.CategoriesData{Empty: len(filtered) == 0}
	labels := make([]string, len(filtered))
	values := make([]float64, len(filtered))
	for i, r := range filtered {
		out.Rows = append(out.Rows, templates.CategoryRow{Category: r.Label, Total: r.Total.StringFixed(2)})
		labels[i] = r.Label
		f, _ := r.Total.Float64()
		values[i] = f
	}
	out.Chart = templates.BarChartData{Labels: labels, Values: values, Color: templates.ColorBar}
	return out, nil
}
