package summary

import (
	"fmt"
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
