package summary

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/quote"
)

// fakeMovements returns canned aggregates. sums maps
// "CUR|type|groupBy|from(YYYY-MM-DD)" -> rows. La ventana entra en la clave
// porque el builder consulta TRES ventanas con la misma forma de query: la
// semana, la semana previa (comparación) y el mes hasta hoy (proyección). Sin
// la fecha en la clave las tres se pisan y un test verde no prueba nada.
type fakeMovements struct {
	sums   map[string][]movement.CategorySum
	tops   map[string]*movement.Movement
	counts []movement.DayCount
	bals   map[uint64]decimal.Decimal
	deltas map[uint64][]movement.MonthlyDelta
}

func (f fakeMovements) SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error) {
	typ := ""
	if q.Type != nil {
		typ = *q.Type
	}
	return f.sums[q.Currency.String()+"|"+typ+"|"+groupBy+"|"+q.From.Format("2006-01-02")], nil
}
func (f fakeMovements) TopExpenseForUser(q movement.MovementQuery) (*movement.Movement, error) {
	return f.tops[q.Currency.String()], nil
}
func (f fakeMovements) CountByDayForUser(_ uint64, _, _ time.Time) ([]movement.DayCount, error) {
	return f.counts, nil
}
func (f fakeMovements) SumAmountForAccount(id uint64) (decimal.Decimal, error) {
	return f.bals[id], nil
}

func (f fakeMovements) MonthlyDeltasForAccount(id uint64) ([]movement.MonthlyDelta, error) {
	return f.deltas[id], nil
}

// fakeIcons stands in for subcategory.Cache, que nunca devuelve vacío: su
// fallback es 📂.
type fakeIcons struct{ byCategory map[string]string }

func (f fakeIcons) IconForCategory(_ uint64, category string) string {
	if ic, ok := f.byCategory[category]; ok {
		return ic
	}
	return "📂"
}

type fakeQuotes struct{ rows map[string]*quote.Quote }

func (f fakeQuotes) FindRateOnOrBefore(_ time.Time, rateType string) (*quote.Quote, error) {
	return f.rows[rateType], nil
}

type fakeAccounts struct {
	list map[uint64][]account.Account
}

func (f fakeAccounts) FindByUserID(userID uint64) ([]account.Account, error) {
	return f.list[userID], nil
}

func dec(s string) decimal.Decimal { d, _ := decimal.NewFromString(s); return d }

func strptr(s string) *string { return &s }

func sums(rows ...movement.CategorySum) []movement.CategorySum { return rows }

func row(label, total string) movement.CategorySum {
	return movement.CategorySum{Label: label, Total: dec(total)}
}

var (
	from     = time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)  // Mon
	to       = time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC) // Sun
	prevFrom = from.AddDate(0, 0, -7)
	prevTo   = from.AddDate(0, 0, -1)
)

// Sufijos de clave: la semana reportada, la previa y el mes hasta hoy.
const (
	wk    = "|2026-07-06"
	prevW = "|2026-06-29"
	month = "|2026-07-01"
)

func TestBuild_EmptyWeek(t *testing.T) {
	b := NewBuilder(fakeMovements{counts: nil}, fakeAccounts{}, fakeIcons{}, fakeQuotes{})
	text, err := b.Build(1, from, to, prevFrom, prevTo)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(text, "no registraste movimientos") {
		t.Fatalf("expected empty-week nudge, got: %q", text)
	}
}

func TestBuild_ReportARSOnly(t *testing.T) {
	fm := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + wk:         sums(row("", "1000")),
			"ARS|income|" + wk:          sums(row("", "1500")),
			"ARS|expense|category" + wk: sums(row("Comida", "600"), row("Transporte", "400")),
		},
		tops:   map[string]*movement.Movement{"ARS": {Amount: dec("-350"), Description: strptr("Cena")}},
		counts: []movement.DayCount{{Date: to, Count: 5}, {Date: from, Count: 2}},
		bals:   map[uint64]decimal.Decimal{0: dec("2500")},
	}
	fa := fakeAccounts{
		list: map[uint64][]account.Account{1: {{Name: "Efectivo", Currency: currency.ARS}}},
	}
	text, err := NewBuilder(fm, fa, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, want := range []string{
		"Tu semana",
		"6 al 12 de julio",
		"Pesos",
		"Gastaste <b>$1.000</b> · entró $1.500",
		"Comida $600",
		"Lo más caro: <b>$350</b> · Cena",
		"Efectivo <b>$2.500</b>",
		// La salida se explica en palabras, no con un botón: un inline button
		// de Telegram queda tocable para siempre en el historial. El resumen
		// tiene que decir la frase que el router entiende.
		"no quiero más el resumen semanal",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	// La jerga contable y los centavos se fueron: son lo que hacía que el
	// mensaje leyera a planilla y no a resumen.
	for _, unwanted := range []string{"1000.00", "Neto", "Prom. diario", "Actividad", "%", "ARS"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("found %q (jerga contable) in:\n%s", unwanted, text)
		}
	}
}

func TestBuild_EscapesUserSuppliedStrings(t *testing.T) {
	fm := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + wk:         sums(row("", "1000")),
			"ARS|income|" + wk:          sums(row("", "1500")),
			"ARS|expense|category" + wk: sums(row("Comida & bebida", "600")),
		},
		tops:   map[string]*movement.Movement{"ARS": {Amount: dec("-350"), Description: strptr("Bar <El Rincón>")}},
		counts: []movement.DayCount{{Date: to, Count: 5}},
		bals:   map[uint64]decimal.Decimal{0: dec("2500")},
	}
	fa := fakeAccounts{
		list: map[uint64][]account.Account{1: {{Name: "Mercado & Pago <test>", Currency: currency.ARS}}},
	}

	text, err := NewBuilder(fm, fa, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// El resumen se manda con ParseMode HTML: un & o un < sin escapar hace que
	// Telegram devuelva 400 y el usuario no reciba NADA.
	for _, want := range []string{
		"Mercado &amp; Pago &lt;test&gt;",
		"Comida &amp; bebida",
		"Bar &lt;El Rincón&gt;",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing escaped %q in:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"Pago <test>", "Comida & bebida", "<El Rincón>"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("found unescaped %q in:\n%s", unwanted, text)
		}
	}
}

func TestBuild_BoldsTheValueThatAnswersEachLine(t *testing.T) {
	fm := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + wk:         sums(row("", "1000")),
			"ARS|income|" + wk:          sums(row("", "1500")),
			"ARS|expense|category" + wk: sums(row("Comida", "600")),
		},
		tops:   map[string]*movement.Movement{"ARS": {Amount: dec("-350"), Description: strptr("Cena")}},
		counts: []movement.DayCount{{Date: to, Count: 5}},
		bals:   map[uint64]decimal.Decimal{0: dec("2500")},
	}
	fa := fakeAccounts{
		list: map[uint64][]account.Account{1: {{Name: "Efectivo", Currency: currency.ARS}}},
	}

	text, err := NewBuilder(fm, fa, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	for _, want := range []string{
		"<b>Tu semana</b>",
		"<b>Pesos</b>",
		"Gastaste <b>$1.000</b>",
		"Lo más caro: <b>$350</b>",
		"<b>Hoy tenés</b>",
		"Efectivo <b>$2.500</b>",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}

	// Lo que entró es insumo de lo que gastaste, y el orden ya jerarquiza el
	// top de gastos: si TODO va en negrita, no resalta nada.
	for _, unwanted := range []string{"entró <b>", "Comida <b>"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("unexpected bold %q in:\n%s", unwanted, text)
		}
	}
}

// Un ajuste de saldo (revaluación de una cuenta con CEDEARs, por ejemplo) no es
// plata que entró: tiene su propio renglón y NO se mezcla con lo que gastaste.
func TestBuild_BalanceVariationIsNotIncome(t *testing.T) {
	fm := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + wk: sums(row("", "1000")),
			"ARS|income|" + wk:  sums(row("", "1500")),
			// Type nil + group by type: la consulta de variación (OnlyReserved).
			"ARS||type" + wk: sums(row("income", "84200")),
		},
		counts: []movement.DayCount{{Date: to, Count: 3}},
	}
	text, err := NewBuilder(fm, fakeAccounts{}, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(text, "Tus saldos subieron <b>$84.200</b> <i>por rendimientos y ajustes</i>") {
		t.Fatalf("falta el renglón de variación; got: %q", text)
	}
	// Lo que importa: lo que entró sigue siendo 1500, sin los 84200 encima.
	if !strings.Contains(text, "entró $1.500") {
		t.Fatalf("la variación no debe tocar el ingreso; got: %q", text)
	}
	if strings.Contains(text, "85.700") {
		t.Fatalf("la variación se sumó a los ingresos; got: %q", text)
	}
}

// Una semana sin ajustes no gasta un renglón en decir que no pasó nada.
func TestBuild_NoVariationLineWhenZero(t *testing.T) {
	fm := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + wk: sums(row("", "1000")),
			"ARS|income|" + wk:  sums(row("", "1500")),
		},
		counts: []movement.DayCount{{Date: to, Count: 3}},
	}
	text, err := NewBuilder(fm, fakeAccounts{}, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.Contains(text, "Tus saldos") {
		t.Fatalf("sin ajustes no debe aparecer el renglón; got: %q", text)
	}
}

// Semana en la que lo único que pasó fue un ajuste: el bloque no debe recitar
// "Gastaste $0" para después contar lo único que sí pasó.
func TestBuild_OnlyVariationSkipsTheZeroCashflowLines(t *testing.T) {
	fm := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS||type" + wk: sums(row("income", "84200")),
		},
		counts: []movement.DayCount{{Date: to, Count: 1}},
	}
	text, err := NewBuilder(fm, fakeAccounts{}, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(text, "Tus saldos subieron") {
		t.Fatalf("la variación debe contarse; got: %q", text)
	}
	if strings.Contains(text, "Gastaste") {
		t.Fatalf("no debe recitar el cashflow en cero; got: %q", text)
	}
}

// La comparación con la semana previa va en plata, no en porcentaje.
func TestBuild_ComparesWithPreviousWeekInMoney(t *testing.T) {
	fm := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + wk:    sums(row("", "1000")),
			"ARS|expense|" + prevW: sums(row("", "1500")),
		},
		counts: []movement.DayCount{{Date: to, Count: 3}},
	}
	text, _ := NewBuilder(fm, fakeAccounts{}, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	if !strings.Contains(text, "<i>↘ $500 menos que la semana pasada</i>") {
		t.Errorf("falta la comparación en plata; got:\n%s", text)
	}
	if strings.Contains(text, "%") {
		t.Errorf("el porcentaje no debería volver; got:\n%s", text)
	}
}

// B — la categoría que se disparó. Sin esto, "gastaste $500 menos" queda sin
// explicación, que es justo la pregunta que dispara.
func TestBuild_AnnotatesTheCategoryThatJumped(t *testing.T) {
	fm := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + wk:            sums(row("", "1000")),
			"ARS|expense|category" + wk:    sums(row("Comida", "600"), row("Transporte", "400")),
			"ARS|expense|category" + prevW: sums(row("Comida", "500"), row("Transporte", "100")),
		},
		counts: []movement.DayCount{{Date: to, Count: 3}},
	}
	text, _ := NewBuilder(fm, fakeAccounts{}, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	if !strings.Contains(text, "Transporte $400 · <i>$300 más que la semana pasada</i>") {
		t.Errorf("falta la anotación del salto; got:\n%s", text)
	}
	// Comida subió $100 sobre $500: 20%, ruido. Se anota UNA sola categoría.
	if strings.Contains(text, "Comida $600 ·") {
		t.Errorf("no debe anotar la categoría que apenas se movió; got:\n%s", text)
	}

	// Y si la ÚNICA categoría es la que apenas se movió, tampoco se anota: la
	// anotación vale porque es excepcional. Sin este caso el umbral no está
	// probado — Transporte gana igual por ser el salto más grande.
	quiet := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + wk:            sums(row("", "1000")),
			"ARS|expense|category" + wk:    sums(row("Comida", "600")),
			"ARS|expense|category" + prevW: sums(row("Comida", "500")),
		},
		counts: []movement.DayCount{{Date: to, Count: 3}},
	}
	text, _ = NewBuilder(quiet, fakeAccounts{}, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	if strings.Contains(text, "más que la semana pasada") {
		t.Errorf("un 20%% no es un salto; got:\n%s", text)
	}
}

// C2 — la proyección sale de la dispersión de los propios días del usuario, y
// se redondea: un rango con centavos se contradice a sí mismo.
func TestBuild_ProjectsMonthEndAsARange(t *testing.T) {
	var daily []movement.CategorySum
	for d := 1; d <= 6; d++ {
		daily = append(daily, row(time.Date(2026, 7, d, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), "1000"))
	}
	for d := 7; d <= 12; d++ {
		daily = append(daily, row(time.Date(2026, 7, d, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), "3000"))
	}
	fm := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + wk:       sums(row("", "1000")),
			"ARS|expense|day" + month: daily,
		},
		counts: []movement.DayCount{{Date: to, Count: 3}},
	}
	text, _ := NewBuilder(fm, fakeAccounts{}, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	// 12 días: p25=$1.000, p75=$3.000, lleva $24.000, quedan 19 días.
	// Piso 24.000+19.000=43.000 → $40.000. Techo 24.000+57.000=81.000 → $90.000.
	if !strings.Contains(text, "Julio va camino a cerrar entre <b>$40.000 y $90.000</b>") {
		t.Errorf("falta la proyección; got:\n%s", text)
	}
}

// Un día sin gastos es un día barato, no un dato que falta. SumForUser sólo
// devuelve los días que tuvieron movimientos: si la muestra fueran esas filas
// nomás, el piso saldría sesgado para arriba y la proyección inflaría.
func TestBuild_ProjectionCountsDaysWithoutSpending(t *testing.T) {
	var daily []movement.CategorySum
	for d := 7; d <= 12; d++ { // seis días con gasto; del 1 al 6, nada
		daily = append(daily, row(time.Date(2026, 7, d, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), "3000"))
	}
	fm := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + wk:       sums(row("", "1000")),
			"ARS|expense|day" + month: daily,
		},
		counts: []movement.DayCount{{Date: to, Count: 3}},
	}
	text, _ := NewBuilder(fm, fakeAccounts{}, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	// Con los seis ceros adentro: p25=$0, p75=$3.000, lleva $18.000, quedan 19
	// días. Piso 18.000 → $10.000. Techo 18.000+57.000=75.000 → $80.000.
	// Sin los ceros el piso saldría $70.000: siete veces más alto.
	if !strings.Contains(text, "entre <b>$10.000 y $80.000</b>") {
		t.Errorf("los días sin gasto no entraron en la muestra; got:\n%s", text)
	}
}

// Los primeros lunes del mes la muestra son tres días y el rango sale absurdo:
// ahí no se proyecta, la línea directamente no aparece.
func TestBuild_NoProjectionEarlyInTheMonth(t *testing.T) {
	earlyFrom := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	earlyTo := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	fm := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense||2026-06-29": sums(row("", "1000")),
			"ARS|expense|day" + month: sums(
				row("2026-07-01", "5000"), row("2026-07-02", "5000"),
				row("2026-07-03", "5000"), row("2026-07-04", "5000"), row("2026-07-05", "5000")),
		},
		counts: []movement.DayCount{{Date: earlyTo, Count: 3}},
	}
	text, _ := NewBuilder(fm, fakeAccounts{}, fakeIcons{}, fakeQuotes{}).Build(1, earlyFrom, earlyTo, earlyFrom.AddDate(0, 0, -7), earlyFrom.AddDate(0, 0, -1))
	if strings.Contains(text, "va camino a cerrar") {
		t.Errorf("con 5 días de muestra no se proyecta; got:\n%s", text)
	}
}

// Los saldos en dólares no se pierden ni se mezclan: van juntos en su renglón,
// con centavos, que a escala dólar sí importan.
func TestBuild_GroupsForeignAccountsInOneLine(t *testing.T) {
	fm := fakeMovements{
		sums:   map[string][]movement.CategorySum{"ARS|expense|" + wk: sums(row("", "1000"))},
		counts: []movement.DayCount{{Date: to, Count: 3}},
		bals: map[uint64]decimal.Decimal{
			1: dec("121208.07"), 2: dec("495.26"), 3: dec("3614.66"),
		},
	}
	fa := fakeAccounts{list: map[uint64][]account.Account{1: {
		{Model: gorm.Model{ID: 1}, Name: "Mercado Pago", Currency: currency.ARS},
		{Model: gorm.Model{ID: 2}, Name: "Mercado Pago", Currency: currency.USD},
		{Model: gorm.Model{ID: 3}, Name: "FCI", Currency: currency.USD},
	}}}
	text, _ := NewBuilder(fm, fa, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	if !strings.Contains(text, "Mercado Pago <b>$121.208</b>") {
		t.Errorf("falta el saldo en pesos; got:\n%s", text)
	}
	if !strings.Contains(text, "En dólares: Mercado Pago US$495,26 · FCI US$3.614,66") {
		t.Errorf("falta el renglón de dólares; got:\n%s", text)
	}
}

// "$267.999 menos que la semana pasada" es precisión de más: la comparación es
// una magnitud, no una liquidación. Se redondea igual que el rango proyectado.
func TestBuild_RoundsTheWeeklyComparison(t *testing.T) {
	fm := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + wk:    sums(row("", "453774.78")),
			"ARS|expense|" + prevW: sums(row("", "721774")),
		},
		counts: []movement.DayCount{{Date: to, Count: 3}},
	}
	text, _ := NewBuilder(fm, fakeAccounts{}, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	if !strings.Contains(text, "↘ $268.000 menos que la semana pasada") {
		t.Errorf("la comparación no se redondeó; got:\n%s", text)
	}
}

// Un rango que cruza el millón no se cuenta en millones: "$0,4 millones" es
// peor que "$400.000" — la escala existe para acortar, no para achicar.
func TestBuild_BandCrossingOneMillionStaysInPesos(t *testing.T) {
	var daily []movement.CategorySum
	for d := 1; d <= 12; d++ {
		amt := "42000"
		if d%3 == 0 {
			amt = "5000"
		}
		daily = append(daily, row(time.Date(2026, 7, d, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), amt))
	}
	fm := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + wk:       sums(row("", "1000")),
			"ARS|expense|day" + month: daily,
		},
		counts: []movement.DayCount{{Date: to, Count: 3}},
	}
	text, _ := NewBuilder(fm, fakeAccounts{}, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	if strings.Contains(text, "$0,4") {
		t.Errorf("un piso de menos de un millón no se cuenta en millones; got:\n%s", text)
	}
	if !strings.Contains(text, "entre <b>$450.000 y $1.160.000</b>") {
		t.Errorf("rango mal escalado; got:\n%s", text)
	}
}

// Sin semana previa con qué comparar, no hay salto que anunciar: "no gastaste
// acá la semana pasada" sobre un usuario que recién arranca es ruido, no dato.
func TestBuild_NoJumpAnnotationWithoutPreviousData(t *testing.T) {
	fm := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + wk:         sums(row("", "1000")),
			"ARS|expense|category" + wk: sums(row("Suscripciones", "120")),
		},
		counts: []movement.DayCount{{Date: to, Count: 3}},
	}
	text, _ := NewBuilder(fm, fakeAccounts{}, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	if strings.Contains(text, "la semana pasada") {
		t.Errorf("sin datos previos no se compara; got:\n%s", text)
	}
}

// Una sola categoría que ES todo el gasto no agrega nada: "Gastaste US$120,50 /
// Suscripciones US$120,50" dice el mismo número dos veces.
func TestBuild_SkipsTheCategoryListWhenItRepeatsTheTotal(t *testing.T) {
	fm := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + wk:         sums(row("", "120")),
			"ARS|expense|category" + wk: sums(row("Suscripciones", "120")),
		},
		counts: []movement.DayCount{{Date: to, Count: 3}},
	}
	text, _ := NewBuilder(fm, fakeAccounts{}, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	if strings.Contains(text, "Suscripciones") {
		t.Errorf("la única categoría repite el total; got:\n%s", text)
	}
}

// Los íconos son los mismos que muestra el resto del bot: el resumen era el
// único lugar donde la categoría aparecía pelada.
func TestBuild_UsesTheRealCategoryIcons(t *testing.T) {
	fm := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + wk:         sums(row("", "1000")),
			"ARS|expense|category" + wk: sums(row("Vivienda", "600"), row("Comida", "400")),
		},
		counts: []movement.DayCount{{Date: to, Count: 3}},
	}
	icons := fakeIcons{byCategory: map[string]string{"Vivienda": "🏠"}}
	text, _ := NewBuilder(fm, fakeAccounts{}, icons, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	if !strings.Contains(text, "🏠 Vivienda $600") {
		t.Errorf("falta el ícono real; got:\n%s", text)
	}
	// Sin ícono propio, el fallback del cache — nunca una categoría pelada.
	if !strings.Contains(text, "📂 Comida $400") {
		t.Errorf("falta el fallback; got:\n%s", text)
	}
}

// La línea de apertura reacciona a la semana, y sale de los números — no es
// una frase al azar. Sin motivo claro, no hay línea.
func TestBuild_OpeningLineReactsToTheWeek(t *testing.T) {
	week := func(spent, prev, earned string) string {
		m := map[string][]movement.CategorySum{
			"ARS|expense|" + wk:    sums(row("", spent)),
			"ARS|expense|" + prevW: sums(row("", prev)),
		}
		if earned != "" {
			m["ARS|income|"+wk] = sums(row("", earned))
		}
		fm := fakeMovements{sums: m, counts: []movement.DayCount{{Date: to, Count: 3}}}
		text, _ := NewBuilder(fm, fakeAccounts{}, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
		return text
	}
	for _, c := range []struct{ spent, prev, earned, want string }{
		{"2000", "1000", "", "<i>Semana cara</i>"},
		{"500", "1000", "", "<i>Semana tranquila</i>"},
		{"1000", "1000", "5000", "<i>Entró bastante más de lo que salió</i>"},
		{"1000", "1000", "", ""}, // nada que destacar: no se fuerza una frase
	} {
		text := week(c.spent, c.prev, c.earned)
		if c.want == "" {
			if strings.Contains(text, "Semana ") || strings.Contains(text, "Entró bastante") {
				t.Errorf("semana sin nada que destacar no lleva apertura; got:\n%s", text)
			}
			continue
		}
		if !strings.Contains(text, c.want) {
			t.Errorf("falta %q; got:\n%s", c.want, text)
		}
	}
}

// La nota va en blockquote: es el pie, no un renglón más del resumen.
func TestBuild_NoteIsAQuote(t *testing.T) {
	fm := fakeMovements{
		sums:   map[string][]movement.CategorySum{"ARS|expense|" + wk: sums(row("", "1000"))},
		counts: []movement.DayCount{{Date: to, Count: 3}},
	}
	text, _ := NewBuilder(fm, fakeAccounts{}, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	if !strings.Contains(text, "<blockquote>📩 Va cada lunes") || !strings.Contains(text, "</blockquote>") {
		t.Errorf("la nota no es un quote; got:\n%s", text)
	}
	if strings.Contains(text, "\n—\n") {
		t.Errorf("el separador a mano sobra con el quote; got:\n%s", text)
	}
}

// La nota de ajustar saldo solo tiene sentido si el resumen listó cuentas;
// sin cuentas no hay nada de qué "diferencia" hablar.
func TestBuild_BalanceNoteOnlyWithAccounts(t *testing.T) {
	fm := fakeMovements{
		sums:   map[string][]movement.CategorySum{"ARS|expense|" + wk: sums(row("", "1000"))},
		counts: []movement.DayCount{{Date: to, Count: 3}},
		bals:   map[uint64]decimal.Decimal{0: dec("2500")},
	}
	fa := fakeAccounts{
		list: map[uint64][]account.Account{1: {{Name: "Efectivo", Currency: currency.ARS}}},
	}
	text, _ := NewBuilder(fm, fa, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	if !strings.Contains(text, "<blockquote>💰 ¿<b>Diferencia de saldo</b>") {
		t.Errorf("falta la nota de ajuste de saldo con cuentas; got:\n%s", text)
	}

	textNoAccts, _ := NewBuilder(fm, fakeAccounts{}, fakeIcons{}, fakeQuotes{}).Build(1, from, to, prevFrom, prevTo)
	if strings.Contains(textNoAccts, "Diferencia de saldo") {
		t.Errorf("la nota de ajuste de saldo no debería aparecer sin cuentas; got:\n%s", textNoAccts)
	}
}
