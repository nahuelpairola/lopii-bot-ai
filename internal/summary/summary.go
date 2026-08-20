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

// IconReader is the subcategory-cache surface the summary needs: the icon a
// category shows everywhere else in the bot. Never returns empty — the cache
// falls back to 📂.
type IconReader interface {
	IconForCategory(userID uint64, category string) string
}

type Builder struct {
	movements MovementReader
	accounts  AccountReader
	icons     IconReader
}

func NewBuilder(m MovementReader, a AccountReader, i IconReader) *Builder {
	return &Builder{movements: m, accounts: a, icons: i}
}

const (
	topCategories = 3

	// jumpThreshold es cuánto tiene que haber subido una categoría, sobre sí
	// misma, para que valga la pena señalarla. Debajo de esto es ruido semanal
	// y anotarlo entrena al usuario a ignorar la anotación.
	jumpThreshold = 0.30

	// sameSpendThreshold: dos semanas que difieren menos que esto gastaron lo
	// mismo. Decir "$40 más que la semana pasada" sobre $400.000 es precisión
	// sin información.
	sameSpendThreshold = 0.05

	// projectionMinDay es el día del mes desde el cual la muestra diaria da
	// para proyectar. Antes de eso son tres o cuatro días y el rango sale
	// absurdo — un lunes 5 no sabe nada de cómo termina el mes.
	projectionMinDay = 10
)

// Build assembles the weekly-summary text for a user. from..to is the reported
// week (Mon–Sun); prevFrom..prevTo the week before (comparisons). Returns the
// empty-week nudge when the user logged nothing in the window. The disable
// button is attached by the caller (sweeper) — this returns text only.
func (b *Builder) Build(userID uint64, from, to, prevFrom, prevTo time.Time) (string, error) {
	counts, err := b.movements.CountByDayForUser(userID, from, to)
	if err != nil {
		return "", err
	}
	if len(counts) == 0 {
		return msgEmptyWeek, nil
	}

	// Los bloques se arman antes que la cabecera: la línea de apertura sale de
	// los números de la primera moneda que tuvo actividad.
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

	sb.WriteString(daysBlock(counts, from, to))
	sb.WriteString(msgNote)
	return sb.String(), nil
}

// currencyBlock renders one currency's story, plus the mood line the header
// shows. Returns "" for both if nothing happened in this currency this week.
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
	// Una semana en la que lo único que pasó fue un ajuste de saldo igual tiene
	// algo que contar: el saldo se movió, aunque no haya habido ni gasto ni
	// ingreso.
	if spent.IsZero() && earned.IsZero() && variation.IsZero() {
		return "", "", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "\n%s <b>%s</b>\n", curIcon(cur), curLabel(cur))

	// Una semana cuyo único evento fue un ajuste llega hasta acá (el ajuste ES
	// un movimiento, así que no la agarra el nudge de semana vacía). Recitarle
	// "Gastaste $0" antes de contar lo único que pasó es un cero pidiendo
	// atención.
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
		// Desde el fold de merchant esto sale de description, que es un campo
		// requerido del CREATE: "sin detalle" pasó de ser lo habitual a ser el
		// último recurso.
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
		// "por rendimientos y ajustes" y no "(rendimientos)": un ajuste de
		// saldo a mano no es un rendimiento, y llamarlo así sería mentira la
		// mitad de las veces. Va en cursiva: es la aclaración, no el número.
		fmt.Fprintf(&sb, "Tus saldos %s <b>%s</b> <i>por rendimientos y ajustes</i>\n", verb, money(variation.Abs(), cur))
	}

	proj, err := b.projectionLine(userID, cur, to)
	if err != nil {
		return "", "", err
	}
	sb.WriteString(proj)

	return sb.String(), moodLine(spent, prev, earned), nil
}

// moodLine is the one sentence under the header. Sale de los números y con
// prioridad fija — nada de azar: una frase que rota sola cada lunes deja de
// leerse como una observación y pasa a leerse como decoración. Sin motivo
// claro devuelve "" y el mensaje arranca derecho con los números.
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

// comparisonLine says what changed against last week in money, never in
// percent: "$268.000 menos" es una plata que se siente, "↓37%" es un indicador.
func comparisonLine(spent, prev decimal.Decimal, cur currency.Currency) string {
	if prev.IsZero() {
		return ""
	}
	delta := spent.Sub(prev)
	if delta.Abs().LessThan(prev.Mul(decimal.NewFromFloat(sameSpendThreshold))) {
		return "<i>Parecido a la semana pasada</i>\n"
	}
	// Redondeada por el mismo motivo que el rango: la comparación es una
	// magnitud, no una liquidación. "$267.999 menos" finge una precisión
	// que el número no tiene.
	arrow, word := "↗", "más"
	if delta.IsNegative() {
		arrow, word = "↘", "menos"
	}
	return fmt.Sprintf("<i>%s %s %s que la semana pasada</i>\n", arrow, money(roundNice(delta.Abs(), cur), cur), word)
}

// categoryLines renders the top expense categories and annotates the ONE that
// jumped hardest against last week. Una sola: la anotación vale porque es
// excepcional, y tres anotaciones seguidas no señalan nada.
func (b *Builder) categoryLines(userID uint64, cur currency.Currency, spent decimal.Decimal, from, to, prevFrom, prevTo time.Time) (string, error) {
	expense := constants.Expense
	cats, err := b.movements.SumForUser(movement.MovementQuery{
		UserID: userID, From: from, To: to, Currency: cur, Type: &expense,
	}, movement.GroupByCategory)
	if err != nil {
		return "", err
	}
	// Una sola categoría que ES todo el gasto dice el mismo número dos veces.
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
	// Sin filas previas no hay con qué comparar: anunciarle a un usuario que
	// recién arranca que "no gastaste acá la semana pasada" es ruido, no dato.
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
		// Una categoría nueva siempre es noticia; una que ya existía tiene que
		// haber subido lo suyo sobre sí misma.
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

// projectionLine estimates where the month lands, as a RANGE built from the
// user's own daily spread — p25 and p75 of the days elapsed, stretched over the
// days left. Sin gaussiana y sin intervalo de confianza: es literalmente "tus
// días baratos y tus días caros". El rango se angosta solo a medida que avanza
// el mes, que es exactamente lo que tiene que pasar.
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

	// Los días sin gasto son parte de la muestra. Si la muestra fueran sólo las
	// filas que volvieron, el p25 saldría sesgado para arriba y la proyección
	// inflaría: un día en que no gastaste nada es un día barato, no un dato que
	// falta.
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

// percentile picks the p-th value of an ascending sample by position. Sin
// interpolar: con diez o veinte días la interpolación agrega decimales, no
// información, y el resultado se redondea igual.
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
	oneMillion   = decimal.NewFromInt(1_000_000)
	stepMillions = decimal.NewFromInt(100_000)
	stepPesos    = decimal.NewFromInt(10_000)
	stepDollars  = decimal.NewFromInt(10)
	stepThousand = decimal.NewFromInt(1_000)
	// roundNiceFloor: debajo de esto redondear borraría el dato entero.
	roundNiceFloor = decimal.NewFromInt(10_000)
)

// band renders the projected range, rounded outward — el piso hacia abajo y el
// techo hacia arriba. Redondear es parte del mensaje: un rango con centavos se
// contradice a sí mismo, porque dice "no sé exactamente" con seis dígitos de
// precisión.
func band(lo, hi decimal.Decimal, cur currency.Currency) string {
	if cur != currency.ARS {
		return fmt.Sprintf("%s y %s",
			plainDollars(floorTo(lo, stepDollars)), plainDollars(ceilTo(hi, stepDollars)))
	}
	// Sólo cuando el PISO llega al millón: "$0,4 millones" es peor que
	// "$400.000" — la escala está para acortar, no para achicar.
	if lo.GreaterThanOrEqual(oneMillion) {
		l := floorTo(lo, stepMillions).Div(oneMillion)
		h := ceilTo(hi, stepMillions).Div(oneMillion)
		return fmt.Sprintf("$%s y $%s millones", comma(l), comma(h))
	}
	return fmt.Sprintf("%s y %s",
		currency.FormatMoney(floorTo(lo, stepPesos), cur),
		currency.FormatMoney(ceilTo(hi, stepPesos), cur))
}

// roundNice drops the digits a magnitude does not carry. Debajo de diez mil
// no toca nada: ahí cada peso todavía se distingue.
func roundNice(d decimal.Decimal, cur currency.Currency) decimal.Decimal {
	if cur != currency.ARS || d.LessThan(roundNiceFloor) {
		return d
	}
	return d.Div(stepThousand).Round(0).Mul(stepThousand)
}

func floorTo(d, step decimal.Decimal) decimal.Decimal { return d.Div(step).Floor().Mul(step) }
func ceilTo(d, step decimal.Decimal) decimal.Decimal  { return d.Div(step).Ceil().Mul(step) }

// comma renders one decimal place the way an Argentine reads it ("1,6").
func comma(d decimal.Decimal) string { return strings.Replace(d.StringFixed(1), ".", ",", 1) }

// plainDollars drops the cents: a projected range is not a balance.
func plainDollars(d decimal.Decimal) string {
	return "US$" + currency.GroupThousands(d.StringFixed(0))
}

// variation is what the balances did with no real transaction behind it —
// balance adjustments and investment yield — as one signed figure. Kept out of
// what you spent and what came in on purpose: a CEDEAR revaluation is not money
// earned, and counting it as income makes the weekly comparison lie.
// SumForUser returns SUM(ABS(amount)), so the direction comes back from the
// type label; leaving Type nil drops the transfers, and with them opening
// balances and transfer legs.
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

// daysBlock says how much of the week the numbers above actually cover. Sin
// esto el resumen afirma "gastaste $X" como si fuera la semana entera, cuando
// bien puede ser el 70% de ella.
func daysBlock(counts []movement.DayCount, from, to time.Time) string {
	seen := make(map[string]bool, len(counts))
	for _, c := range counts {
		seen[c.Date.Format("2006-01-02")] = true
	}
	total, logged := 0, 0
	var missing []string
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		total++
		if seen[d.Format("2006-01-02")] {
			logged++
			continue
		}
		missing = append(missing, movement.WeekdayEs(d))
	}
	if len(missing) == 0 {
		return fmt.Sprintf("\n<i>Anotaste los %d días 💪</i>\n", total)
	}
	// Nombrar los días sólo mientras sea una frase. Con tres o más es una lista
	// y ya no se lee, se saltea.
	if len(missing) <= 2 {
		return fmt.Sprintf("\n<i>Anotaste %d de %d días (te faltaron %s)</i>\n",
			logged, total, strings.Join(missing, " y "))
	}
	// Con media semana sin anotar el cierre dice de dónde salen los números:
	// misma honestidad que el conteo, dicha en voz alta.
	return fmt.Sprintf("\n<i>Anotaste %d de %d días · lo de arriba sale de ahí nomás 👀</i>\n", logged, total)
}

// accountsBlock lists the ARS balances one per line and folds the rest into a
// single tail line. Un usuario tiene dos o tres cuentas en pesos y las mismas
// repetidas en dólares: listadas planas, "Mercado Pago" aparece dos veces
// seguidas sin decir por qué.
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
	// Sin cuentas en pesos, las otras no son "las otras": se listan derecho.
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

// money is the one money renderer in this package: AR thousands, no cents on
// pesos. Antes cada renglón hacía StringFixed(2) por su cuenta, y "$453774.78"
// es lo que hacía que el resumen leyera a planilla.
func money(d decimal.Decimal, cur currency.Currency) string {
	return currency.FormatMoney(d, cur)
}

// weekRange renders the reported week the way it gets said out loud: "6 al 12
// de julio", y sólo repite el mes cuando la semana lo cruza.
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
