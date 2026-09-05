package miniapp

import (
	"net/http"
	"slices"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

const accountParam = "account"

func (c *controller) handleAccounts(ctx *gin.Context) {
	userID := ctx.GetUint64(contextUserIDKey)

	if raw := ctx.Query(accountParam); raw != "" {
		c.handleAccountLeaf(ctx, userID, raw)
		return
	}

	p := periodFromQuery(ctx, templates.SinglePeriodScope)

	accounts, err := c.accounts.FindByUserID(userID)
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	accounts = slices.DeleteFunc(accounts, func(a account.Account) bool {
		return a.Currency != p.Currency
	})

	data := templates.AccountsData{Period: p, Empty: len(accounts) == 0}
	var allMonths []string
	seenMonths := map[string]bool{}
	var runningByAccount [][]monthBalance

	total := decimal.Zero

	for _, a := range accounts {
		bal, err := c.movements.SumAmountForAccount(uint64(a.ID))
		if err != nil {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		total = total.Add(bal)
		data.Snapshots = append(data.Snapshots, templates.AccountSnapshot{
			Name:      a.Name,
			Balance:   templates.FormatMoney(bal, a.Currency),
			IsDefault: a.IsDefault,
			Href:      p.Query() + "&" + accountParam + "=" + strconv.FormatUint(uint64(a.ID), 10),
		})

		deltas, err := c.movements.MonthlyDeltasForAccount(uint64(a.ID))
		if err != nil {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		running := cumulativeBalances(deltas, p.Months)
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
		color := templates.AccountSlotColors[i%len(templates.AccountSlotColors)]
		datasets = append(datasets, templates.TrendDataset{
			Label: a.Name, Data: valuesForRunning(runningByAccount[i], allMonths), BackgroundColor: color,
		})
	}

	data.Total = templates.FormatMoney(total, p.Currency)
	data.TrendChart = templates.TrendChartData{Labels: allMonths, Datasets: datasets}

	ctx.Status(http.StatusOK)
	templates.Accounts(data).Render(ctx.Request.Context(), ctx.Writer)
}

type monthBalance struct {
	Month   string
	Balance float64
}

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

const movementLeafLimit = 50

const msgUnclassified = "Sin clasificar"

const offsetParam = "offset"

func parseOffset(ctx *gin.Context) int {
	n, err := strconv.Atoi(ctx.Query(offsetParam))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func nextOffsetHref(base string, offset int, hasMore bool) string {
	if !hasMore {
		return ""
	}
	return base + "&" + offsetParam + "=" + strconv.Itoa(offset+movementLeafLimit)
}

func (c *controller) handleAccountLeaf(ctx *gin.Context, userID uint64, raw string) {

	p := periodFromQuery(ctx, templates.SinglePeriodScope)
	offset := parseOffset(ctx)

	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	accounts, err := c.accounts.FindByUserID(userID)
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	idx := slices.IndexFunc(accounts, func(a account.Account) bool { return uint64(a.ID) == id })
	if idx < 0 {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}
	acc := accounts[idx]

	p = p.WithDrill("&" + accountParam + "=" + strconv.FormatUint(id, 10))
	p.HideCurrency = true

	movs, err := c.movements.ListForAccount(id, p.From, p.To, movementLeafLimit, offset)
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	hasMore := len(movs) == movementLeafLimit
	rows := movementRows(movs, acc.Currency)
	next := nextOffsetHref(p.Query()+"&"+accountParam+"="+strconv.FormatUint(id, 10), offset, hasMore)

	if offset > 0 {
		ctx.Status(http.StatusOK)
		templates.MovementFragment(rows, next).Render(ctx.Request.Context(), ctx.Writer)
		return
	}

	deltas, err := c.movements.MonthlyDeltasForAccount(id)
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	fromKey := p.From.Format("2006-01")
	opening := sumDeltas(deltas, func(month string) bool { return month < fromKey })
	inWindow := sumDeltas(deltas, func(month string) bool {
		return month >= fromKey && month <= p.AnchorKey()
	})

	data := templates.AccountLeafData{
		Period:       p,
		AccountName:  acc.Name,
		OpeningLabel: "Saldo al " + p.From.Format("02/01"),
		Opening:      templates.FormatMoney(opening, acc.Currency),
		ClosingLabel: "Saldo al " + p.To.Format("02/01"),
		Closing:      templates.FormatMoney(opening.Add(inWindow), acc.Currency),
		BackQuery:    p.Query(),
		Rows:         rows,
		MoreHref:     next,
		Empty:        len(movs) == 0,
	}

	ctx.Status(http.StatusOK)
	templates.AccountLeaf(data).Render(ctx.Request.Context(), ctx.Writer)
}

func sumDeltas(deltas []movement.MonthlyDelta, keep func(month string) bool) decimal.Decimal {
	sum := decimal.Zero
	for _, d := range deltas {
		if keep(d.Month) {
			sum = sum.Add(d.Delta)
		}
	}
	return sum
}

func movementRows(movs []movement.Movement, cur currency.Currency) []templates.MovementRow {
	rows := make([]templates.MovementRow, 0, len(movs))
	for _, m := range movs {
		subName, category := "", ""
		if m.Subcategory != nil {
			subName, category = m.Subcategory.Subcategory, m.Subcategory.Category
		}
		title := templates.RowTitle(m.Description, subName)
		rows = append(rows, templates.MovementRow{
			Icon:   movement.IconForType(m.Type),
			Title:  title,
			Date:   templates.RowDate(m.Date),
			Note:   taxonomyNote(category, subName, title),
			Amount: templates.FormatMoney(m.Amount, cur),
		})
	}
	return rows
}

func taxonomyNote(category, subcategory, title string) string {
	if category == constants.PendingReview {
		return msgUnclassified
	}
	if subcategory == "" || subcategory == title {
		return category
	}
	if category == "" {
		return subcategory
	}
	return category + " › " + subcategory
}
