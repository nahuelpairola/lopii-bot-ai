package summary

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

// fakeMovements returns canned aggregates. sums maps "CUR|type|groupBy" ->
// rows; tops maps "CUR" -> movement; counts is returned as-is.
type fakeMovements struct {
	sums   map[string][]movement.CategorySum
	tops   map[string]*movement.Movement
	counts []movement.DayCount
}

func (f fakeMovements) SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error) {
	typ := ""
	if q.Type != nil {
		typ = *q.Type
	}
	return f.sums[q.Currency.String()+"|"+typ+"|"+groupBy], nil
}
func (f fakeMovements) TopExpenseForUser(q movement.MovementQuery) (*movement.Movement, error) {
	return f.tops[q.Currency.String()], nil
}
func (f fakeMovements) CountByDayForUser(_ uint64, _, _ time.Time) ([]movement.DayCount, error) {
	return f.counts, nil
}

type fakeAccounts struct {
	list map[uint64][]account.Account
	bals map[uint64]decimal.Decimal
}

func (f fakeAccounts) FindByUserID(userID uint64) ([]account.Account, error) {
	return f.list[userID], nil
}
func (f fakeAccounts) SumAmountForAccount(id uint64) (decimal.Decimal, error) {
	return f.bals[id], nil
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

func TestBuild_EmptyWeek(t *testing.T) {
	b := NewBuilder(fakeMovements{counts: nil}, fakeAccounts{})
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
			"ARS|expense|":         sums(row("", "1000")),
			"ARS|income|":          sums(row("", "1500")),
			"ARS|expense|category": sums(row("Comida", "600"), row("Transporte", "400")),
		},
		tops:   map[string]*movement.Movement{"ARS": {Amount: dec("-350"), Merchant: strptr("Cena")}},
		counts: []movement.DayCount{{Date: to, Count: 5}, {Date: from, Count: 2}},
	}
	fa := fakeAccounts{
		list: map[uint64][]account.Account{1: {{Name: "Efectivo", Currency: currency.ARS}}},
		bals: map[uint64]decimal.Decimal{0: dec("2500")},
	}
	text, err := NewBuilder(fm, fa).Build(1, from, to, prevFrom, prevTo)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, want := range []string{
		"Resumen semanal",
		"ARS",
		"1500.00", // entró
		"1000.00", // salió
		"500.00",  // neto
		"Comida",
		"350.00",      // biggest expense
		"7 registros", // activity count (2+5)
		"domingo",     // top day = the day with count 5 (to = 2026-07-12 is Sunday)
		"Efectivo",
		"2500.00",
		"🔕", // note references the disable button
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}
