package summary

import (
	"fmt"
	"html"
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
	_ = closingARS

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
