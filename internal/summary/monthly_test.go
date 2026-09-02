package summary

import (
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/quote"
)

var (
	mFrom     = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	mTo       = time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	mPrevFrom = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	mPrevTo   = time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
)

const (
	julKey = "|2026-07-01"
	junKey = "|2026-06-01"
)

func monthlyBuilder(t *testing.T, m fakeMovements, a fakeAccounts) *Builder {
	t.Helper()
	return NewBuilder(m, a, fakeIcons{}, fakeQuotes{})
}

func TestBuildMonthly_OpensWithIncomeSpentAndLeftover(t *testing.T) {
	m := fakeMovements{sums: map[string][]movement.CategorySum{
		"ARS|expense|" + julKey: sums(row("", "1240000")),
		"ARS|income|" + julKey:  sums(row("", "2100000")),
		"ARS|expense|" + junKey: sums(row("", "1420000")),
	}}
	p, err := monthlyBuilder(t, m, fakeAccounts{}).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)
	if err != nil {
		t.Fatalf("BuildMonthly: %v", err)
	}

	for _, want := range []string{"Así cerró julio", "$2.100.000", "$1.240.000", "$860.000"} {
		if !strings.Contains(p.Text, want) {
			t.Errorf("missing %q in:\n%s", want, p.Text)
		}
	}
}

func TestBuildMonthly_ComparesAgainstThePreviousMonthInMoney(t *testing.T) {
	m := fakeMovements{sums: map[string][]movement.CategorySum{
		"ARS|expense|" + julKey: sums(row("", "1240000")),
		"ARS|income|" + julKey:  sums(row("", "2100000")),
		"ARS|expense|" + junKey: sums(row("", "1420000")),
	}}
	p, _ := monthlyBuilder(t, m, fakeAccounts{}).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)

	if !strings.Contains(p.Text, "$180.000 menos que en junio") {
		t.Errorf("missing the money comparison in:\n%s", p.Text)
	}
	if strings.Contains(p.Text, "%") {
		t.Errorf("percentages are forbidden in the comparison:\n%s", p.Text)
	}
}

func TestBuildMonthly_EmptyMonthSendsNothing(t *testing.T) {
	p, err := monthlyBuilder(t, fakeMovements{}, fakeAccounts{}).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)
	if err != nil {
		t.Fatalf("BuildMonthly: %v", err)
	}
	if p.Text != "" {
		t.Fatalf("expected the zero Prompt for an empty month, got:\n%s", p.Text)
	}
	if len(p.Buttons) != 0 {
		t.Fatalf("expected no buttons on the zero Prompt, got %d", len(p.Buttons))
	}
}

func TestBuildMonthly_ZeroIncomeDoesNotReciteIt(t *testing.T) {
	m := fakeMovements{sums: map[string][]movement.CategorySum{
		"ARS|expense|" + julKey: sums(row("", "1240000")),
		"ARS|expense|" + junKey: sums(row("", "1420000")),
	}}
	p, _ := monthlyBuilder(t, m, fakeAccounts{}).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)

	if strings.Contains(p.Text, "$0") {
		t.Errorf("a zero is recited in:\n%s", p.Text)
	}
	if !strings.Contains(p.Text, "$1.240.000") {
		t.Errorf("missing what was spent in:\n%s", p.Text)
	}
}

func TestBuildMonthly_NegativeLeftoverSaysItPlainly(t *testing.T) {
	m := fakeMovements{sums: map[string][]movement.CategorySum{
		"ARS|expense|" + julKey: sums(row("", "1500000")),
		"ARS|income|" + julKey:  sums(row("", "1200000")),
		"ARS|expense|" + junKey: sums(row("", "1420000")),
	}}
	p, _ := monthlyBuilder(t, m, fakeAccounts{}).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)

	if !strings.Contains(p.Text, "se te fueron") {
		t.Errorf("missing the negative-leftover sentence in:\n%s", p.Text)
	}
	if strings.Contains(p.Text, "-$") || strings.Contains(p.Text, "$-") {
		t.Errorf("a sign escaped into the copy:\n%s", p.Text)
	}
}

func TestBalanceAt_AccumulatesFullHistoryAndStopsAtTheMonth(t *testing.T) {
	deltas := []movement.MonthlyDelta{
		{Month: "2026-05", Delta: dec("100000")},
		{Month: "2026-06", Delta: dec("135000")},
		{Month: "2026-07", Delta: dec("85000")},
		{Month: "2026-08", Delta: dec("999999")},
	}

	closing, delta := balanceAt(deltas, "2026-07")

	if !closing.Equal(dec("320000")) {
		t.Errorf("closing = %s, want 320000", closing)
	}
	if !delta.Equal(dec("85000")) {
		t.Errorf("delta = %s, want 85000", delta)
	}
}

func TestBalanceAt_MonthWithNoActivityCarriesTheBalance(t *testing.T) {
	deltas := []movement.MonthlyDelta{{Month: "2026-05", Delta: dec("100000")}}

	closing, delta := balanceAt(deltas, "2026-07")

	if !closing.Equal(dec("100000")) {
		t.Errorf("closing = %s, want 100000", closing)
	}
	if !delta.IsZero() {
		t.Errorf("delta = %s, want 0", delta)
	}
}

func TestBuildMonthly_ListsAccountsThatMovedWithTheirCloseBalance(t *testing.T) {
	m := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + julKey: sums(row("", "1240000")),
			"ARS|income|" + julKey:  sums(row("", "2100000")),
		},
		deltas: map[uint64][]movement.MonthlyDelta{
			1: {{Month: "2026-06", Delta: dec("235000")}, {Month: "2026-07", Delta: dec("85000")}},
			2: {{Month: "2026-06", Delta: dec("552000")}, {Month: "2026-07", Delta: dec("-12000")}},
		},
	}
	a := fakeAccounts{list: map[uint64][]account.Account{1: {
		{Model: gorm.Model{ID: 1}, UserID: 1, Name: "Mercado Pago", Currency: currency.ARS},
		{Model: gorm.Model{ID: 2}, UserID: 1, Name: "Galicia", Currency: currency.ARS},
	}}}

	p, _ := monthlyBuilder(t, m, a).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)

	for _, want := range []string{"Mercado Pago", "$320.000", "↗", "Galicia", "$540.000", "↘"} {
		if !strings.Contains(p.Text, want) {
			t.Errorf("missing %q in:\n%s", want, p.Text)
		}
	}
}

func TestBuildMonthly_AccountThatDidNotMoveIsNotListed(t *testing.T) {
	m := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + julKey: sums(row("", "1240000")),
		},
		deltas: map[uint64][]movement.MonthlyDelta{
			1: {{Month: "2026-07", Delta: dec("85000")}},
			2: {{Month: "2026-01", Delta: dec("50000")}},
		},
	}
	a := fakeAccounts{list: map[uint64][]account.Account{1: {
		{Model: gorm.Model{ID: 1}, UserID: 1, Name: "Mercado Pago", Currency: currency.ARS},
		{Model: gorm.Model{ID: 2}, UserID: 1, Name: "Dormida", Currency: currency.ARS},
	}}}

	p, _ := monthlyBuilder(t, m, a).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)

	if strings.Contains(p.Text, "Dormida") {
		t.Errorf("an account with no movement in the month was listed:\n%s", p.Text)
	}
}

func TestBuildMonthly_NoAccountsOmitsTheBlock(t *testing.T) {
	m := fakeMovements{sums: map[string][]movement.CategorySum{
		"ARS|expense|" + julKey: sums(row("", "1240000")),
	}}

	p, _ := monthlyBuilder(t, m, fakeAccounts{}).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)

	if strings.Contains(p.Text, "🏦") {
		t.Errorf("the accounts block was rendered with no accounts:\n%s", p.Text)
	}
}

func TestBuildMonthly_ZeroVariationOmitsTheAside(t *testing.T) {
	m := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + julKey: sums(row("", "1240000")),
		},
		deltas: map[uint64][]movement.MonthlyDelta{
			1: {{Month: "2026-07", Delta: dec("85000")}},
		},
	}
	a := fakeAccounts{list: map[uint64][]account.Account{1: {
		{Model: gorm.Model{ID: 1}, UserID: 1, Name: "Mercado Pago", Currency: currency.ARS},
	}}}

	p, _ := monthlyBuilder(t, m, a).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)

	if strings.Contains(p.Text, "no es plata que entró") {
		t.Errorf("the variation aside was rendered with zero variation:\n%s", p.Text)
	}
}

func TestBuildMonthly_DollarLineNamesTheQuoteRowsOwnDate(t *testing.T) {
	m := fakeMovements{sums: map[string][]movement.CategorySum{
		"ARS|expense|" + julKey: sums(row("", "1240000")),
	}}
	q := fakeQuotes{rows: map[string]*quote.Quote{
		rateTypeMEP: {
			Date:     time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC),
			RateType: rateTypeMEP,
			Ask:      dec("1204"),
		},
	}}

	p, _ := NewBuilder(m, fakeAccounts{}, fakeIcons{}, q).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)

	if !strings.Contains(p.Text, "29 de julio") {
		t.Errorf("the line must name the quote row's own date, not the 31st:\n%s", p.Text)
	}
	if strings.Contains(p.Text, "31 de julio") {
		t.Errorf("the asked-for date leaked into the copy:\n%s", p.Text)
	}
	if !strings.Contains(p.Text, "$1.204") {
		t.Errorf("the line must name the rate it converted at:\n%s", p.Text)
	}
	if !strings.Contains(p.Text, "US$1.030") {
		t.Errorf("expected 1240000/1204 rounded = US$1.030 in:\n%s", p.Text)
	}
}

func TestBuildMonthly_NoQuoteOmitsTheDollarLine(t *testing.T) {
	m := fakeMovements{sums: map[string][]movement.CategorySum{
		"ARS|expense|" + julKey: sums(row("", "1240000")),
	}}

	p, _ := NewBuilder(m, fakeAccounts{}, fakeIcons{}, fakeQuotes{}).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)

	if strings.Contains(p.Text, "US$") {
		t.Errorf("a dollar figure was invented with no quote:\n%s", p.Text)
	}
}

func TestBuildMonthly_RunwayNeedsThreeClosedMonths(t *testing.T) {
	m := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + julKey: sums(row("", "1240000")),
		},
		deltas: map[uint64][]movement.MonthlyDelta{
			1: {{Month: "2026-07", Delta: dec("2400000")}},
		},
	}
	a := fakeAccounts{list: map[uint64][]account.Account{1: {
		{Model: gorm.Model{ID: 1}, UserID: 1, Name: "Galicia", Currency: currency.ARS},
	}}}

	p, _ := NewBuilder(m, a, fakeIcons{}, fakeQuotes{}).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)

	if strings.Contains(p.Text, "gastando como venís gastando") {
		t.Errorf("runway rendered with fewer than 3 closed months of expense:\n%s", p.Text)
	}
}

func TestRunwayWords_RoundsToHalfMonths(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0.4, "menos de un mes"},
		{1.1, "un mes"},
		{1.8, "casi dos meses"},
		{2.5, "dos meses y medio"},
		{4.2, "más de cuatro meses"},
		{13.0, "más de un año"},
	}
	for _, c := range cases {
		if got := runwayWords(c.in); got != c.want {
			t.Errorf("runwayWords(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBuildMonthly_FlagsTheCategoryThatJumped(t *testing.T) {
	m := fakeMovements{sums: map[string][]movement.CategorySum{
		"ARS|expense|" + julKey:                            sums(row("", "1240000")),
		"ARS|expense|" + movement.GroupByCategory + julKey: sums(row("Transporte", "150000"), row("Súper", "420000")),
		"ARS|expense|" + movement.GroupByCategory + junKey: sums(row("Transporte", "75000"), row("Súper", "430000")),
	}}

	p, _ := monthlyBuilder(t, m, fakeAccounts{}).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)

	if !strings.Contains(p.Text, "⚠️") || !strings.Contains(p.Text, "<b>Transporte</b>") {
		t.Errorf("the jumped category is missing:\n%s", p.Text)
	}
	if strings.Contains(p.Text, "⚠️ Lo que más cambió: <b>Súper</b>") {
		t.Errorf("a category that did not jump was flagged:\n%s", p.Text)
	}
}

func TestBuildMonthly_NoJumpOmitsTheWarningLine(t *testing.T) {
	m := fakeMovements{sums: map[string][]movement.CategorySum{
		"ARS|expense|" + julKey:                            sums(row("", "1240000")),
		"ARS|expense|" + movement.GroupByCategory + julKey: sums(row("Súper", "420000"), row("Casa", "260000")),
		"ARS|expense|" + movement.GroupByCategory + junKey: sums(row("Súper", "410000"), row("Casa", "250000")),
	}}

	p, _ := monthlyBuilder(t, m, fakeAccounts{}).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)

	if strings.Contains(p.Text, "⚠️") {
		t.Errorf("a warning line was rendered with no jump:\n%s", p.Text)
	}
}

func TestBuildMonthly_SingleCategoryOmitsTheFoldedBlock(t *testing.T) {
	m := fakeMovements{sums: map[string][]movement.CategorySum{
		"ARS|expense|" + julKey:                            sums(row("", "420000")),
		"ARS|expense|" + movement.GroupByCategory + julKey: sums(row("Súper", "420000")),
	}}

	p, _ := monthlyBuilder(t, m, fakeAccounts{}).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)

	if strings.Contains(p.Text, "blockquote expandable") {
		t.Errorf("the fold repeats the one figure already stated:\n%s", p.Text)
	}
}

func TestBuildMonthly_CarriesOneWebAppButtonForThatMonth(t *testing.T) {
	m := fakeMovements{sums: map[string][]movement.CategorySum{
		"ARS|expense|" + julKey: sums(row("", "1240000")),
	}}

	p, _ := monthlyBuilder(t, m, fakeAccounts{}).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)

	if len(p.Buttons) != 1 {
		t.Fatalf("Buttons = %d, want 1", len(p.Buttons))
	}
	btn := p.Buttons[0]
	if btn.WebAppPath != "/app/overview?p=month&m=2026-07" {
		t.Errorf("WebAppPath = %q", btn.WebAppPath)
	}
	if btn.Data != "" {
		t.Errorf("Data = %q, want empty — Telegram rejects a button carrying both", btn.Data)
	}
	if !strings.Contains(btn.Label, "julio") {
		t.Errorf("Label = %q, want the reported month named", btn.Label)
	}
}

func TestBuildMonthly_EscapesUserText(t *testing.T) {
	m := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + julKey:                            sums(row("", "420000")),
			"ARS|expense|" + movement.GroupByCategory + julKey: sums(row("Súper", "300000"), row("Casa", "120000")),
		},
		tops: map[string]*movement.Movement{
			"ARS": {Description: strptr("neumáticos <b>& llantas</b>"), Amount: dec("-96000"), Type: constants.Expense, Currency: currency.ARS},
		},
	}

	p, _ := monthlyBuilder(t, m, fakeAccounts{}).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)

	if strings.Contains(p.Text, "<b>& llantas</b>") {
		t.Errorf("user text reached the message unescaped — Telegram will 400:\n%s", p.Text)
	}
	if !strings.Contains(p.Text, "&amp;") {
		t.Errorf("expected the ampersand escaped in:\n%s", p.Text)
	}
}

func TestSpellNumber_CoversTheRangeBothCallersNeed(t *testing.T) {
	cases := map[int]string{
		1: "un", 2: "dos", 4: "cuatro", 7: "siete", 10: "diez", 11: "once",
		0: "", 12: "", -1: "",
	}
	for in, want := range cases {
		if got := spellNumber(in); got != want {
			t.Errorf("spellNumber(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestJumpWords_SpeaksTheMagnitude(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{1.35, "bastante más que en julio"},
		{1.5, "la mitad más que en julio"},
		{1.7, "casi el doble de julio"},
		{2.1, "el doble de julio"},
		{2.7, "casi el triple de julio"},
		{3.0, "el triple de julio"},
		{4.0, "cuatro veces lo de julio"},
		{7.005, "siete veces lo de julio"},
		{12.0, "más de diez veces lo de julio"},
	}
	for _, c := range cases {
		if got := jumpWords(c.in, "julio"); got != c.want {
			t.Errorf("jumpWords(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestShareWords_SpeaksTheFraction(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0.70, "dos tercios"},
		{0.50, "la mitad"},
		{0.33, "un tercio"},
		{0.25, "un cuarto"},
		{0.10, ""},
	}
	for _, c := range cases {
		if got := shareWords(c.in); got != c.want {
			t.Errorf("shareWords(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBuildMonthly_AccountRiseNamesThePreviousMonth(t *testing.T) {
	m := fakeMovements{
		sums: map[string][]movement.CategorySum{
			"ARS|expense|" + julKey: sums(row("", "1240000")),
		},
		deltas: map[uint64][]movement.MonthlyDelta{
			1: {{Month: "2026-06", Delta: dec("235000")}, {Month: "2026-07", Delta: dec("85000")}},
		},
	}
	a := fakeAccounts{list: map[uint64][]account.Account{1: {
		{Model: gorm.Model{ID: 1}, UserID: 1, Name: "Mercado Pago", Currency: currency.ARS},
	}}}

	p, _ := monthlyBuilder(t, m, a).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)

	if !strings.Contains(p.Text, "cierre de junio") {
		t.Errorf("a 31-day month must not roll over into itself:\n%s", p.Text)
	}
	if strings.Contains(p.Text, "cierre de julio") {
		t.Errorf("the reported month was named as its own predecessor:\n%s", p.Text)
	}
}

func TestBuildMonthly_SaysWhatLivingCostPerDay(t *testing.T) {
	m := fakeMovements{sums: map[string][]movement.CategorySum{
		"ARS|expense|" + julKey: sums(row("", "1240000")),
		"ARS|income|" + julKey:  sums(row("", "2100000")),
	}}
	p, err := monthlyBuilder(t, m, fakeAccounts{}).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)
	if err != nil {
		t.Fatalf("BuildMonthly: %v", err)
	}

	want := "<b>VIVIR TE SALIÓ $40.000 POR DÍA</b>"
	if !strings.Contains(p.Text, want) {
		t.Errorf("missing %q in:\n%s", want, p.Text)
	}
}

func TestBuildMonthly_PerDayDividesByEveryDayOfTheClosedMonth(t *testing.T) {
	febFrom := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	febTo := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	m := fakeMovements{sums: map[string][]movement.CategorySum{
		"ARS|expense||2026-02-01": sums(row("", "280000")),
	}}
	p, err := monthlyBuilder(t, m, fakeAccounts{}).BuildMonthly(1, febFrom, febTo, mPrevFrom, mPrevTo)
	if err != nil {
		t.Fatalf("BuildMonthly: %v", err)
	}

	if !strings.Contains(p.Text, "$10.000 POR DÍA") {
		t.Errorf("february must divide by 28, not 31, in:\n%s", p.Text)
	}
}

func TestBuildMonthly_NoSpendingOmitsThePerDayLine(t *testing.T) {
	m := fakeMovements{sums: map[string][]movement.CategorySum{
		"ARS|income|" + julKey: sums(row("", "2100000")),
	}}
	p, err := monthlyBuilder(t, m, fakeAccounts{}).BuildMonthly(1, mFrom, mTo, mPrevFrom, mPrevTo)
	if err != nil {
		t.Fatalf("BuildMonthly: %v", err)
	}

	if strings.Contains(p.Text, "POR DÍA") {
		t.Errorf("a month with no spending must not price a day:\n%s", p.Text)
	}
}
