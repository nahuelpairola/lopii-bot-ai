package summary

import (
	"fmt"
	"html"
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
	// SumAmountForAccount computes an account's current balance (SUM over its
	// movements) — the balance is never stored (movement.repository owns it).
	SumAmountForAccount(accountID uint64) (decimal.Decimal, error)
}

// AccountReader is the account-repo surface for the balances snapshot (listing
// only; per-account balances come from MovementReader).
type AccountReader interface {
	FindByUserID(userID uint64) ([]account.Account, error)
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
	fmt.Fprintf(&sb, "📊 <b>Resumen semanal</b> · %s a %s\n", fmtDay(from), fmtDay(to))

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
	variation, err := b.variation(userID, cur, from, to)
	if err != nil {
		return "", err
	}
	// Una semana en la que lo único que pasó fue un ajuste de saldo igual tiene
	// algo que contar: el saldo se movió, aunque no haya habido ni gasto ni
	// ingreso.
	if spent.IsZero() && earned.IsZero() && variation.IsZero() {
		return "", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "\n%s <b>%s</b>\n", curIcon(cur), cur.String())

	// Una semana cuyo único evento fue un ajuste llega hasta acá (el ajuste ES
	// un movimiento, así que no la agarra el nudge de semana vacía). Recitarle
	// "Entró $0 · Salió $0 · Neto $0 · Prom. diario $0" antes de contar lo
	// único que pasó son cuatro números en cero pidiendo atención.
	hasCashflow := !spent.IsZero() || !earned.IsZero()
	if hasCashflow {
		fmt.Fprintf(&sb, "Entró $%s · Salió $%s · Neto <b>$%s</b>\n",
			earned.StringFixed(2), spent.StringFixed(2), earned.Sub(spent).StringFixed(2))
	}
	if !variation.IsZero() {
		sign := "+"
		if variation.IsNegative() {
			sign = "-"
		}
		fmt.Fprintf(&sb, "Variación de saldos <b>%s$%s</b> · ajustes y rendimiento, no cuenta como gasto ni ingreso\n",
			sign, variation.Abs().StringFixed(2))
	}

	if hasCashflow {
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
			line += fmt.Sprintf(" · vs semana previa <b>%s%s%%</b>", arrow, pct.Abs().StringFixed(0))
		}
		sb.WriteString(line + "\n")
	}

	cats, err := b.movements.SumForUser(movement.MovementQuery{UserID: userID, From: from, To: to, Currency: cur, Type: &expense}, movement.GroupByCategory)
	if err != nil {
		return "", err
	}
	if len(cats) > 0 {
		sb.WriteString("Top gastos:\n")
		for i, c := range cats {
			if i == topCategories {
				break
			}
			fmt.Fprintf(&sb, " %s $%s\n", html.EscapeString(c.Label), c.Total.StringFixed(2))
		}
	}

	if top, err := b.movements.TopExpenseForUser(movement.MovementQuery{UserID: userID, From: from, To: to, Currency: cur}); err != nil {
		return "", err
	} else if top != nil {
		desc := "sin detalle"
		if top.Merchant != nil && *top.Merchant != "" {
			desc = html.EscapeString(*top.Merchant)
		}
		fmt.Fprintf(&sb, "Lo más caro: <b>$%s</b> · %s\n", top.Amount.Abs().StringFixed(2), desc)
	}

	return sb.String(), nil
}

// variation is what the balances did with no real transaction behind it —
// balance adjustments and investment yield — as one signed figure. Kept out of
// Entró/Salió/Neto on purpose: a CEDEAR revaluation is not money earned, and
// counting it as income makes the weekly comparison lie. SumForUser returns
// SUM(ABS(amount)), so the direction comes back from the type label; leaving
// Type nil drops the transfers, and with them opening balances and transfer
// legs.
func (b *Builder) variation(userID uint64, cur currency.Currency, from, to time.Time) (decimal.Decimal, error) {
	rows, err := b.movements.SumForUser(movement.MovementQuery{
		UserID: userID, From: from, To: to, Currency: cur, OnlyReserved: true,
	}, movement.GroupByType)
	if err != nil {
		return decimal.Zero, err
	}
	v := decimal.Zero
	for _, r := range rows {
		if r.Label == constants.Income {
			v = v.Add(r.Total)
			continue
		}
		v = v.Sub(r.Total)
	}
	return v, nil
}

// total runs SumForUser with no grouping and returns the single total (0 if none).
func (b *Builder) total(q movement.MovementQuery) (decimal.Decimal, error) {
	rows, err := b.movements.SumForUser(q, movement.GroupByNone)
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
	return fmt.Sprintf("\n📈 <b>Actividad</b>\n%d registros · día top: <b>%s</b>\n", totalN, weekdayEs[best.Date.Weekday()])
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
	sb.WriteString("\n🏦 <b>Cuentas (hoy)</b>\n")
	for _, a := range accts {
		bal, err := b.movements.SumAmountForAccount(uint64(a.ID))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&sb, " %s <b>$%s</b> (%s)\n", html.EscapeString(a.Name), bal.StringFixed(2), a.Currency.String())
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
