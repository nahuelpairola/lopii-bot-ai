package messaging

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

// --- fakes scoped to the query-executor use-case tests ---

type fakeQueryMovements struct {
	lastQuery   movement.MovementQuery
	lastGroupBy string
	lastLimit   int
	sumRows     []movement.CategorySum
	listRows    []movement.Movement
	balances    map[uint64]decimal.Decimal
}

func (r *fakeQueryMovements) SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error) {
	r.lastQuery, r.lastGroupBy = q, groupBy
	return r.sumRows, nil
}
func (r *fakeQueryMovements) ListForUser(q movement.MovementQuery, limit int) ([]movement.Movement, error) {
	r.lastQuery, r.lastLimit = q, limit
	return r.listRows, nil
}
func (r *fakeQueryMovements) SumAmountForAccount(id uint64) (decimal.Decimal, error) {
	return r.balances[id], nil
}
func (r *fakeQueryMovements) InsertBatch([]movement.Movement) error              { return nil }
func (r *fakeQueryMovements) ReplaceMovements([]uint, []movement.Movement) error { return nil }
func (r *fakeQueryMovements) FindSimilarForUser(uint64, string, time.Time, *time.Time) ([]movement.Movement, error) {
	return nil, nil
}
func (r *fakeQueryMovements) FindRecentlyCreatedForUser(uint64, time.Time, int) ([]movement.Movement, error) {
	return nil, nil
}
func (r *fakeQueryMovements) SoftDeleteByIDs([]uint) error                               { return nil }
func (r *fakeQueryMovements) InsertAccountsWithOpenings([]movement.AccountOpening) error { return nil }
func (r *fakeQueryMovements) ReassignAccount(uint64, uint64) error                       { return nil }
func (r *fakeQueryMovements) CountForUser(uint64) (int64, error)                         { return 0, nil }
func (r *fakeQueryMovements) CountBySubcategory(uint64, uint64) (int64, error)           { return 0, nil }
func (r *fakeQueryMovements) ReassignSubcategory(uint64, uint64, uint64) error           { return nil }
func (r *fakeQueryMovements) TopMerchantsBySubcategory(uint64, uint64, int) ([]string, error) {
	return nil, nil
}

type fakeQueryAccounts struct{ accts []account.Account }

func (r *fakeQueryAccounts) FindByUserID(uint64) ([]account.Account, error) { return r.accts, nil }
func (r *fakeQueryAccounts) Insert(*account.Account) error                  { return nil }
func (r *fakeQueryAccounts) FindDefaultByCurrency(uint64, currency.Currency) (*account.Account, error) {
	return nil, nil
}
func (r *fakeQueryAccounts) GetAccount(uint64) (*account.Account, error)          { return nil, nil }
func (r *fakeQueryAccounts) Rename(uint64, string) error                          { return nil }
func (r *fakeQueryAccounts) UnsetDefault(uint64, currency.Currency) error         { return nil }
func (r *fakeQueryAccounts) SetDefault(uint64) error                              { return nil }
func (r *fakeQueryAccounts) HasDefaultForCurrency(uint64, currency.Currency) bool { return false }

type fakeQuerySubcats struct {
	subs          []subcategory.Subcategory
	owned         []subcategory.Subcategory
	deletedUserID uint64
	deletedID     uint64
	deleteCalls   int
	deleteErr     error
}

func (r *fakeQuerySubcats) FindAllForUser(uint64) ([]subcategory.Subcategory, error) {
	return r.subs, nil
}
func (r *fakeQuerySubcats) FindByCategoryAndSubcategory(uint64, string, string) (*subcategory.Subcategory, error) {
	return nil, nil
}
func (r *fakeQuerySubcats) DistinctCategoriesForUser(uint64) ([]string, error) { return nil, nil }
func (r *fakeQuerySubcats) IconForCategory(uint64, string) string              { return "📂" }
func (r *fakeQuerySubcats) Insert(*subcategory.Subcategory) error              { return nil }
func (r *fakeQuerySubcats) Reload() error                                      { return nil }
func (r *fakeQuerySubcats) Delete(userID uint64, id uint64) error {
	r.deletedUserID, r.deletedID = userID, id
	r.deleteCalls++
	return r.deleteErr
}
func (r *fakeQuerySubcats) FindOwnedByUser(userID uint64) ([]subcategory.Subcategory, error) {
	return r.owned, nil
}

func newQueryController(m *fakeQueryMovements, a *fakeQueryAccounts, s *fakeQuerySubcats) *controller {
	return &controller{movements: m, accounts: a, subcategories: s}
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// Case: "qué categorías hay y para qué sirve cada una"
func TestExec_ListCategories(t *testing.T) {
	s := &fakeQuerySubcats{subs: []subcategory.Subcategory{
		{Category: "Comida", Subcategory: "Restaurante", Description: "cuando comés afuera"},
		{Category: "Auto", Subcategory: "Nafta", Description: "combustible del auto"},
	}}
	exec := newQueryController(&fakeQueryMovements{}, &fakeQueryAccounts{}, s).buildQueryExecutor(1)

	out, err := exec("list_categories", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "comés afuera") || !strings.Contains(out, "combustible del auto") {
		t.Fatalf("descriptions missing: %s", out)
	}
	out, _ = exec("list_categories", json.RawMessage(`{"category":"Auto"}`))
	if strings.Contains(out, "Restaurante") {
		t.Errorf("category filter leaked another category: %s", out)
	}
}

func TestExec_ListCategories_LeadsWithIcon(t *testing.T) {
	s := &fakeQuerySubcats{subs: []subcategory.Subcategory{
		{Category: "Comida", Subcategory: "Restaurante", Description: "afuera", Icon: "🍔"},
	}}
	exec := newQueryController(&fakeQueryMovements{}, &fakeQueryAccounts{}, s).buildQueryExecutor(1)

	out, err := exec("list_categories", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "🍔 Comida") {
		t.Errorf("category line should lead with its icon, got: %s", out)
	}
}

func TestExec_SumMovements_GroupByCategory_LeadsWithIcon(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "Comida", Total: dec("5000")}}}
	s := &fakeQuerySubcats{} // IconForCategory returns "📂"
	exec := newQueryController(m, &fakeQueryAccounts{}, s).buildQueryExecutor(1)

	out, _ := exec("sum_movements", json.RawMessage(`{"from":"2026-05-01","to":"2026-05-31","currency":"ARS","group_by":"category"}`))
	if !strings.Contains(out, "📂 Comida") {
		t.Errorf("category total should lead with an icon, got: %s", out)
	}
}

// Case: "cuánto gasté en comida en mayo" — range + category + currency,
// transfer excluded by default (Type nil).
func TestExec_SumMovements_FoodInMay(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "", Total: dec("5000")}}}
	exec := newQueryController(m, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)

	out, err := exec("sum_movements", json.RawMessage(`{"from":"2026-05-01","to":"2026-05-31","currency":"ARS","category":"Comida"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.lastQuery.Currency != currency.ARS {
		t.Errorf("currency = %v", m.lastQuery.Currency)
	}
	if m.lastQuery.Category == nil || *m.lastQuery.Category != "Comida" {
		t.Errorf("category filter not set")
	}
	if m.lastQuery.Type != nil {
		t.Errorf("Type must be nil so the repo excludes transfers by default")
	}
	if m.lastQuery.From.Format("2006-01-02") != "2026-05-01" || m.lastQuery.To.Format("2006-01-02") != "2026-05-31" {
		t.Errorf("range = %v..%v", m.lastQuery.From, m.lastQuery.To)
	}
	if !strings.Contains(out, "5000") {
		t.Errorf("total not rendered: %s", out)
	}
}

// Case: "resumen del mes por categoría"
func TestExec_SumMovements_GroupByCategory(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "Comida", Total: dec("5000")}, {Label: "Auto", Total: dec("3000")}}}
	exec := newQueryController(m, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)

	out, _ := exec("sum_movements", json.RawMessage(`{"from":"2026-05-01","to":"2026-05-31","currency":"ARS","group_by":"category"}`))
	if m.lastGroupBy != "category" {
		t.Errorf("groupBy = %q", m.lastGroupBy)
	}
	if !strings.Contains(out, "Comida") || !strings.Contains(out, "Auto") {
		t.Errorf("grouped rows missing: %s", out)
	}
}

// Case: "gastos por cuenta" — group_by=account maps account_id label to name.
func TestExec_SumMovements_GroupByAccount_MapsNames(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "7", Total: dec("1000")}}}
	a := &fakeQueryAccounts{accts: []account.Account{{Model: gorm.Model{ID: 7}, Name: "Banco", Currency: currency.ARS}}}
	exec := newQueryController(m, a, &fakeQuerySubcats{}).buildQueryExecutor(1)

	out, _ := exec("sum_movements", json.RawMessage(`{"from":"2026-05-01","to":"2026-05-31","currency":"ARS","group_by":"account"}`))
	if m.lastGroupBy != "account" {
		t.Errorf("groupBy = %q", m.lastGroupBy)
	}
	if !strings.Contains(out, "Banco") {
		t.Errorf("account id 7 not mapped to name: %s", out)
	}
}

// Cases: "promedio mensual del auto" / "5k más que el mes pasado" / "20k por
// día" — the data side. The loop composes these from per-month totals.
func TestExec_SumMovements_GroupByMonth(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "2026-04", Total: dec("8000")}, {Label: "2026-05", Total: dec("13000")}}}
	exec := newQueryController(m, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)

	out, _ := exec("sum_movements", json.RawMessage(`{"from":"2026-04-01","to":"2026-05-31","currency":"ARS","group_by":"month","category":"Auto"}`))
	if m.lastGroupBy != "month" {
		t.Errorf("groupBy = %q", m.lastGroupBy)
	}
	if !strings.Contains(out, "2026-04") || !strings.Contains(out, "2026-05") {
		t.Errorf("per-month buckets missing: %s", out)
	}
}

// Case: "cuánto me rindió el broker" — income + Sistema|Rendimiento inversión
// + account resolution to an id.
func TestExec_SumMovements_BrokerIncome(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "", Total: dec("10000")}}}
	a := &fakeQueryAccounts{accts: []account.Account{{Model: gorm.Model{ID: 3}, Name: "Broker", Currency: currency.ARS}}}
	exec := newQueryController(m, a, &fakeQuerySubcats{}).buildQueryExecutor(1)

	_, err := exec("sum_movements", json.RawMessage(`{"from":"2026-01-01","to":"2026-12-31","currency":"ARS","type":"income","subcategory":"Rendimiento inversión","account":"Broker"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.lastQuery.Type == nil || *m.lastQuery.Type != "income" {
		t.Errorf("type filter not income")
	}
	if m.lastQuery.Subcategory == nil || *m.lastQuery.Subcategory != "Rendimiento inversión" {
		t.Errorf("subcategory filter not set")
	}
	if m.lastQuery.AccountID == nil || *m.lastQuery.AccountID != 3 {
		t.Errorf("account name not resolved to id 3")
	}
}

// Case: "mis compras en Carrefour" — merchant filter + abs rendering.
func TestExec_ListMovements_MerchantAbs(t *testing.T) {
	desc := "compra semanal"
	m := &fakeQueryMovements{listRows: []movement.Movement{{
		Type:        movement.Expense,
		Amount:      dec("-1500"), // stored signed; must render abs
		Currency:    currency.ARS,
		Date:        time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		Description: &desc,
		Subcategory: &subcategory.Subcategory{Category: "Comida", Subcategory: "Super"},
	}}}
	exec := newQueryController(m, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)

	out, err := exec("list_movements", json.RawMessage(`{"from":"2026-05-01","to":"2026-05-31","currency":"ARS","merchant":"Carrefour","limit":5}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.lastQuery.Merchant == nil || *m.lastQuery.Merchant != "Carrefour" {
		t.Errorf("merchant filter not set")
	}
	if m.lastLimit != 5 {
		t.Errorf("limit = %d", m.lastLimit)
	}
	if !strings.Contains(out, "1500.00") {
		t.Errorf("amount not rendered: %s", out)
	}
	if strings.Contains(out, "-1500") {
		t.Errorf("SIGNED amount leaked — must be abs: %s", out)
	}
}

// Case: "mis últimos movimientos de Mercado Pago" (account) + default limit.
func TestExec_ListMovements_AccountAndDefaultLimit(t *testing.T) {
	m := &fakeQueryMovements{listRows: []movement.Movement{}}
	a := &fakeQueryAccounts{accts: []account.Account{{Model: gorm.Model{ID: 9}, Name: "Mercado Pago", Currency: currency.ARS}}}
	exec := newQueryController(m, a, &fakeQuerySubcats{}).buildQueryExecutor(1)

	_, err := exec("list_movements", json.RawMessage(`{"from":"2026-05-01","to":"2026-05-31","currency":"ARS","account":"Mercado Pago"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.lastQuery.AccountID == nil || *m.lastQuery.AccountID != 9 {
		t.Errorf("account not resolved to id 9")
	}
}

// Cases: "mi saldo en cada cuenta" / "cuánto tengo en total" — signed balance,
// per currency, never mixed.
func TestExec_AccountBalance_AllSignedPerCurrency(t *testing.T) {
	a := &fakeQueryAccounts{accts: []account.Account{
		{Model: gorm.Model{ID: 1}, Name: "Banco", Currency: currency.ARS},
		{Model: gorm.Model{ID: 2}, Name: "Wallet USD", Currency: currency.USD},
	}}
	m := &fakeQueryMovements{balances: map[uint64]decimal.Decimal{1: dec("15000"), 2: dec("-200")}}
	exec := newQueryController(m, a, &fakeQuerySubcats{}).buildQueryExecutor(1)

	out, err := exec("account_balance", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Banco") || !strings.Contains(out, "15000.00") {
		t.Errorf("ARS balance missing: %s", out)
	}
	if !strings.Contains(out, "Wallet USD") || !strings.Contains(out, "-200.00") {
		t.Errorf("USD balance must show the real (negative) sign: %s", out)
	}
	if !strings.Contains(out, "ARS") || !strings.Contains(out, "USD") {
		t.Errorf("currencies must be reported separately: %s", out)
	}
}

// Case: "mi saldo en dólares" filtered to one account by name.
func TestExec_AccountBalance_ByName(t *testing.T) {
	a := &fakeQueryAccounts{accts: []account.Account{
		{Model: gorm.Model{ID: 1}, Name: "Banco", Currency: currency.ARS},
		{Model: gorm.Model{ID: 2}, Name: "Wallet USD", Currency: currency.USD},
	}}
	m := &fakeQueryMovements{balances: map[uint64]decimal.Decimal{1: dec("15000"), 2: dec("300")}}
	exec := newQueryController(m, a, &fakeQuerySubcats{}).buildQueryExecutor(1)

	out, _ := exec("account_balance", json.RawMessage(`{"account":"Wallet USD"}`))
	if strings.Contains(out, "Banco") {
		t.Errorf("account filter leaked another account: %s", out)
	}
	if !strings.Contains(out, "300.00") {
		t.Errorf("filtered balance missing: %s", out)
	}
}

// Currency defaults to ARS when the model omits it (per the ARS-if-unspecified rule).
func TestExec_SumMovements_DefaultsCurrencyARS(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "", Total: dec("100")}}}
	exec := newQueryController(m, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)

	_, err := exec("sum_movements", json.RawMessage(`{"from":"2026-05-01","to":"2026-05-31"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.lastQuery.Currency != currency.ARS {
		t.Errorf("currency default = %v, want ARS", m.lastQuery.Currency)
	}
}

func TestExec_UnknownTool(t *testing.T) {
	exec := newQueryController(&fakeQueryMovements{}, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)
	if _, err := exec("nope", json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected an error for an unknown tool")
	}
}
