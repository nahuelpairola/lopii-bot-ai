package miniapp

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/movement"
)

// accountSlotColors mirrors the dataviz skill's fixed categorical order —
// each account gets a stable slot by position, so its snapshot and its
// trend line always share the same color (spec §8, "color follows the
// entity").
var accountSlotColors = []string{"#2a78d6", "#1baf7a", "#eda100", "#008300", "#4a3aa7", "#e34948", "#e87ba4", "#eb6834"}

func (c *controller) handleCuentas(ctx *gin.Context) {
	userID := ctx.GetUint64(contextUserIDKey)

	accounts, err := c.accounts.FindByUserID(userID)
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	data := templates.CuentasData{Empty: len(accounts) == 0}
	var allMonths []string
	seenMonths := map[string]bool{}
	var runningByAccount [][]monthBalance

	for _, a := range accounts {
		bal, err := c.movements.SumAmountForAccount(uint64(a.ID))
		if err != nil {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		data.Snapshots = append(data.Snapshots, templates.AccountSnapshot{
			Name: a.Name, Balance: bal.StringFixed(2), Currency: a.Currency.String(),
		})

		deltas, err := c.movements.MonthlyDeltasForAccount(uint64(a.ID))
		if err != nil {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		running := cumulativeBalances(deltas, trendMonths)
		runningByAccount = append(runningByAccount, running)
		for _, m := range running {
			if !seenMonths[m.Month] {
				seenMonths[m.Month] = true
				allMonths = append(allMonths, m.Month)
			}
		}
	}

	var datasets []templates.TrendDataset
	for i, a := range accounts {
		color := accountSlotColors[i%len(accountSlotColors)]
		datasets = append(datasets, templates.TrendDataset{
			Label: a.Name, Data: valuesForRunning(runningByAccount[i], allMonths), BackgroundColor: color,
		})
	}

	data.TrendChart = templates.TrendChartData{Labels: allMonths, Datasets: datasets}

	ctx.Status(http.StatusOK)
	templates.Cuentas(data).Render(ctx.Request.Context(), ctx.Writer)
}

type monthBalance struct {
	Month   string
	Balance float64
}

// cumulativeBalances runs a cumsum over ALL deltas (oldest first, as
// MonthlyDeltasForAccount returns them) and only then keeps the trailing
// `keep` months — cumsumming over the full history avoids the
// opening-balance boundary bug a pre-filtered window would hit (spec §5).
func cumulativeBalances(deltas []movement.MonthlyDelta, keep int) []monthBalance {
	var running float64
	all := make([]monthBalance, len(deltas))
	for i, d := range deltas {
		f, _ := d.Delta.Float64()
		running += f
		all[i] = monthBalance{Month: d.Month, Balance: running}
	}
	if len(all) > keep {
		return all[len(all)-keep:]
	}
	return all
}

func valuesForRunning(running []monthBalance, months []string) []float64 {
	byMonth := map[string]float64{}
	for _, r := range running {
		byMonth[r.Month] = r.Balance
	}
	out := make([]float64, len(months))
	for i, m := range months {
		out[i] = byMonth[m]
	}
	return out
}
