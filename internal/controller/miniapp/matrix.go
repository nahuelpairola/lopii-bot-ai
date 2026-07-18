package miniapp

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

func (c *controller) handleMatrix(ctx *gin.Context) {
	userID := ctx.GetUint64(contextUserIDKey)
	expenseType := "expense"
	now := time.Now()

	months := make([]string, trendMonths)
	// bySubcatByMonth[subcategory][monthIndex] = total for that cell.
	bySubcatByMonth := map[string]map[int]float64{}
	var maxTotal float64

	for i := 0; i < trendMonths; i++ {
		monthStart := now.AddDate(0, -(trendMonths - 1 - i), 0)
		from := time.Date(monthStart.Year(), monthStart.Month(), 1, 0, 0, 0, 0, time.UTC)
		to := from.AddDate(0, 1, 0).Add(-time.Second)
		months[i] = from.Format("2006-01")

		rows, err := c.movements.SumForUser(movement.MovementQuery{
			UserID: userID, From: from, To: to, Currency: currency.ARS, Type: &expenseType,
		}, "subcategory")
		if err != nil {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		for _, r := range rows {
			if subcategory.IsReserved(r.Label) {
				continue
			}
			if bySubcatByMonth[r.Label] == nil {
				bySubcatByMonth[r.Label] = map[int]float64{}
			}
			f, _ := r.Total.Float64()
			bySubcatByMonth[r.Label][i] = f
			if f > maxTotal {
				maxTotal = f
			}
		}
	}

	data := templates.MatrixData{Months: months, Empty: len(bySubcatByMonth) == 0}
	for subcat, cellsByMonth := range bySubcatByMonth {
		row := templates.MatrixRow{Subcategory: subcat, Cells: make([]templates.MatrixCell, trendMonths)}
		for i := 0; i < trendMonths; i++ {
			v := cellsByMonth[i]
			intensity := 0.0
			if maxTotal > 0 {
				intensity = v / maxTotal
			}
			row.Cells[i] = templates.MatrixCell{
				Value:     fmt.Sprintf("%.0f", v),
				Intensity: fmt.Sprintf("%.2f", intensity),
			}
		}
		data.Rows = append(data.Rows, row)
	}

	ctx.Status(http.StatusOK)
	templates.Matrix(data).Render(ctx.Request.Context(), ctx.Writer)
}
