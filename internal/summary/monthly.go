package summary

import (
	"fmt"
	"html"
	"math"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/constants"
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
	fmt.Fprintf(&sb, msgMonthlyHeader, monthTitle(to))
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

	return conversation.Prompt{Text: sb.String()}, nil
}

func monthQuery(userID uint64, from, to time.Time, typ string) movement.MovementQuery {
	t := typ
	return movement.MovementQuery{UserID: userID, From: from, To: to, Currency: currency.ARS, Type: &t}
}

func openingLines(spent, earned decimal.Decimal) string {
	var sb strings.Builder
	if earned.IsZero() {
		fmt.Fprintf(&sb, msgMonthlyOnlySpent, money(spent, currency.ARS))
		return sb.String()
	}
	fmt.Fprintf(&sb, msgMonthlyInAndOut, money(earned, currency.ARS), money(spent, currency.ARS))
	left := earned.Sub(spent)
	if left.IsNegative() {
		fmt.Fprintf(&sb, msgMonthlyOverspent, money(left.Abs(), currency.ARS))
		return sb.String()
	}
	fmt.Fprintf(&sb, msgMonthlyLeftover, money(left, currency.ARS))
	return sb.String()
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

func monthTitle(t time.Time) string {
	name := constants.MonthLongEs[t.Month()-1]
	return strings.ToUpper(name[:1]) + name[1:]
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
	prevMonth := constants.MonthLongEs[to.AddDate(0, -1, 0).Month()-1]
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
		if delta.IsPositive() {
			lines = append(lines, fmt.Sprintf(msgMonthlyAccountUp,
				name, money(closing, cur), money(delta, cur), prevMonth))
			continue
		}
		lines = append(lines, fmt.Sprintf(msgMonthlyAccountDown,
			name, money(closing, cur), money(delta.Abs(), cur)))
	}
	if len(lines) == 0 {
		return "", totalClosing, nil
	}
	return fmt.Sprintf(msgMonthlyAccountsHeader, lastDayOfMonth(to)) + strings.Join(lines, ""), totalClosing, nil
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
		money(usd, currency.USD), q.Date.Day(), constants.MonthLongEs[q.Date.Month()-1]), nil
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
