package summary

import (
	"fmt"
	"html"
	"math"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

// BuildMonthly assembles the closed-month summary for a user. from..to is the
// reported month and prevFrom..prevTo the month before it. Returns the zero
// Prompt when the user logged nothing in the reported month.
func (b *Builder) BuildMonthly(userID uint64, from, to, prevFrom, prevTo time.Time) (conversation.Prompt, error) {
	spent, err := b.total(monthQuery(userID, from, to, constants.Expense))
	if err != nil {
		return conversation.Prompt{}, err
	}
	earned, err := b.total(monthQuery(userID, from, to, constants.Income))
	if err != nil {
		return conversation.Prompt{}, err
	}
	if spent.IsZero() && earned.IsZero() {
		return conversation.Prompt{}, nil
	}
	prevSpent, err := b.total(monthQuery(userID, prevFrom, prevTo, constants.Expense))
	if err != nil {
		return conversation.Prompt{}, err
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, msgMonthlyHeader, constants.MonthLongEs[to.Month()-1])
	sb.WriteString(openingLines(spent, earned))
	sb.WriteString(spendComparison(spent, prevSpent, prevTo))

	dollars, err := b.dollarLine(spent, to)
	if err != nil {
		return conversation.Prompt{}, err
	}
	sb.WriteString(dollars)

	accountsBlock, closingARS, err := b.accountsCloseBlock(userID, currency.ARS, to)
	if err != nil {
		return conversation.Prompt{}, err
	}
	sb.WriteString(accountsBlock)

	if accountsBlock != "" {
		v, err := b.variation(userID, currency.ARS, from, to)
		if err != nil {
			return conversation.Prompt{}, err
		}
		if !v.IsZero() {
			fmt.Fprintf(&sb, msgMonthlyVariation, money(v.Abs(), currency.ARS))
		}
	}
	_, closingUSD, err := b.accountsCloseBlock(userID, currency.USD, to)
	if err != nil {
		return conversation.Prompt{}, err
	}
	_, prevClosingUSD, err := b.accountsCloseBlock(userID, currency.USD, prevTo)
	if err != nil {
		return conversation.Prompt{}, err
	}
	if closingUSD.IsPositive() {
		diff := closingUSD.Sub(prevClosingUSD)
		switch {
		case diff.IsZero():
			fmt.Fprintf(&sb, msgMonthlyUSDFlat, money(closingUSD, currency.USD))
		case diff.IsPositive():
			fmt.Fprintf(&sb, msgMonthlyUSDHeld, money(closingUSD, currency.USD), money(diff, currency.USD))
		default:
			fmt.Fprintf(&sb, msgMonthlyUSDHeldDn, money(closingUSD, currency.USD), money(diff.Abs(), currency.USD))
		}
	}

	runway, err := b.runwayLine(userID, closingARS, from)
	if err != nil {
		return conversation.Prompt{}, err
	}
	sb.WriteString(runway)

	cats, err := b.monthlyCategories(userID, from, to)
	if err != nil {
		return conversation.Prompt{}, err
	}
	prevCats, err := b.monthlyCategories(userID, prevFrom, prevTo)
	if err != nil {
		return conversation.Prompt{}, err
	}
	sb.WriteString(monthlyJumpLine(cats, prevCats, prevTo))

	fold, err := b.monthlyFold(userID, cats, spent, from, to)
	if err != nil {
		return conversation.Prompt{}, err
	}
	sb.WriteString(fold)
	sb.WriteString(msgMonthlyNote)

	return conversation.Prompt{
		Text: sb.String(),
		Buttons: []conversation.Button{{
			Label:      fmt.Sprintf(msgMonthlyButton, constants.MonthLongEs[to.Month()-1]),
			WebAppPath: templates.RouteOverview + monthlyButtonQuery + to.Format("2006-01"),
		}},
	}, nil
}

func monthQuery(userID uint64, from, to time.Time, typ string) movement.MovementQuery {
	t := typ
	return movement.MovementQuery{UserID: userID, From: from, To: to, Currency: currency.ARS, Type: &t}
}

func openingLines(spent, earned decimal.Decimal) string {
	if earned.IsZero() {
		return fmt.Sprintf(msgMonthlyOnlySpent, money(spent, currency.ARS))
	}
	left := earned.Sub(spent)
	if left.IsNegative() {
		return fmt.Sprintf(msgMonthlyOverspent,
			money(earned, currency.ARS), money(spent, currency.ARS), money(left.Abs(), currency.ARS))
	}
	return fmt.Sprintf(msgMonthlyInAndOut,
		money(earned, currency.ARS), money(spent, currency.ARS), money(left, currency.ARS))
}

func spendComparison(spent, prevSpent decimal.Decimal, prevTo time.Time) string {
	if prevSpent.IsZero() {
		return ""
	}
	diff := roundNice(spent.Sub(prevSpent).Abs(), currency.ARS)
	if diff.IsZero() {
		return ""
	}
	tpl := msgMonthlySpentMore
	if spent.LessThan(prevSpent) {
		tpl = msgMonthlySpentLess
	}
	return fmt.Sprintf(tpl, money(diff, currency.ARS), constants.MonthLongEs[prevTo.Month()-1])
}

func balanceAt(deltas []movement.MonthlyDelta, month string) (decimal.Decimal, decimal.Decimal) {
	closing := decimal.Zero
	delta := decimal.Zero
	for _, d := range deltas {
		if d.Month > month {
			break
		}
		closing = closing.Add(d.Delta)
		if d.Month == month {
			delta = d.Delta
		}
	}
	return closing, delta
}

func (b *Builder) accountsCloseBlock(userID uint64, cur currency.Currency, to time.Time) (string, decimal.Decimal, error) {
	accts, err := b.accounts.FindByUserID(userID)
	if err != nil {
		return "", decimal.Zero, err
	}
	month := to.Format("2006-01")
	prevMonth := constants.MonthLongEs[time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, to.Location()).AddDate(0, 0, -1).Month()-1]
	totalClosing := decimal.Zero
	var lines []string
	for _, a := range accts {
		if a.Currency != cur {
			continue
		}
		deltas, err := b.movements.MonthlyDeltasForAccount(uint64(a.ID))
		if err != nil {
			return "", decimal.Zero, err
		}
		closing, delta := balanceAt(deltas, month)
		totalClosing = totalClosing.Add(closing)
		if delta.IsZero() {
			continue
		}
		name := html.EscapeString(a.Name)
		tpl := msgMonthlyAccountUp
		if delta.IsNegative() {
			tpl = msgMonthlyAccountDown
		}
		lines = append(lines, fmt.Sprintf(tpl, name, money(closing, cur), money(delta.Abs(), cur)))
	}
	if len(lines) == 0 {
		return "", totalClosing, nil
	}
	header := fmt.Sprintf(msgMonthlyAccountsHeader,
		lastDayOfMonth(to), constants.MonthLongEs[to.Month()-1], prevMonth)
	return header + strings.Join(lines, ""), totalClosing, nil
}

func lastDayOfMonth(t time.Time) int {
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
}

func (b *Builder) dollarLine(spent decimal.Decimal, to time.Time) (string, error) {
	q, err := b.quotes.FindRateOnOrBefore(to, rateTypeMEP)
	if err != nil {
		return "", err
	}
	if q == nil || !q.Ask.IsPositive() {
		return "", nil
	}
	usd := spent.Div(q.Ask).Round(0)
	return fmt.Sprintf(msgMonthlyInDollars,
		q.Date.Day(), constants.MonthLongEs[q.Date.Month()-1], roundedUSD(usd)), nil
}

// roundedUSD renders an approximate dollar figure without the cents that
// money() always prints.
func roundedUSD(d decimal.Decimal) string {
	return "US$" + currency.GroupThousands(d.StringFixed(0))
}

func (b *Builder) runwayLine(userID uint64, closingARS decimal.Decimal, from time.Time) (string, error) {
	if !closingARS.IsPositive() {
		return "", nil
	}
	sum := decimal.Zero
	for i := 0; i < runwayMonths; i++ {
		start := from.AddDate(0, -i, 0)
		end := time.Date(start.Year(), start.Month()+1, 0, 0, 0, 0, 0, start.Location())
		spent, err := b.total(monthQuery(userID, start, end, constants.Expense))
		if err != nil {
			return "", err
		}
		if spent.IsZero() {
			return "", nil
		}
		sum = sum.Add(spent)
	}
	avg := sum.Div(decimal.NewFromInt(runwayMonths))
	if !avg.IsPositive() {
		return "", nil
	}
	months, _ := closingARS.Div(avg).Float64()
	return fmt.Sprintf(msgMonthlyRunway, runwayWords(months)), nil
}

func runwayWords(m float64) string {
	half := math.Round(m*2) / 2
	switch {
	case m < 1:
		return "menos de un mes"
	case m < 1.5:
		return "un mes"
	case m < 2:
		return "casi dos meses"
	case half >= 12:
		return "más de un año"
	case half == math.Trunc(half):
		return fmt.Sprintf("más de %d meses", int(half))
	default:
		return fmt.Sprintf("%d y medio meses", int(half))
	}
}

func jumpWords(ratio float64, prevMonth string) string {
	switch {
	case ratio >= 3.5:
		return timesOver(int(math.Round(ratio))) + " lo de " + prevMonth
	case ratio >= 2.85:
		return "el triple de " + prevMonth
	case ratio >= 2.5:
		return "casi el triple de " + prevMonth
	case ratio >= 1.85:
		return "el doble de " + prevMonth
	case ratio >= 1.6:
		return "casi el doble de " + prevMonth
	case ratio >= 1.4:
		return "la mitad más que en " + prevMonth
	default:
		return "bastante más que en " + prevMonth
	}
}

// timesOver spells a whole multiple the way it gets said out loud, never as a
// digit: "siete veces", not "7 veces".
func timesOver(n int) string {
	names := [...]string{"cuatro", "cinco", "seis", "siete", "ocho", "nueve", "diez"}
	if n < 4 || n > 3+len(names) {
		return "más de diez veces"
	}
	return names[n-4] + " veces"
}

func shareWords(frac float64) string {
	switch {
	case frac >= 0.66:
		return "dos tercios"
	case frac >= 0.45:
		return "la mitad"
	case frac >= 0.28:
		return "un tercio"
	case frac >= 0.20:
		return "un cuarto"
	default:
		return ""
	}
}

func (b *Builder) monthlyCategories(userID uint64, from, to time.Time) ([]movement.CategorySum, error) {
	return b.movements.SumForUser(monthQuery(userID, from, to, constants.Expense), movement.GroupByCategory)
}

func monthlyJumpLine(cats, prevCats []movement.CategorySum, prevTo time.Time) string {
	if len(prevCats) == 0 {
		return ""
	}
	prev := make(map[string]decimal.Decimal, len(prevCats))
	for _, r := range prevCats {
		prev[r.Label] = r.Total
	}

	jumped, best := -1, decimal.Zero
	for i, c := range cats {
		p, seen := prev[c.Label]
		delta := c.Total.Sub(p)
		if !delta.IsPositive() {
			continue
		}
		if seen && !p.IsZero() && delta.Div(p).LessThan(decimal.NewFromFloat(jumpThreshold)) {
			continue
		}
		if delta.GreaterThan(best) {
			jumped, best = i, delta
		}
	}
	if jumped < 0 {
		return ""
	}

	c := cats[jumped]
	name := html.EscapeString(c.Label)
	amount := money(c.Total, currency.ARS)
	prevMonth := constants.MonthLongEs[prevTo.Month()-1]
	p, seen := prev[c.Label]
	if !seen || p.IsZero() {
		return fmt.Sprintf(msgMonthlyJumpNew, name, amount, prevMonth)
	}
	ratio, _ := c.Total.Div(p).Float64()
	return fmt.Sprintf(msgMonthlyJump, name, amount, jumpWords(ratio, prevMonth))
}

func (b *Builder) monthlyFold(userID uint64, cats []movement.CategorySum, spent decimal.Decimal, from, to time.Time) (string, error) {
	if len(cats) < 2 {
		return "", nil
	}
	rows := cats
	if len(rows) > monthlyRankingRows {
		rows = rows[:monthlyRankingRows]
	}

	var sb strings.Builder
	sb.WriteString(msgMonthlyFoldOpen)
	for i, c := range rows {
		icon := b.icons.IconForCategory(userID, c.Label)
		name := icon + " " + html.EscapeString(c.Label)
		share := ""
		if i == 0 && spent.IsPositive() {
			frac, _ := c.Total.Div(spent).Float64()
			share = shareWords(frac)
		}
		if share != "" {
			fmt.Fprintf(&sb, msgMonthlyFoldShare, name, money(c.Total, currency.ARS), share)
			continue
		}
		fmt.Fprintf(&sb, msgMonthlyFoldRow, name, money(c.Total, currency.ARS))
	}

	top, err := b.movements.TopExpenseForUser(movement.MovementQuery{
		UserID: userID, From: from, To: to, Currency: currency.ARS,
	})
	if err != nil {
		return "", err
	}
	if top != nil {
		desc := "sin detalle"
		if top.Description != nil && *top.Description != "" {
			desc = html.EscapeString(*top.Description)
		}
		fmt.Fprintf(&sb, msgMonthlyFoldTop, money(top.Amount.Abs(), currency.ARS), desc)
		sb.WriteString("\n")
	}
	sb.WriteString(msgMonthlyFoldClose)
	return sb.String(), nil
}
