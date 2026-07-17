package summary

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

// MovementReader is the movement-repo surface the builder needs (consumer-local
// interface, per repo convention).
type MovementReader interface {
	SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error)
	TopExpenseForUser(q movement.MovementQuery) (*movement.Movement, error)
	CountByDayForUser(userID uint64, from, to time.Time) ([]movement.DayCount, error)
}

// AccountReader is the account-repo surface for the balances snapshot.
type AccountReader interface {
	FindByUserID(userID uint64) ([]account.Account, error)
	SumAmountForAccount(accountID uint64) (decimal.Decimal, error)
}

type Builder struct {
	movements MovementReader
	accounts  AccountReader
}

func NewBuilder(m MovementReader, a AccountReader) *Builder {
	return &Builder{movements: m, accounts: a}
}

const topCategories = 3

// Build assembles the weekly-summary text for a user. from..to is the reported
// week (Mon–Sun); prevFrom..prevTo the week before (for the delta). Returns the
// empty-week nudge when the user logged nothing in the window. The disable
// button is attached by the caller (sweeper) — this returns text only.
func (b *Builder) Build(userID uint64, from, to, prevFrom, prevTo time.Time) (string, error) {
	counts, err := b.movements.CountByDayForUser(userID, from, to)
	if err != nil {
		return "", err
	}
	totalN := 0
	for _, c := range counts {
		totalN += c.Count
	}
	if totalN == 0 {
		return msgEmptyWeek, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📊 Resumen semanal · %s a %s\n", fmtDay(from), fmtDay(to))

	for _, cur := range currency.SupportedCurrencies {
		block, err := b.currencyBlock(userID, cur, from, to, prevFrom, prevTo)
		if err != nil {
			return "", err
		}
		sb.WriteString(block)
	}

	sb.WriteString(activityBlock(counts, totalN))

	accts, err := b.accountsBlock(userID)
	if err != nil {
		return "", err
	}
	sb.WriteString(accts)

	sb.WriteString(msgNote)
	return sb.String(), nil
}

// currencyBlock renders one currency's money metrics, or "" if the user had no
// expense and no income in that currency this week.
func (b *Builder) currencyBlock(userID uint64, cur currency.Currency, from, to, prevFrom, prevTo time.Time) (string, error) {
	expense := constants.Expense
	income := constants.Income

	spent, err := b.total(movement.MovementQuery{UserID: userID, From: from, To: to, Currency: cur, Type: &expense})
	if err != nil {
		return "", err
	}
	earned, err := b.total(movement.MovementQuery{UserID: userID, From: from, To: to, Currency: cur, Type: &income})
	if err != nil {
		return "", err
	}
	if spent.IsZero() && earned.IsZero() {
		return "", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "\n%s %s\n", curIcon(cur), cur.String())
	fmt.Fprintf(&sb, "Entró $%s · Salió $%s · Neto $%s\n",
		earned.StringFixed(2), spent.StringFixed(2), earned.Sub(spent).StringFixed(2))

	daily := spent.Div(decimal.NewFromInt(7))
	line := fmt.Sprintf("Prom. diario $%s", daily.StringFixed(2))
	if prev, err := b.total(movement.MovementQuery{UserID: userID, From: prevFrom, To: prevTo, Currency: cur, Type: &expense}); err != nil {
		return "", err
	} else if !prev.IsZero() {
		pct := spent.Sub(prev).Div(prev).Mul(decimal.NewFromInt(100))
		arrow := "↑"
		if pct.IsNegative() {
			arrow = "↓"
		}
		line += fmt.Sprintf(" · vs semana previa %s%s%%", arrow, pct.Abs().StringFixed(0))
	}
	sb.WriteString(line + "\n")

	cats, err := b.movements.SumForUser(movement.MovementQuery{UserID: userID, From: from, To: to, Currency: cur, Type: &expense}, "category")
	if err != nil {
		return "", err
	}
	if len(cats) > 0 {
		sb.WriteString("Top gastos:\n")
		for i, c := range cats {
			if i == topCategories {
				break
			}
			fmt.Fprintf(&sb, " %s $%s\n", c.Label, c.Total.StringFixed(2))
		}
	}

	if top, err := b.movements.TopExpenseForUser(movement.MovementQuery{UserID: userID, From: from, To: to, Currency: cur}); err != nil {
		return "", err
	} else if top != nil {
		desc := "sin detalle"
		if top.Merchant != nil && *top.Merchant != "" {
			desc = *top.Merchant
		}
		fmt.Fprintf(&sb, "Lo más caro: $%s · %s\n", top.Amount.Abs().StringFixed(2), desc)
	}

	return sb.String(), nil
}

// total runs SumForUser with no grouping and returns the single total (0 if none).
func (b *Builder) total(q movement.MovementQuery) (decimal.Decimal, error) {
	rows, err := b.movements.SumForUser(q, "")
	if err != nil {
		return decimal.Zero, err
	}
	if len(rows) == 0 {
		return decimal.Zero, nil
	}
	return rows[0].Total, nil
}

func activityBlock(counts []movement.DayCount, totalN int) string {
	best := counts[0]
	for _, c := range counts[1:] {
		if c.Count > best.Count {
			best = c
		}
	}
	return fmt.Sprintf("\n📈 Actividad\n%d registros · día top: %s\n", totalN, weekdayEs[best.Date.Weekday()])
}

func (b *Builder) accountsBlock(userID uint64) (string, error) {
	accts, err := b.accounts.FindByUserID(userID)
	if err != nil {
		return "", err
	}
	if len(accts) == 0 {
		return "", nil
	}
	var sb strings.Builder
	sb.WriteString("\n🏦 Cuentas (hoy)\n")
	for _, a := range accts {
		bal, err := b.accounts.SumAmountForAccount(uint64(a.ID))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&sb, " %s $%s (%s)\n", a.Name, bal.StringFixed(2), a.Currency.String())
	}
	return sb.String(), nil
}

func fmtDay(t time.Time) string { return fmt.Sprintf("%d/%d", t.Day(), int(t.Month())) }

func curIcon(cur currency.Currency) string {
	if cur == currency.ARS {
		return "💸"
	}
	return "💵"
}
