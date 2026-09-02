package query

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/chathistory"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
)

type fakeQueryMovements struct {
	lastQuery   movement.MovementQuery
	lastGroupBy string
	lastLimit   int
	sumRows     []movement.CategorySum
	listRows    []movement.Movement
	balances    map[uint64]decimal.Decimal
	listCalls   int
	listByCall  [][]movement.Movement
}

func (r *fakeQueryMovements) SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error) {
	r.lastQuery, r.lastGroupBy = q, groupBy
	return r.sumRows, nil
}
func (r *fakeQueryMovements) ListForUser(q movement.MovementQuery, limit int) ([]movement.Movement, error) {
	r.lastQuery, r.lastLimit = q, limit
	r.listCalls++
	if r.listByCall != nil {
		if r.listCalls-1 < len(r.listByCall) {
			return r.listByCall[r.listCalls-1], nil
		}
		return nil, nil
	}
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
func (r *fakeQueryMovements) TopDescriptionsBySubcategory(uint64, uint64, int) ([]string, error) {
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

type queryTestServices struct {
	movements *fakeQueryMovements
	accounts  *fakeQueryAccounts
	subcats   *fakeQuerySubcats
}

func (s *queryTestServices) QueryAccountsByUserID(userID uint64) ([]account.Account, error) {
	return s.accounts.FindByUserID(userID)
}
func (s *queryTestServices) QueryCategoriesByUser(userID uint64) ([]subcategory.Subcategory, error) {
	return s.subcats.FindAllForUser(userID)
}
func (s *queryTestServices) QueryIconForCategory(userID uint64, category string) string {
	return s.subcats.IconForCategory(userID, category)
}
func (s *queryTestServices) QueryListMovements(q movement.MovementQuery, limit int) ([]movement.Movement, error) {
	return s.movements.ListForUser(q, limit)
}
func (s *queryTestServices) QuerySumMovements(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error) {
	return s.movements.SumForUser(q, groupBy)
}
func (s *queryTestServices) QueryBalanceForAccount(accountID uint64) (decimal.Decimal, error) {
	return s.movements.SumAmountForAccount(accountID)
}
func (s *queryTestServices) QueryReminderByUser(userID uint64) (*reminder.Reminder, error) {
	return nil, nil
}
func (s *queryTestServices) QueryChatRecent(userID uint64) ([]chathistory.Turn, error) {
	return nil, nil
}
func (s *queryTestServices) QueryChatAppend(userID uint64, question, answer string) error { return nil }
func (s *queryTestServices) QuerySendText(ctx context.Context, chat messenger.Chat, text string) {
}
func (s *queryTestServices) AnswerQuery(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error) {
	return "", nil
}

func newQueryExecutor(m *fakeQueryMovements, a *fakeQueryAccounts, s *fakeQuerySubcats) func(string, json.RawMessage) (string, error) {
	return NewExecutor(&queryTestServices{movements: m, accounts: a, subcats: s}, 1)
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestExec_ListCategories(t *testing.T) {
	s := &fakeQuerySubcats{subs: []subcategory.Subcategory{
		{Category: "Comida", Subcategory: "Restaurante", Description: "cuando comés afuera"},
		{Category: "Auto", Subcategory: "Nafta", Description: "combustible del auto"},
	}}
	exec := newQueryExecutor(&fakeQueryMovements{}, &fakeQueryAccounts{}, s)

	out, err := exec("list_categories", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Comida | Restaurante") || !strings.Contains(out, "Auto | Nafta") {
		t.Fatalf("la lista completa tiene que traer el mapeo categoría | subcategoría: %s", out)
	}
	if strings.Contains(out, "comés afuera") || strings.Contains(out, "combustible del auto") {
		t.Errorf("la lista SIN filtro no puede traer descripciones (son ~825 tokens de más por ronda): %s", out)
	}

	out, _ = exec("list_categories", json.RawMessage(`{"category":"Auto"}`))
	if strings.Contains(out, "Restaurante") {
		t.Errorf("category filter leaked another category: %s", out)
	}
	if !strings.Contains(out, "combustible del auto") {
		t.Errorf("la lista filtrada SÍ trae descripciones — es como el modelo desambigua: %s", out)
	}
}

func TestExec_ListCategories_LeadsWithIcon(t *testing.T) {
	s := &fakeQuerySubcats{subs: []subcategory.Subcategory{
		{Category: "Comida", Subcategory: "Restaurante", Description: "afuera", Icon: "🍔"},
	}}
	exec := newQueryExecutor(&fakeQueryMovements{}, &fakeQueryAccounts{}, s)

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
	s := &fakeQuerySubcats{}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, s)

	out, _ := exec("sum_movements", json.RawMessage(`{"from":"2026-05-01","to":"2026-05-31","currency":"ARS","group_by":"category"}`))
	if !strings.Contains(out, "📂 Comida") {
		t.Errorf("category total should lead with an icon, got: %s", out)
	}
}

func TestExec_SumMovements_FoodInMay(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "", Total: dec("5000")}}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, err := exec("sum_movements", json.RawMessage(`{"from":"2026-05-01","to":"2026-05-31","currency":"ARS","search":"Comida"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.lastQuery.Currency != currency.ARS {
		t.Errorf("currency = %v", m.lastQuery.Currency)
	}
	if m.lastQuery.Search == nil || *m.lastQuery.Search != "Comida" {
		t.Errorf("search filter not set")
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

func TestExec_SumMovements_GroupByCategory(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "Comida", Total: dec("5000")}, {Label: "Auto", Total: dec("3000")}}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, _ := exec("sum_movements", json.RawMessage(`{"from":"2026-05-01","to":"2026-05-31","currency":"ARS","group_by":"category"}`))
	if m.lastGroupBy != "category" {
		t.Errorf("groupBy = %q", m.lastGroupBy)
	}
	if !strings.Contains(out, "Comida") || !strings.Contains(out, "Auto") {
		t.Errorf("grouped rows missing: %s", out)
	}
}

func TestExec_SumMovements_GroupByAccount_MapsNames(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "7", Total: dec("1000")}}}
	a := &fakeQueryAccounts{accts: []account.Account{{Model: gorm.Model{ID: 7}, Name: "Banco", Currency: currency.ARS}}}
	exec := newQueryExecutor(m, a, &fakeQuerySubcats{})

	out, _ := exec("sum_movements", json.RawMessage(`{"from":"2026-05-01","to":"2026-05-31","currency":"ARS","group_by":"account"}`))
	if m.lastGroupBy != "account" {
		t.Errorf("groupBy = %q", m.lastGroupBy)
	}
	if !strings.Contains(out, "Banco") {
		t.Errorf("account id 7 not mapped to name: %s", out)
	}
}

func TestExec_SumMovements_GroupByMonth(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "2026-04", Total: dec("8000")}, {Label: "2026-05", Total: dec("13000")}}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, _ := exec("sum_movements", json.RawMessage(`{"from":"2026-04-01","to":"2026-05-31","currency":"ARS","group_by":"month","search":"Auto"}`))
	if m.lastGroupBy != "month" {
		t.Errorf("groupBy = %q", m.lastGroupBy)
	}
	if !strings.Contains(out, "2026-04") || !strings.Contains(out, "2026-05") {
		t.Errorf("per-month buckets missing: %s", out)
	}
}

func TestExec_SumMovements_BrokerIncome(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "", Total: dec("10000")}}}
	a := &fakeQueryAccounts{accts: []account.Account{{Model: gorm.Model{ID: 3}, Name: "Broker", Currency: currency.ARS}}}
	exec := newQueryExecutor(m, a, &fakeQuerySubcats{})

	_, err := exec("sum_movements", json.RawMessage(`{"from":"2026-01-01","to":"2026-12-31","currency":"ARS","type":"income","search":"Rendimiento inversión","account":"Broker"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.lastQuery.Type == nil || *m.lastQuery.Type != "income" {
		t.Errorf("type filter not income")
	}
	if m.lastQuery.Search == nil || *m.lastQuery.Search != "Rendimiento inversión" {
		t.Errorf("search filter not set")
	}
	if m.lastQuery.AccountID == nil || *m.lastQuery.AccountID != 3 {
		t.Errorf("account name not resolved to id 3")
	}
}

func TestExec_ListMovements_SearchFilterAbs(t *testing.T) {
	desc := "compra semanal"
	m := &fakeQueryMovements{listRows: []movement.Movement{{
		Type:        movement.Expense,
		Amount:      dec("-1500"),
		Currency:    currency.ARS,
		Date:        time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		Description: &desc,
		Subcategory: &subcategory.Subcategory{Category: "Comida", Subcategory: "Super"},
	}}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, err := exec("list_movements", json.RawMessage(`{"from":"2026-05-01","to":"2026-05-31","currency":"ARS","search":"Carrefour","limit":5}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.lastQuery.Search == nil || *m.lastQuery.Search != "Carrefour" {
		t.Errorf("search filter not set")
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

func TestExec_SumMovements_SearchStripsLeadingIcon(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "", Total: dec("5000")}}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	_, err := exec("sum_movements", json.RawMessage(`{"from":"2026-05-01","to":"2026-05-31","currency":"ARS","search":"🍔 Alimentación"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.lastQuery.Search == nil || *m.lastQuery.Search != "Alimentación" {
		t.Errorf("el ícono tiene que salir del filtro; quedó %v", m.lastQuery.Search)
	}
}

func TestExec_ListMovements_AccountAndDefaultLimit(t *testing.T) {
	m := &fakeQueryMovements{listRows: []movement.Movement{}}
	a := &fakeQueryAccounts{accts: []account.Account{{Model: gorm.Model{ID: 9}, Name: "Mercado Pago", Currency: currency.ARS}}}
	exec := newQueryExecutor(m, a, &fakeQuerySubcats{})

	_, err := exec("list_movements", json.RawMessage(`{"from":"2026-05-01","to":"2026-05-31","currency":"ARS","account":"Mercado Pago"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.lastQuery.AccountID == nil || *m.lastQuery.AccountID != 9 {
		t.Errorf("account not resolved to id 9")
	}
}

func TestExec_AccountBalance_AllSignedPerCurrency(t *testing.T) {
	a := &fakeQueryAccounts{accts: []account.Account{
		{Model: gorm.Model{ID: 1}, Name: "Banco", Currency: currency.ARS},
		{Model: gorm.Model{ID: 2}, Name: "Wallet USD", Currency: currency.USD},
	}}
	m := &fakeQueryMovements{balances: map[uint64]decimal.Decimal{1: dec("15000"), 2: dec("-200")}}
	exec := newQueryExecutor(m, a, &fakeQuerySubcats{})

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

func TestExec_AccountBalance_ByName(t *testing.T) {
	a := &fakeQueryAccounts{accts: []account.Account{
		{Model: gorm.Model{ID: 1}, Name: "Banco", Currency: currency.ARS},
		{Model: gorm.Model{ID: 2}, Name: "Wallet USD", Currency: currency.USD},
	}}
	m := &fakeQueryMovements{balances: map[uint64]decimal.Decimal{1: dec("15000"), 2: dec("300")}}
	exec := newQueryExecutor(m, a, &fakeQuerySubcats{})

	out, _ := exec("account_balance", json.RawMessage(`{"account":"Wallet USD"}`))
	if strings.Contains(out, "Banco") {
		t.Errorf("account filter leaked another account: %s", out)
	}
	if !strings.Contains(out, "300.00") {
		t.Errorf("filtered balance missing: %s", out)
	}
}

func TestExec_SumMovements_DefaultsCurrencyARS(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "", Total: dec("100")}}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	_, err := exec("sum_movements", json.RawMessage(`{"from":"2026-05-01","to":"2026-05-31"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.lastQuery.Currency != currency.ARS {
		t.Errorf("currency default = %v, want ARS", m.lastQuery.Currency)
	}
}

func TestExec_NoRows_DoesNotAssertThereWereNoExpenses(t *testing.T) {
	for _, tool := range []string{"sum_movements", "list_movements"} {
		m := &fakeQueryMovements{}
		exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

		out, err := exec(tool, json.RawMessage(`{"from":"2026-08-01","to":"2026-08-31","currency":"ARS","search":"lote"}`))
		if err == nil {
			t.Errorf("%s: un término que no existe tiene que cortar con error, no devolver un cero narrable: %s", tool, out)
			continue
		}
		if !strings.Contains(err.Error(), "lote") {
			t.Errorf("%s: el error tiene que nombrar el término: %v", tool, err)
		}
	}
}

func TestExec_UnknownAccount_FailsInsteadOfQueryingAllAccounts(t *testing.T) {
	a := &fakeQueryAccounts{accts: []account.Account{
		{Model: gorm.Model{ID: 1}, Name: "Banco", Currency: currency.ARS},
		{Model: gorm.Model{ID: 2}, Name: "Wallet", Currency: currency.USD},
	}}
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "", Total: dec("999999")}}}
	exec := newQueryExecutor(m, a, &fakeQuerySubcats{})

	out, err := exec("sum_movements", json.RawMessage(`{"from":"2026-05-01","to":"2026-05-31","currency":"ARS","account":"Galicia"}`))
	if err == nil {
		t.Fatalf("una cuenta inexistente tiene que fallar, no contestar por TODAS las cuentas; devolvió: %s", out)
	}
	if !strings.Contains(err.Error(), "Galicia") {
		t.Errorf("el error tiene que nombrar la cuenta que no encontró: %v", err)
	}
	if !strings.Contains(err.Error(), "Banco") || !strings.Contains(err.Error(), "Wallet") {
		t.Errorf("el error tiene que listar las cuentas reales para que el modelo se corrija: %v", err)
	}
	if m.lastQuery.UserID != 0 {
		t.Errorf("no puede haber consultado la base con el filtro caído: %+v", m.lastQuery)
	}
}

func TestBuildMovementQuery_StripsTheIconWeAddedFromTheFilter(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "", Total: dec("5000")}}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	_, err := exec("sum_movements", json.RawMessage(
		`{"from":"2026-07-01","to":"2026-07-31","currency":"ARS","search":"🍔 Alimentación"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.lastQuery.Search == nil || *m.lastQuery.Search != "Alimentación" {
		t.Errorf("search llegó al repo con el ícono adentro: %v", m.lastQuery.Search)
	}
}

func TestBuildMovementQuery_AnAllIconFilterStillFilters(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "", Total: dec("999999")}}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	_, err := exec("sum_movements", json.RawMessage(
		`{"from":"2026-07-01","to":"2026-07-31","currency":"ARS","search":"🍔"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.lastQuery.Search == nil {
		t.Fatal("el filtro desapareció: la consulta corrió sin filtrar y contesta por TODO")
	}
	if *m.lastQuery.Search != "🍔" {
		t.Errorf("un filtro que no se entiende se deja como vino, got %q", *m.lastQuery.Search)
	}
}

func TestBuildMovementQuery_LeavesARealNameAlone(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "", Total: dec("1")}}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	_, err := exec("sum_movements", json.RawMessage(
		`{"from":"2026-07-01","to":"2026-07-31","currency":"ARS","search":"Deudas / préstamos"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.lastQuery.Search == nil || *m.lastQuery.Search != "Deudas / préstamos" {
		t.Errorf("un nombre sin ícono tiene que llegar intacto: %v", m.lastQuery.Search)
	}
}

func oneRow() []movement.Movement {
	d := "algo"
	return []movement.Movement{{
		Type: movement.Expense, Amount: dec("-100"), Currency: currency.ARS,
		Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Description: &d,
		Subcategory: &subcategory.Subcategory{Category: "Comida", Subcategory: "Super"},
	}}
}

func TestExec_ListMovements_EmptyInRangeButExistsElsewhere(t *testing.T) {
	m := &fakeQueryMovements{listByCall: [][]movement.Movement{
		{},
		oneRow(),
	}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, err := exec("list_movements", json.RawMessage(`{"from":"2026-08-01","to":"2026-08-14","currency":"ARS","search":"netflix"}`))
	if err != nil {
		t.Fatalf("no puede ser error: el término existe, sólo que fuera del rango: %v", err)
	}
	if !strings.Contains(out, "netflix") || !strings.Contains(out, "otras fechas") {
		t.Errorf("tiene que nombrar el término y decir que hay en otras fechas: %s", out)
	}
}

func TestExec_ListMovements_EmptyButOnlyInReserved(t *testing.T) {
	m := &fakeQueryMovements{listByCall: [][]movement.Movement{
		{},
		{},
		oneRow(),
	}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, err := exec("list_movements", json.RawMessage(`{"from":"2026-08-01","to":"2026-08-14","currency":"ARS","search":"transferencia"}`))
	if err != nil {
		t.Fatalf("no puede ser error: los movimientos existen, están filtrados por reservadas: %v", err)
	}
	if !strings.Contains(out, "internos") {
		t.Errorf("tiene que explicar que sólo aparece en movimientos internos: %s", out)
	}
	if !m.lastQuery.OnlyReserved {
		t.Error("la última sonda tiene que correr con OnlyReserved = true")
	}
}

func TestExec_ListMovements_ReservedProbeFindsTransfersWhenTypeIsNil(t *testing.T) {
	m := &fakeQueryMovements{listByCall: [][]movement.Movement{
		{},
		{},
		{},
		oneRow(),
	}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, err := exec("list_movements", json.RawMessage(`{"from":"2026-08-01","to":"2026-08-31","currency":"ARS","search":"transferencia"}`))
	if err != nil {
		t.Fatalf("las transferencias reservadas existen: no puede cortar con error: %v", err)
	}
	if !strings.Contains(out, "internos") {
		t.Errorf("tiene que explicar que sólo aparece en movimientos internos: %s", out)
	}
	if m.lastQuery.Type == nil || *m.lastQuery.Type != constants.Transfer {
		t.Errorf("la última sonda tiene que forzar type=transfer; quedó %v", m.lastQuery.Type)
	}
	if !m.lastQuery.OnlyReserved {
		t.Error("la última sonda tiene que correr con OnlyReserved = true")
	}
}

func TestReinstateAppVerdict(t *testing.T) {
	verdict := fmt.Sprintf(msgSearchOnlyInternalFmt, "transferencia")

	got := reinstateAppVerdict("No se encontraron movimientos que digan transferencia en agosto.", verdict)
	if !strings.Contains(got, msgOnlyInternalMark) {
		t.Errorf("el veredicto de la app tiene que volver a la respuesta: %s", got)
	}

	yaLoDijo := "Esas transferencias son movimientos internos entre tus cuentas."
	if got := reinstateAppVerdict(yaLoDijo, verdict); got != yaLoDijo {
		t.Errorf("no puede repetir lo que el modelo ya supo decir: %s", got)
	}

	if got := reinstateAppVerdict("total: $500", ""); got != "total: $500" {
		t.Errorf("sin veredicto la respuesta va intacta: %s", got)
	}
}

func TestExec_ListMovements_SearchNotFoundAnywhere_IsError(t *testing.T) {
	m := &fakeQueryMovements{listByCall: [][]movement.Movement{{}, {}, {}}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	_, err := exec("list_movements", json.RawMessage(`{"from":"2026-08-01","to":"2026-08-14","currency":"ARS","search":"cochinchina"}`))
	if err == nil {
		t.Fatal("un término que no existe en ningún lado tiene que cortar con error, no devolver un cero interpretable")
	}
	if !strings.Contains(err.Error(), "cochinchina") {
		t.Errorf("el error tiene que nombrar el término para que el modelo lo pueda corregir: %v", err)
	}
}

func TestExec_SumMovements_EmptyWithoutSearch_NoProbes(t *testing.T) {
	m := &fakeQueryMovements{sumRows: nil}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, err := exec("sum_movements", json.RawMessage(`{"from":"2026-08-01","to":"2026-08-14","currency":"ARS"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.listCalls != 0 {
		t.Errorf("sin search no hay término que sondear; se llamó a ListForUser %d veces", m.listCalls)
	}
	if !strings.Contains(out, "sin movimientos") {
		t.Errorf("respuesta inesperada: %s", out)
	}
}

func TestExec_UnknownTool(t *testing.T) {
	exec := newQueryExecutor(&fakeQueryMovements{}, &fakeQueryAccounts{}, &fakeQuerySubcats{})
	if _, err := exec("nope", json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected an error for an unknown tool")
	}
}

func (r *fakeQueryMovements) CountByDayForUser(userID uint64, from, to time.Time) ([]movement.DayCount, error) {
	return nil, nil
}

func TestQueryMessages_NameNoTool(t *testing.T) {
	msgs := map[string]string{
		"msgQueryNoRowsInRange":    msgQueryNoRowsInRange,
		"msgSearchOutOfRangeFmt":   msgSearchOutOfRangeFmt,
		"msgSearchOnlyInternalFmt": msgSearchOnlyInternalFmt,
		"msgSearchNotFoundFmt":     msgSearchNotFoundFmt,
	}
	for name, msg := range msgs {
		for _, tool := range Tools {
			if strings.Contains(msg, tool.Name) {
				t.Errorf("%s nombra la herramienta %q; el modelo la imita y el turno se cae:\n%s", name, tool.Name, msg)
			}
		}
	}
}

func TestExec_SumMovements_GroupedCarriesTheTotal(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{
		{Label: "Supermercado", Total: dec("2031070")},
		{Label: "Almacén", Total: dec("34000")},
	}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, _ := exec("sum_movements", json.RawMessage(`{"from":"2026-07-01","to":"2026-07-31","currency":"ARS","group_by":"subcategory"}`))
	if !strings.Contains(out, "2065070.00") {
		t.Errorf("falta el total de las filas agrupadas: %s", out)
	}
}

func TestExec_SumMovements_GroupByTypeHasNoTotal(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{
		{Label: "expense", Total: dec("500000")},
		{Label: "income", Total: dec("800000")},
	}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, _ := exec("sum_movements", json.RawMessage(`{"from":"2026-07-01","to":"2026-07-31","currency":"ARS","group_by":"type"}`))
	if strings.Contains(out, "1300000") {
		t.Errorf("sumó gastos con ingresos en valor absoluto: %s", out)
	}
	if strings.Contains(strings.ToLower(out), "total") {
		t.Errorf("group_by=type no lleva línea de total: %s", out)
	}
}

func TestExec_SumMovements_SingleGroupedRowHasNoTotal(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "Supermercado", Total: dec("5000")}}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, _ := exec("sum_movements", json.RawMessage(`{"from":"2026-07-01","to":"2026-07-31","currency":"ARS","group_by":"subcategory"}`))
	if strings.Contains(strings.ToLower(out), "total") {
		t.Errorf("una sola fila no lleva total: %s", out)
	}
}

func TestAppendConsultedRange_AddsTheWindowTheAppLookedIn(t *testing.T) {
	got := appendConsultedRange("No encontré gastos en Supermercado.", "01/08/2024", "31/08/2024")
	if !strings.Contains(got, "01/08/2024") || !strings.Contains(got, "31/08/2024") {
		t.Errorf("el rango consultado no volvió a la respuesta: %q", got)
	}
	if !strings.Contains(got, "No encontré gastos en Supermercado.") {
		t.Errorf("se comió la respuesta del modelo: %q", got)
	}
}

func TestAppendConsultedRange_SkipsWhenTheAnswerAlreadyNamesIt(t *testing.T) {
	ya := "Entre 01/08/2024 y 31/08/2024 no hubo gastos en Supermercado."
	if got := appendConsultedRange(ya, "01/08/2024", "31/08/2024"); got != ya {
		t.Errorf("repitió un rango que la respuesta ya traía: %q", got)
	}
}

func TestAppendConsultedRange_SkipsWhenThereIsNoRange(t *testing.T) {
	if got := appendConsultedRange("total: $500", "", ""); got != "total: $500" {
		t.Errorf("tocó una respuesta sin rango: %q", got)
	}
}

func TestFriendlyDate_RendersArgentineFormat(t *testing.T) {
	if got := friendlyDate("2024-08-01"); got != "01/08/2024" {
		t.Errorf("friendlyDate = %q", got)
	}
	if got := friendlyDate("no es fecha"); got != "no es fecha" {
		t.Errorf("friendlyDate no respetó lo impareseable: %q", got)
	}
}

func TestEmptyResultNamesARange(t *testing.T) {
	for _, tc := range []struct {
		name string
		out  string
		want bool
	}{
		{"sin filas en el rango", msgQueryNoRowsInRange, true},
		{"el término existe fuera del rango", fmt.Sprintf(msgSearchOutOfRangeFmt, "Super", "2024-08-01", "2024-08-31"), true},
		{"sólo movimientos internos", fmt.Sprintf(msgSearchOnlyInternalFmt, "transferencia"), false},
		{"un resultado con datos", "Supermercado: 5000.00 ARS", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := emptyResultNamesARange(tc.out); got != tc.want {
				t.Errorf("emptyResultNamesARange(%q) = %v, want %v", tc.out, got, tc.want)
			}
		})
	}
}

func TestExec_SumMovements_UngroupedZeroRowIsAnEmptyResult(t *testing.T) {
	m := &fakeQueryMovements{
		sumRows:    []movement.CategorySum{{Label: "", Total: dec("0")}},
		listByCall: [][]movement.Movement{oneRow()},
	}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, err := exec("sum_movements", json.RawMessage(`{"from":"2026-06-01","to":"2026-06-30","currency":"ARS","search":"Supermercado"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "total:") {
		t.Errorf("un cero sin filas se narró como total: %q", out)
	}
	if !strings.Contains(out, msgOutOfRangeMark) {
		t.Errorf("no corrió el camino de resultado vacío: %q", out)
	}
}

func TestGroupedTotalLine_RefusesTransfers(t *testing.T) {
	rows := []movement.CategorySum{
		{Label: "43", Total: dec("1448595.59")},
		{Label: "45", Total: dec("1448595.59")},
	}
	if _, ok := groupedTotalLine(rows, movement.GroupByAccount, "ARS", constants.Transfer); ok {
		t.Error("un resultado de transferencias no puede llevar línea de total: sumaría la misma plata dos veces")
	}
	if _, ok := groupedTotalLine(rows, movement.GroupByAccount, "ARS", constants.Expense); !ok {
		t.Error("un agrupado de gastos sí lleva su total")
	}
}

func TestExec_SumMovements_TransferSplitsByDirection(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{
		{Label: "out", Total: dec("1548595.59")},
		{Label: "in", Total: dec("0")},
	}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, err := exec("sum_movements", json.RawMessage(`{"from":"2026-08-01","to":"2026-08-31","currency":"ARS","type":"transfer","group_by":"none","search":"mercado pago"}`))
	if err != nil {
		t.Fatalf("sum de transferencias: %v", err)
	}
	if m.lastGroupBy != movement.GroupByDirection {
		t.Errorf("el ejecutor tiene que forzar el agrupado por dirección, agrupó por %q", m.lastGroupBy)
	}
	if !strings.Contains(out, "1548595.59") {
		t.Errorf("falta el total que sale:\n%s", out)
	}
	if !strings.Contains(out, "salió") || !strings.Contains(out, "entró") {
		t.Errorf("las dos direcciones tienen que estar etiquetadas:\n%s", out)
	}
	if strings.Contains(out, "total") {
		t.Errorf("no puede haber línea de total: sumar las dos patas es contar la misma plata dos veces:\n%s", out)
	}
}

func TestExec_SumMovements_TransferShowsBothDirectionsEvenAtZero(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{
		{Label: "out", Total: dec("5000")},
	}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, _ := exec("sum_movements", json.RawMessage(`{"from":"2026-08-01","to":"2026-08-31","currency":"ARS","type":"transfer"}`))
	if !strings.Contains(out, "entró") || !strings.Contains(out, "0.00") {
		t.Errorf("la dirección sin filas tiene que salir en cero explícito:\n%s", out)
	}
}

func TestExec_SumMovements_TransferHonoursAnExplicitGroupBy(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{
		{Label: "43", Total: dec("1000")},
		{Label: "45", Total: dec("1000")},
	}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, _ := exec("sum_movements", json.RawMessage(`{"from":"2026-08-01","to":"2026-08-31","currency":"ARS","type":"transfer","group_by":"account"}`))
	if m.lastGroupBy != movement.GroupByAccount {
		t.Errorf("el agrupado explícito del modelo se respeta, agrupó por %q", m.lastGroupBy)
	}
	if strings.Contains(out, "total") {
		t.Errorf("ni siquiera agrupado lleva total:\n%s", out)
	}
}

func TestExec_SumMovements_NonTransferUnchanged(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Total: dec("1000")}}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, _ := exec("sum_movements", json.RawMessage(`{"from":"2026-07-01","to":"2026-07-31","currency":"ARS"}`))
	if out != "total: 1000.00 ARS" {
		t.Errorf("un sum normal tiene que salir igual que siempre: %q", out)
	}
	if m.lastGroupBy == movement.GroupByDirection {
		t.Error("sólo las transferencias se agrupan por dirección")
	}
}

func TestDaysInRange_CountsCalendarDaysAndStopsAtToday(t *testing.T) {
	utc := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	casos := []struct {
		nombre          string
		from, to, today time.Time
		want            int
	}{
		{"mes cerrado en el pasado", utc(2026, 8, 1), utc(2026, 8, 31), utc(2026, 9, 1), 31},
		{"mes en curso corta en hoy", utc(2026, 9, 1), utc(2026, 9, 30), utc(2026, 9, 3), 3},
		{"un solo dia", utc(2026, 8, 15), utc(2026, 8, 15), utc(2026, 9, 1), 1},
		{"rango enteramente futuro", utc(2026, 10, 1), utc(2026, 10, 31), utc(2026, 9, 1), 0},
		{"hoy es el primer dia del rango", utc(2026, 9, 3), utc(2026, 9, 30), utc(2026, 9, 3), 1},
	}
	for _, c := range casos {
		if got := daysInRange(c.from, c.to, c.today); got != c.want {
			t.Errorf("%s: daysInRange = %d, want %d", c.nombre, got, c.want)
		}
	}
}

func TestDaysInRange_ReadsEachDateInItsOwnZoneNotAsAnInstant(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	tardeEnArgentina := time.Date(2026, 9, 3, 23, 0, 0, 0, constants.ArgentinaZone)

	if got := daysInRange(from, to, tardeEnArgentina); got != 3 {
		t.Errorf("daysInRange = %d, want 3: el 3 de septiembre a las 23:00 ART sigue siendo el dia 3, "+
			"pero como instante ya cayo en el 4 de septiembre UTC", got)
	}
}

func TestExec_SpendingReport_GroupedCarriesTheDailyRatePerRow(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{
		{Label: "Ocio", Total: dec("310")},
		{Label: "Comida", Total: dec("620")},
	}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, err := exec("spending_report", json.RawMessage(
		`{"from":"2026-08-01","to":"2026-08-31","currency":"ARS","group_by":"category"}`))
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(out, "10.00/día") {
		t.Errorf("Ocio 310 en 31 dias son 10.00/dia: %s", out)
	}
	if !strings.Contains(out, "20.00/día") {
		t.Errorf("Comida 620 en 31 dias son 20.00/dia: %s", out)
	}
	if !strings.Contains(out, "30.00/día") {
		t.Errorf("el total (930) tambien lleva su tasa: %s", out)
	}
	if !strings.Contains(out, "31 días") {
		t.Errorf("el resultado tiene que decir sobre cuantos dias promedio: %s", out)
	}
}

func TestExec_SpendingReport_UngroupedIsASingleTotalWithItsRate(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "", Total: dec("930")}}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, err := exec("spending_report", json.RawMessage(
		`{"from":"2026-08-01","to":"2026-08-31","currency":"ARS","group_by":"none"}`))
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(out, "930.00 ARS") || !strings.Contains(out, "30.00/día") {
		t.Errorf("total con su tasa diaria: %s", out)
	}
	if strings.Contains(out, "suma de las") {
		t.Errorf("sin agrupar no hay linea de total de filas: %s", out)
	}
}

func TestExec_SpendingReport_OneRowHasNoTotalLine(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "Ocio", Total: dec("310")}}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, _ := exec("spending_report", json.RawMessage(
		`{"from":"2026-08-01","to":"2026-08-31","currency":"ARS","group_by":"category"}`))
	if strings.Contains(out, "suma de las") {
		t.Errorf("una sola fila: el total ES la fila, repetirlo son dos hechos donde hay uno: %s", out)
	}
	if !strings.Contains(out, "10.00/día") {
		t.Errorf("pero su promedio si va: %s", out)
	}
}

func TestExec_SpendingReport_EmptyGoesThroughTheSameProbesAsSum(t *testing.T) {
	m := &fakeQueryMovements{sumRows: nil}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, err := exec("spending_report", json.RawMessage(
		`{"from":"2026-08-01","to":"2026-08-31","currency":"ARS","group_by":"category"}`))
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(out, msgQueryNoRowsInRange) {
		t.Errorf("un resultado vacio sin search es el mismo mensaje que en sum_movements: %s", out)
	}
}

func TestExec_SpendingReport_RefusesTheGroupingsThatWouldProduceAFalseAverage(t *testing.T) {
	for _, args := range []string{
		`{"from":"2026-08-01","to":"2026-08-31","currency":"ARS","group_by":"month"}`,
		`{"from":"2026-08-01","to":"2026-08-31","currency":"ARS","group_by":"day"}`,
		`{"from":"2026-08-01","to":"2026-08-31","currency":"ARS","group_by":"type"}`,
		`{"from":"2026-08-01","to":"2026-08-31","currency":"ARS","type":"transfer"}`,
	} {
		m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "x", Total: dec("310")}}}
		exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})
		if _, err := exec("spending_report", json.RawMessage(args)); err == nil {
			t.Errorf("%s tendria que ser rechazado: el enum del schema es la primera linea de defensa, "+
				"pero si un modelo la esquiva el promedio que sale es FALSO, no impreciso", args)
		}
	}
}

func TestExec_SpendingReport_UngroupedZeroIsNotAMuteZero(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "", Total: dec("0")}}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, _ := exec("spending_report", json.RawMessage(
		`{"from":"2026-08-01","to":"2026-08-31","currency":"ARS","group_by":"none"}`))
	if strings.Contains(out, "0.00/día") {
		t.Errorf("un sum sin agrupar devuelve SIEMPRE una fila: el cero es ausencia, no una tasa: %s", out)
	}
	if !strings.Contains(out, msgQueryNoRowsInRange) {
		t.Errorf("tiene que caer en describeEmptyResult: %s", out)
	}
}

func TestSpendingReportSchema_ExcludesTheGroupingsThatWouldLie(t *testing.T) {
	var tool *orchestrator.AgentTool
	for i := range Tools {
		if Tools[i].Name == "spending_report" {
			tool = &Tools[i]
		}
	}
	if tool == nil {
		t.Fatal("spending_report no esta en Tools: el modelo no la puede pedir")
	}

	var schema struct {
		Properties map[string]struct {
			Enum []any `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
		t.Fatalf("el schema no parsea: %v", err)
	}

	tiene := func(campo, valor string) bool {
		for _, v := range schema.Properties[campo].Enum {
			if s, ok := v.(string); ok && s == valor {
				return true
			}
		}
		return false
	}

	for _, prohibido := range []string{"day", "month", "type"} {
		if tiene("group_by", prohibido) {
			t.Errorf("group_by=%q no puede estar en el enum: el promedio diario de esa agrupacion "+
				"es redundante o directamente falso, y el schema es lo unico que se lo impide al modelo", prohibido)
		}
	}
	for _, obligatorio := range []string{"none", "category", "subcategory", "account"} {
		if !tiene("group_by", obligatorio) {
			t.Errorf("group_by=%q falta: sin el, el modelo tiene que agrupar de otra forma y sumar a mano", obligatorio)
		}
	}
	if tiene("type", constants.Transfer) {
		t.Error("type=transfer no puede estar: las dos patas de un transfer son la misma plata, " +
			"y cualquier agregado sobre las dos es 2x")
	}
}

func TestSystemPrompt_SendsAveragesToTheToolAndKeepsSubtractionExplicit(t *testing.T) {
	p := SystemPrompt()

	if strings.Contains(p, "promediar") || strings.Contains(p, "dividí el total") {
		t.Error("el prompt no puede seguir licenciando la tasa diaria: la calcula la app, y el modelo " +
			"dividiendo a mano erro 0,05% contra el SQL el 2026-09-01")
	}
	if !strings.Contains(p, "spending_report") {
		t.Error("el prompt tiene que nombrar la tool: si no, el modelo no sabe donde pedir el promedio")
	}
	if !strings.Contains(p, "restar") {
		t.Error("comparar dos periodos SIGUE siendo del modelo, y tiene que estar dicho: una licencia " +
			"implicita no se puede auditar en la proxima medicion")
	}
	if !strings.Contains(p, "group_by=month") {
		t.Error("el promedio MENSUAL sigue siendo del modelo y necesita su camino: spending_report " +
			"excluye group_by=month a proposito, asi que sin esta linea la pregunta se queda sin ninguno")
	}
}
