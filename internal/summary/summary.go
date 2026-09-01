package summary

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/quote"
)

type MovementReader interface {
	SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error)
	TopExpenseForUser(q movement.MovementQuery) (*movement.Movement, error)
	CountByDayForUser(userID uint64, from, to time.Time) ([]movement.DayCount, error)
	SumAmountForAccount(accountID uint64) (decimal.Decimal, error)
	MonthlyDeltasForAccount(accountID uint64) ([]movement.MonthlyDelta, error)
}

type AccountReader interface {
	FindByUserID(userID uint64) ([]account.Account, error)
}

type IconReader interface {
	IconForCategory(userID uint64, category string) string
}

type QuoteReader interface {
	FindRateOnOrBefore(date time.Time, rateType string) (*quote.Quote, error)
}

type Builder struct {
	movements MovementReader
	accounts  AccountReader
	icons     IconReader
	quotes    QuoteReader
}

func NewBuilder(m MovementReader, a AccountReader, i IconReader, q QuoteReader) *Builder {
	return &Builder{movements: m, accounts: a, icons: i, quotes: q}
}

const (
	topCategories = 3

	jumpThreshold = 0.30

	sameSpendThreshold = 0.05

	projectionMinDay = 10
)

func (b *Builder) Build(userID uint64, from, to, prevFrom, prevTo time.Time) (string, error) {
	counts, err := b.movements.CountByDayForUser(userID, from, to)
	if err != nil {
		return "", err
	}
	if len(counts) == 0 {
		return msgEmptyWeek, nil
	}

	var body strings.Builder
	mood := ""
	for _, cur := range currency.SupportedCurrencies {
		block, m, err := b.currencyBlock(userID, cur, from, to, prevFrom, prevTo)
		if err != nil {
			return "", err
		}
		if mood == "" {
			mood = m
		}
		body.WriteString(block)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📊 <b>Tu semana</b> · %s\n", weekRange(from, to))
	if mood != "" {
		fmt.Fprintf(&sb, "<i>%s</i>\n", mood)
	}
	sb.WriteString(body.String())

	accts, err := b.accountsBlock(userID)
	if err != nil {
		return "", err
	}
	sb.WriteString(accts)
	if accts != "" {
		sb.WriteString(msgBalanceNote)
	}

	sb.WriteString(msgNote)
	return sb.String(), nil
}

func (b *Builder) currencyBlock(userID uint64, cur currency.Currency, from, to, prevFrom, prevTo time.Time) (string, string, error) {
	expense := constants.Expense
	income := constants.Income

	spent, err := b.total(movement.MovementQuery{UserID: userID, From: from, To: to, Currency: cur, Type: &expense})
	if err != nil {
		return "", "", err
	}
	earned, err := b.total(movement.MovementQuery{UserID: userID, From: from, To: to, Currency: cur, Type: &income})
	if err != nil {
		return "", "", err
	}
	variation, err := b.variation(userID, cur, from, to)
	if err != nil {
		return "", "", err
	}
	if spent.IsZero() && earned.IsZero() && variation.IsZero() {
		return "", "", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "\n%s <b>%s</b>\n", curIcon(cur), curLabel(cur))

	switch {
	case !spent.IsZero() && !earned.IsZero():
		fmt.Fprintf(&sb, "Gastaste <b>%s</b> · entró %s\n", money(spent, cur), money(earned, cur))
	case !spent.IsZero():
		fmt.Fprintf(&sb, "Gastaste <b>%s</b>\n", money(spent, cur))
	case !earned.IsZero():
		fmt.Fprintf(&sb, "Entró <b>%s</b> y no gastaste nada\n", money(earned, cur))
	}

	prev := decimal.Zero
	if !spent.IsZero() {
		prev, err = b.total(movement.MovementQuery{UserID: userID, From: prevFrom, To: prevTo, Currency: cur, Type: &expense})
		if err != nil {
			return "", "", err
		}
		sb.WriteString(comparisonLine(spent, prev, cur))
	}

	cats, err := b.categoryLines(userID, cur, spent, from, to, prevFrom, prevTo)
	if err != nil {
		return "", "", err
	}
	sb.WriteString(cats)

	if top, err := b.movements.TopExpenseForUser(movement.MovementQuery{UserID: userID, From: from, To: to, Currency: cur}); err != nil {
		return "", "", err
	} else if top != nil {
		desc := "sin detalle"
		if top.Description != nil && *top.Description != "" {
			desc = html.EscapeString(*top.Description)
		}
		fmt.Fprintf(&sb, "Lo más caro: <b>%s</b> · %s\n", money(top.Amount.Abs(), cur), desc)
	}

	if !variation.IsZero() {
		verb := "subieron"
		if variation.IsNegative() {
			verb = "bajaron"
		}
		fmt.Fprintf(&sb, "Tus saldos %s <b>%s</b> <i>por rendimientos y ajustes</i>\n", verb, money(variation.Abs(), cur))
	}

	proj, err := b.projectionLine(userID, cur, to)
	if err != nil {
		return "", "", err
	}
	sb.WriteString(proj)

	return sb.String(), moodLine(spent, prev, earned), nil
}

func moodLine(spent, prev, earned decimal.Decimal) string {
	switch {
	case !prev.IsZero() && spent.GreaterThan(prev.Mul(decimal.NewFromFloat(1.3))):
		return "Semana cara"
	case !prev.IsZero() && spent.LessThan(prev.Mul(decimal.NewFromFloat(0.7))):
		return "Semana tranquila"
	case !spent.IsZero() && earned.GreaterThan(spent.Mul(decimal.NewFromInt(2))):
		return "Entró bastante más de lo que salió"
	default:
		return ""
	}
}

func comparisonLine(spent, prev decimal.Decimal, cur currency.Currency) string {
	if prev.IsZero() {
		return ""
	}
	delta := spent.Sub(prev)
	if delta.Abs().LessThan(prev.Mul(decimal.NewFromFloat(sameSpendThreshold))) {
		return "<i>Parecido a la semana pasada</i>\n"
	}
	arrow, word := "↗", "más"
	if delta.IsNegative() {
		arrow, word = "↘", "menos"
	}
	return fmt.Sprintf("<i>%s %s %s que la semana pasada</i>\n", arrow, money(roundNice(delta.Abs(), cur), cur), word)
}

func (b *Builder) categoryLines(userID uint64, cur currency.Currency, spent decimal.Decimal, from, to, prevFrom, prevTo time.Time) (string, error) {
	expense := constants.Expense
	cats, err := b.movements.SumForUser(movement.MovementQuery{
		UserID: userID, From: from, To: to, Currency: cur, Type: &expense,
	}, movement.GroupByCategory)
	if err != nil {
		return "", err
	}
	if len(cats) == 0 || (len(cats) == 1 && cats[0].Total.Equal(spent)) {
		return "", nil
	}
	if len(cats) > topCategories {
		cats = cats[:topCategories]
	}

	prevRows, err := b.movements.SumForUser(movement.MovementQuery{
		UserID: userID, From: prevFrom, To: prevTo, Currency: cur, Type: &expense,
	}, movement.GroupByCategory)
	if err != nil {
		return "", err
	}
	comparable := len(prevRows) > 0
	prev := make(map[string]decimal.Decimal, len(prevRows))
	for _, r := range prevRows {
		prev[r.Label] = r.Total
	}

	jumped, best := -1, decimal.Zero
	for i, c := range cats {
		if !comparable {
			break
		}
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

	var sb strings.Builder
	for i, c := range cats {
		fmt.Fprintf(&sb, "  %s %s %s", b.icons.IconForCategory(userID, c.Label), html.EscapeString(c.Label), money(c.Total, cur))
		if i == jumped {
			if p, seen := prev[c.Label]; !seen || p.IsZero() {
				sb.WriteString(" · <i>no gastaste acá la semana pasada</i>")
			} else {
				fmt.Fprintf(&sb, " · <i>%s más que la semana pasada</i>", money(roundNice(best, cur), cur))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func (b *Builder) projectionLine(userID uint64, cur currency.Currency, to time.Time) (string, error) {
	elapsed := to.Day()
	if elapsed < projectionMinDay {
		return "", nil
	}
	first := time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, to.Location())
	expense := constants.Expense
	rows, err := b.movements.SumForUser(movement.MovementQuery{
		UserID: userID, From: first, To: to, Currency: cur, Type: &expense,
	}, movement.GroupByDay)
	if err != nil {
		return "", err
	}

	sample := make([]decimal.Decimal, elapsed)
	for i := range sample {
		sample[i] = decimal.Zero
	}
	spent := decimal.Zero
	for _, r := range rows {
		d, err := time.Parse("2006-01-02", r.Label)
		if err != nil || d.Day() > elapsed {
			continue
		}
		sample[d.Day()-1] = r.Total
		spent = spent.Add(r.Total)
	}
	sort.Slice(sample, func(i, j int) bool { return sample[i].LessThan(sample[j]) })

	p75 := percentile(sample, 75)
	if p75.IsZero() {
		return "", nil
	}
	left := decimal.NewFromInt(int64(daysInMonth(to) - elapsed))
	lo := spent.Add(percentile(sample, 25).Mul(left))
	hi := spent.Add(p75.Mul(left))

	m := constants.MonthLongEs[to.Month()-1]
	return fmt.Sprintf("%s%s va camino a cerrar entre <b>%s</b>\n",
		strings.ToUpper(m[:1]), m[1:], band(lo, hi, cur)), nil
}

func percentile(sorted []decimal.Decimal, p int) decimal.Decimal {
	if len(sorted) == 0 {
		return decimal.Zero
	}
	i := len(sorted) * p / 100
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return sorted[i]
}

var (
	oneMillion     = decimal.NewFromInt(1_000_000)
	stepMillions   = decimal.NewFromInt(100_000)
	stepPesos      = decimal.NewFromInt(10_000)
	stepDollars    = decimal.NewFromInt(10)
	stepThousand   = decimal.NewFromInt(1_000)
	roundNiceFloor = decimal.NewFromInt(10_000)
)

func band(lo, hi decimal.Decimal, cur currency.Currency) string {
	if cur != currency.ARS {
		return fmt.Sprintf("%s y %s",
			plainDollars(floorTo(lo, stepDollars)), plainDollars(ceilTo(hi, stepDollars)))
	}
	if lo.GreaterThanOrEqual(oneMillion) {
		l := floorTo(lo, stepMillions).Div(oneMillion)
		h := ceilTo(hi, stepMillions).Div(oneMillion)
		return fmt.Sprintf("$%s y $%s millones", comma(l), comma(h))
	}
	return fmt.Sprintf("%s y %s",
		currency.FormatMoney(floorTo(lo, stepPesos), cur),
		currency.FormatMoney(ceilTo(hi, stepPesos), cur))
}

func roundNice(d decimal.Decimal, cur currency.Currency) decimal.Decimal {
	if cur != currency.ARS || d.LessThan(roundNiceFloor) {
		return d
	}
	return d.Div(stepThousand).Round(0).Mul(stepThousand)
}

func floorTo(d, step decimal.Decimal) decimal.Decimal { return d.Div(step).Floor().Mul(step) }
func ceilTo(d, step decimal.Decimal) decimal.Decimal  { return d.Div(step).Ceil().Mul(step) }

func comma(d decimal.Decimal) string { return strings.Replace(d.StringFixed(1), ".", ",", 1) }

func plainDollars(d decimal.Decimal) string {
	return "US$" + currency.GroupThousands(d.StringFixed(0))
}

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

func (b *Builder) accountsBlock(userID uint64) (string, error) {
	accts, err := b.accounts.FindByUserID(userID)
	if err != nil {
		return "", err
	}
	if len(accts) == 0 {
		return "", nil
	}
	var lines, foreign []string
	for _, a := range accts {
		bal, err := b.movements.SumAmountForAccount(uint64(a.ID))
		if err != nil {
			return "", err
		}
		name := html.EscapeString(a.Name)
		if a.Currency == currency.ARS {
			lines = append(lines, fmt.Sprintf("  %s <b>%s</b>", name, money(bal, a.Currency)))
			continue
		}
		foreign = append(foreign, fmt.Sprintf("%s %s", name, money(bal, a.Currency)))
	}
	if len(lines) == 0 {
		for _, a := range accts {
			bal, _ := b.movements.SumAmountForAccount(uint64(a.ID))
			lines = append(lines, fmt.Sprintf("  %s <b>%s</b>", html.EscapeString(a.Name), money(bal, a.Currency)))
		}
		foreign = nil
	}
	if len(foreign) > 0 {
		lines = append(lines, "  En dólares: "+strings.Join(foreign, " · "))
	}
	return "\n🏦 <b>Hoy tenés</b>\n" + strings.Join(lines, "\n") + "\n", nil
}

func money(d decimal.Decimal, cur currency.Currency) string {
	return currency.FormatMoney(d, cur)
}

func weekRange(from, to time.Time) string {
	if from.Month() == to.Month() {
		return fmt.Sprintf("%d al %d de %s", from.Day(), to.Day(), constants.MonthLongEs[to.Month()-1])
	}
	return fmt.Sprintf("%d de %s al %d de %s",
		from.Day(), constants.MonthLongEs[from.Month()-1],
		to.Day(), constants.MonthLongEs[to.Month()-1])
}

func daysInMonth(t time.Time) int {
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
}

func curIcon(cur currency.Currency) string {
	if cur == currency.ARS {
		return "💸"
	}
	return "💵"
}

func curLabel(cur currency.Currency) string {
	if cur == currency.ARS {
		return "Pesos"
	}
	return "Dólares"
}
