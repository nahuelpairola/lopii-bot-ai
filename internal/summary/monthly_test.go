package summary

import (
	"strings"
	"testing"
	"time"

	"lopiibot.com/internal/movement"
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
	return NewBuilder(m, a, fakeIcons{})
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

	for _, want := range []string{"Julio cerró", "$2.100.000", "$1.240.000", "$860.000"} {
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

	if !strings.Contains(p.Text, "Se te fueron") {
		t.Errorf("missing the negative-leftover sentence in:\n%s", p.Text)
	}
	if strings.Contains(p.Text, "-$") || strings.Contains(p.Text, "$-") {
		t.Errorf("a sign escaped into the copy:\n%s", p.Text)
	}
}
