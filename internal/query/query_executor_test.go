package query

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/chathistory"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
)

// --- fakes scoped to the query-executor use-case tests ---

type fakeQueryMovements struct {
	lastQuery   movement.MovementQuery
	lastGroupBy string
	lastLimit   int
	// OJO con la forma que le ponés a sumRows: SIN agrupar el repo devuelve
	// SIEMPRE una fila (COALESCE(SUM(...),0) sin GROUP BY), nunca nil. Un fake
	// que devuelve nil para ese caso prueba una forma que no existe — y así fue
	// como el cero mudo de execSumMovements sobrevivió a toda la suite.
	sumRows  []movement.CategorySum
	listRows []movement.Movement
	balances map[uint64]decimal.Decimal
	// listCalls/listByCall existen para el camino del resultado vacío, que hace
	// hasta dos sondas ADEMÁS de la consulta real: sin poder devolver algo
	// distinto por llamada no se puede distinguir "no hay en el rango" de "no
	// existe en ningún lado".
	listCalls  int
	listByCall [][]movement.Movement
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

// queryTestServices adapta los fakes de repo a la interfaz query.services.
// Solo los métodos que los tests ejercitan delegan; el resto son stubs.
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
func (s *queryTestServices) QuerySendText(ctx context.Context, b *bot.Bot, chatID int64, text string) {
}
func (s *queryTestServices) AnswerQuery(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error) {
	return "", nil
}

func newQueryExecutor(m *fakeQueryMovements, a *fakeQueryAccounts, s *fakeQuerySubcats) func(string, json.RawMessage) (string, error) {
	return NewExecutor(&queryTestServices{movements: m, accounts: a, subcats: s}, 1)
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// Case: "qué categorías hay y para qué sirve cada una"
//
// Las descripciones viajan SOLO en la lista filtrada. Medido el 2026-08-10 con la
// taxonomía real (66 filas): con descripciones son ~1.325 tokens, sin ellas ~500. El
// resultado de una tool se reinyecta en cada ronda posterior, así que la diferencia
// se paga dos o tres veces por consulta contra un TPM de 8.000 — una consulta
// multi-entidad se pasaba del techo justo por eso, y el 429 resultante mandaba el
// mensaje a la cola, que reintentaba y volvía a pasarse. Para contestar alcanza el
// mapeo nombre → categoría | subcategoría.
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
	s := &fakeQuerySubcats{} // IconForCategory returns "📂"
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, s)

	out, _ := exec("sum_movements", json.RawMessage(`{"from":"2026-05-01","to":"2026-05-31","currency":"ARS","group_by":"category"}`))
	if !strings.Contains(out, "📂 Comida") {
		t.Errorf("category total should lead with an icon, got: %s", out)
	}
}

// Case: "cuánto gasté en comida en mayo" — range + category + currency,
// transfer excluded by default (Type nil).
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

// Case: "resumen del mes por categoría"
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

// Case: "gastos por cuenta" — group_by=account maps account_id label to name.
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

// Cases: "promedio mensual del auto" / "5k más que el mes pasado" / "20k por
// día" — the data side. The loop composes these from per-month totals.
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

// Case: "cuánto me rindió el broker" — income + Sistema|Rendimiento inversión
// + account resolution to an id.
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

// Case: "mis compras en Carrefour" — search filter + abs rendering.
func TestExec_ListMovements_SearchFilterAbs(t *testing.T) {
	desc := "compra semanal"
	m := &fakeQueryMovements{listRows: []movement.Movement{{
		Type:        movement.Expense,
		Amount:      dec("-1500"), // stored signed; must render abs
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

// El prompt le pide al modelo arrancar la línea con el emoji y list_categories
// devuelve "🍔 Alimentación". El modelo aprende ese string y lo copia al filtro.
// unaccent no borra emojis: sin stripLeadingIcon el LIKE no matchea nada y la
// respuesta sale $0 sobre gastos que existen.
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

// Case: "mis últimos movimientos de Mercado Pago" (account) + default limit.
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

// Cases: "mi saldo en cada cuenta" / "cuánto tengo en total" — signed balance,
// per currency, never mixed.
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

// Case: "mi saldo en dólares" filtered to one account by name.
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

// Currency defaults to ARS when the model omits it (per the ARS-if-unspecified rule).
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

// Regresión del 2026-08-13: ante `list_movements(category="lote")` —donde "lote" no es
// una categoría sino una palabra en la descripción de tres gastos de Vivienda— el
// ejecutor devolvía "Sin movimientos en ese rango." y el modelo narró "No tenés
// registros de gastos en la categoría Lote. El total gastado es $0 ARS". Eran $30.343,74.
//
// Cero filas y cero gastos son hechos distintos. Desde el 2026-08-14 el ejecutor
// ya no se limita a AVISARLE al modelo que no los confunda: cuando las sondas
// confirman que el término no aparece en ningún lado, corta con error, y un
// error no se puede narrar como "$0". Vale para las DOS tools.
func TestExec_NoRows_DoesNotAssertThereWereNoExpenses(t *testing.T) {
	for _, tool := range []string{"sum_movements", "list_movements"} {
		m := &fakeQueryMovements{} // sin filas: ni la consulta ni las sondas encuentran
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

// buildMovementQuery resolvía el nombre de cuenta a un id y, si no matcheaba ninguna,
// dejaba AccountID en nil y corría la consulta SIN filtrar por cuenta: preguntás por
// una cuenta y te contestan por todas, sin ninguna señal.
//
// Es el peor de los tres defectos del 2026-08-13 porque devuelve un número GRANDE y
// plausible, mientras los otros dos devuelven cero o un total chico que llaman la
// atención. Y no tenía cobertura: los dos tests que pasan `account`
// (BrokerIncome, AccountAndDefaultLimit) usan nombres que sí existen en su fixture.
//
// No es una convención nueva: execAccountBalance, dos funciones más abajo, ya devuelve
// "No encontré esa cuenta." ante lo mismo. De las tres tools que aceptan `account`,
// una avisaba y dos se lo tragaban.
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
	// El error vuelve al modelo como texto (el loop no aborta), así que tiene que
	// alcanzar para corregir el nombre solo.
	if !strings.Contains(err.Error(), "Galicia") {
		t.Errorf("el error tiene que nombrar la cuenta que no encontró: %v", err)
	}
	if !strings.Contains(err.Error(), "Banco") || !strings.Contains(err.Error(), "Wallet") {
		t.Errorf("el error tiene que listar las cuentas reales para que el modelo se corrija: %v", err)
	}
	// Lo que de verdad importa: no llegó a consultar.
	if m.lastQuery.UserID != 0 {
		t.Errorf("no puede haber consultado la base con el filtro caído: %+v", m.lastQuery)
	}
}

// El emoji que ponemos nosotros volvía adentro del filtro. list_categories y
// sum_movements(group_by=category) anteponen el ícono al nombre —"🍔 Alimentación"—
// porque el prompt le pide al modelo que arranque la línea con él. El modelo, que
// aprende el nombre de ahí, lo copia entero al filtro siguiente, el SQL no matchea
// nada y la respuesta es "$0".
//
// Encontrado por el eval el 2026-08-13: preguntando por tres subcategorías con
// 5.000, 3.000 y 8.000 cargados, contestó "$0 / sin registros / $0". Es el mismo
// daño que una cuenta inexistente, pero con el agravante de que el dato corrupto lo
// generamos nosotros.
//
// Se limpia en buildMovementQuery, que es por donde pasan los dos filtros de
// taxonomía de las dos tools, en vez de en cada productor de íconos.
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

// Un filtro que queda VACÍO al sacarle el ícono no puede desaparecer: tiene que
// seguir filtrando, aunque no matchee nada.
//
// Bug introducido el 2026-08-13 por el fix del emoji: stripLeadingIcon devuelve ""
// cuando el string no tiene ninguna letra ni dígito, y el llamador trataba ese ""
// como "no vino filtro" — o sea la consulta pasaba a correr SIN filtrar y contestaba
// por todo. Es exactamente el defecto que este mismo commit venía a arreglar para
// `account`, reintroducido un campo más allá.
//
// La regla es que un filtro nunca se ENSANCHA en silencio: si no se entiende, se
// deja como vino y la consulta devuelve cero, que el modelo sí sabe explicar.
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

// Un nombre real no se toca: hay categorías con espacios y barras ("Deudas /
// préstamos") y no puede recortarse nada de eso.
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

// oneRow es una fila cualquiera, para que una sonda "encuentre algo".
func oneRow() []movement.Movement {
	d := "algo"
	return []movement.Movement{{
		Type: movement.Expense, Amount: dec("-100"), Currency: currency.ARS,
		Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Description: &d,
		Subcategory: &subcategory.Subcategory{Category: "Comida", Subcategory: "Super"},
	}}
}

// Cero en el rango pero SÍ en otras fechas: es ausencia verificada, y decirlo
// así es verdadero. Antes esto y "el término no existe" eran el mismo cero.
func TestExec_ListMovements_EmptyInRangeButExistsElsewhere(t *testing.T) {
	m := &fakeQueryMovements{listByCall: [][]movement.Movement{
		{},       // la consulta real: vacía
		oneRow(), // sonda 1, rango ensanchado: hay
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

// El caso que rompía el arreglo: "transferencia" matchea 12 movimientos reales
// que apply() esconde por ser de una categoría reservada. Sin la sonda 2, la app
// afirmaría que no existe. Es peor que el cero mudo de antes.
func TestExec_ListMovements_EmptyButOnlyInReserved(t *testing.T) {
	m := &fakeQueryMovements{listByCall: [][]movement.Movement{
		{},       // la consulta real
		{},       // sonda 1, rango ensanchado, sin reservadas: nada
		oneRow(), // sonda 2, sólo reservadas: hay
	}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, err := exec("list_movements", json.RawMessage(`{"from":"2026-08-01","to":"2026-08-14","currency":"ARS","search":"transferencia"}`))
	if err != nil {
		t.Fatalf("no puede ser error: los movimientos existen, están filtrados por reservadas: %v", err)
	}
	if !strings.Contains(out, "internos") {
		t.Errorf("tiene que explicar que sólo aparece en movimientos internos: %s", out)
	}
	// La sonda 2 es la que mira las reservadas: sin esto el test pasaría aunque
	// alguien la escribiera sin activar el flag.
	if !m.lastQuery.OnlyReserved {
		t.Error("la última sonda tiene que correr con OnlyReserved = true")
	}
}

// El agujero de la sonda 2, encontrado contra el bot el 2026-08-14: hereda el Type de
// la consulta original, y con Type nil apply agrega `type <> transfer`, que esconde
// justo las reservadas que más importan — Sistema | Transferencia y los saldos
// iniciales son TODAS transferencias.
//
// Medido contra la base ese día: la sonda con type=transfer encuentra 12 filas y la
// misma sonda con Type nil encuentra 0, así que esos 12 movimientos se declaraban
// inexistentes con un error duro.
func TestExec_ListMovements_ReservedProbeFindsTransfersWhenTypeIsNil(t *testing.T) {
	m := &fakeQueryMovements{listByCall: [][]movement.Movement{
		{},       // la consulta real
		{},       // sonda 1, rango ensanchado
		{},       // sonda 2 heredando Type nil: no ve las transferencias
		oneRow(), // sonda 2 forzando type=transfer: ahí están
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

// El modelo puede INVERTIR el veredicto de la app. El 2026-08-14 el ejecutor entregó
// "«transferencia» sólo aparece en movimientos internos…" —12 filas reales detrás— y el
// usuario leyó "No se encontraron movimientos que digan transferencia". La app vuelve a
// pegar lo suyo cuando eso pasa.
func TestReinstateAppVerdict(t *testing.T) {
	verdict := fmt.Sprintf(msgSearchOnlyInternalFmt, "transferencia")

	got := reinstateAppVerdict("No se encontraron movimientos que digan transferencia en agosto.", verdict)
	if !strings.Contains(got, msgOnlyInternalMark) {
		t.Errorf("el veredicto de la app tiene que volver a la respuesta: %s", got)
	}

	// Si el modelo ya lo dijo, no se repite.
	yaLoDijo := "Esas transferencias son movimientos internos entre tus cuentas."
	if got := reinstateAppVerdict(yaLoDijo, verdict); got != yaLoDijo {
		t.Errorf("no puede repetir lo que el modelo ya supo decir: %s", got)
	}

	// Sin veredicto de la app no se toca nada.
	if got := reinstateAppVerdict("total: $500", ""); got != "total: $500" {
		t.Errorf("sin veredicto la respuesta va intacta: %s", got)
	}
}

// Las tres consultas en cero: el término no existe en ningún lado. ESTE es el
// que cierra el portón — el modelo no tiene con qué afirmar ausencia.
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

// Una consulta SIN search que da cero es un cero honesto y sin ambigüedad: no
// hubo movimientos en ese rango. No corresponde sondear nada.
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

// Ningún mensaje que le llegue al MODELO puede nombrar una herramienta.
//
// La narración forzada corre con tool_choice:"none" y sin schemas: ahí el modelo imita
// cualquier cosa que se parezca a una tool. Medido el 2026-08-13 con el texto que
// nombraba list_categories: gpt-oss-20b devolvió HTTP 400 "Tool choice is none, but
// model called a tool", y gpt-oss-120b —que es el queryModel de producción— le imprimió
// al usuario el texto {"tool": "list_categories", "params": {}}.
//
// Se escribe contra la lista REAL de tools y no contra un string: así también atrapa a
// quien mañana meta sum_movements en un mensaje. Y contra los CUATRO mensajes, no uno:
// desde el 2026-08-14 el vacío tiene cuatro salidas, y cada una es una puerta nueva
// para el mismo mecanismo.
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

// El bug: el 2026-08-14, contra la base real, el modelo recibió dos filas
// agrupadas —Supermercado 2.031.070 y Almacén 34.000— y contestó 2.031.070.
// Leyó la primera y tiró la segunda. La casa ya tiene la regla de que la
// aritmética es de la app y nunca del modelo; acá no se estaba aplicando.
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

// group_by=type es la excepción, y no es un detalle: las filas llegan en valor
// absoluto (CategorySum.Total es SUM(ABS(amount))), así que sumar el renglón de
// gastos con el de ingresos da un número que no es ni el gasto, ni el ingreso,
// ni el neto. La app no puede escribir eso, porque toda la línea existe para
// que el modelo la cite en vez de sumar él.
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

// Una sola fila agrupada no lleva total: el total ES la fila, y repetirlo le
// hace creer al modelo que hay dos hechos donde hay uno.
func TestExec_SumMovements_SingleGroupedRowHasNoTotal(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "Supermercado", Total: dec("5000")}}}
	exec := newQueryExecutor(m, &fakeQueryAccounts{}, &fakeQuerySubcats{})

	out, _ := exec("sum_movements", json.RawMessage(`{"from":"2026-07-01","to":"2026-07-31","currency":"ARS","group_by":"subcategory"}`))
	if strings.Contains(strings.ToLower(out), "total") {
		t.Errorf("una sola fila no lleva total: %s", out)
	}
}

// El bug: una consulta vacía por resolver mal el año es INVISIBLE. El ejecutor
// dice "sin movimientos con «Supermercado» entre 2024-08-01 y 2024-08-31", el
// modelo redacta "no gastaste en Supermercado" y el rango —el único dato que
// delata el error— no llega nunca al usuario. Reponerlo no previene la
// resolución equivocada: la hace visible en el acto.
func TestAppendConsultedRange_AddsTheWindowTheAppLookedIn(t *testing.T) {
	got := appendConsultedRange("No encontré gastos en Supermercado.", "01/08/2024", "31/08/2024")
	if !strings.Contains(got, "01/08/2024") || !strings.Contains(got, "31/08/2024") {
		t.Errorf("el rango consultado no volvió a la respuesta: %q", got)
	}
	if !strings.Contains(got, "No encontré gastos en Supermercado.") {
		t.Errorf("se comió la respuesta del modelo: %q", got)
	}
}

// Si el modelo sí nombró el rango, repetirlo es ruido.
func TestAppendConsultedRange_SkipsWhenTheAnswerAlreadyNamesIt(t *testing.T) {
	ya := "Entre 01/08/2024 y 31/08/2024 no hubo gastos en Supermercado."
	if got := appendConsultedRange(ya, "01/08/2024", "31/08/2024"); got != ya {
		t.Errorf("repitió un rango que la respuesta ya traía: %q", got)
	}
}

// Sin rango capturado no se toca la respuesta: la mayoría de las consultas no
// terminan vacías y no tienen nada que reponer.
func TestAppendConsultedRange_SkipsWhenThereIsNoRange(t *testing.T) {
	if got := appendConsultedRange("total: $500", "", ""); got != "total: $500" {
		t.Errorf("tocó una respuesta sin rango: %q", got)
	}
}

// Las fechas se reponen en formato argentino, no ISO. El prompt le prohíbe al
// modelo mostrar 2026-07-01, así que la app tampoco puede colarlo por atrás.
func TestFriendlyDate_RendersArgentineFormat(t *testing.T) {
	if got := friendlyDate("2024-08-01"); got != "01/08/2024" {
		t.Errorf("friendlyDate = %q", got)
	}
	// Lo que no parsea vuelve tal cual: el rango es informativo y nunca vale
	// romper una respuesta que ya está lista por una fecha rara.
	if got := friendlyDate("no es fecha"); got != "no es fecha" {
		t.Errorf("friendlyDate no respetó lo impareseable: %q", got)
	}
}

// Qué resultados vacíos merecen que se reponga el rango. Los dos que sí son los
// que hablan de una VENTANA: la app miró un período concreto y no encontró nada,
// y ahí el período es el dato sospechoso. Los otros dos desenlaces no: "sólo
// aparece en movimientos internos" y "no encontré nada que diga X" son hechos
// sobre el término, verdaderos en cualquier rango.
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

// LA FORMA QUE DEVUELVE EL REPO DE VERDAD. Un sum SIN agrupar hace
// `COALESCE(SUM(ABS(amount)), 0)` sin GROUP BY, y eso en SQL devuelve SIEMPRE
// exactamente una fila, con total 0. Nunca cero filas.
//
// Por eso describeEmptyResult era inalcanzable desde el camino más común
// —"¿cuánto gasté en X?"— y ese camino seguía devolviendo el cero mudo que los
// cuatro mensajes venían a matar. Medido contra el bot el 2026-08-18: preguntar
// por Supermercado en junio devolvió "total: 0.00 ARS" y el modelo narró "no hay
// registros", sin sonda, sin veredicto y sin rango.
//
// El fake tenía la culpa de que nadie lo viera: devolvía nil, una forma que el
// repo no produce jamás, así que el test de al lado pasaba con producción rota.
func TestExec_SumMovements_UngroupedZeroRowIsAnEmptyResult(t *testing.T) {
	m := &fakeQueryMovements{
		sumRows:    []movement.CategorySum{{Label: "", Total: dec("0")}},
		listByCall: [][]movement.Movement{oneRow()}, // la sonda 1 sí encuentra
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

// Un transfer son dos patas de la misma plata. Con group_by=account las filas
// por cuenta sí significan algo, pero el total entre ellas es 2×. La línea de
// total existe justamente porque el modelo la cita sin revisarla, así que acá
// no puede existir.
func TestGroupedTotalLine_RefusesTransfers(t *testing.T) {
	rows := []movement.CategorySum{
		{Label: "43", Total: dec("1448595.59")},
		{Label: "45", Total: dec("1448595.59")},
	}
	if _, ok := groupedTotalLine(rows, movement.GroupByAccount, "ARS", constants.Transfer); ok {
		t.Error("un resultado de transferencias no puede llevar línea de total: sumaría la misma plata dos veces")
	}
	// El caso normal no cambia.
	if _, ok := groupedTotalLine(rows, movement.GroupByAccount, "ARS", constants.Expense); !ok {
		t.Error("un agrupado de gastos sí lleva su total")
	}
}

// La consulta real que rompió en producción el 2026-08-21. Los montos son los
// de la base: 8 patas salientes de FCI hacia Mercado Pago en agosto.
//
// Antes contestaba $100.000 —el único movimiento que zafaba del filtro, y
// encima mal categorizado— sobre $1.548.595,59 reales.
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

// Una dirección sin filas sale en cero EXPLÍCITO y no se omite. Una ausencia es
// un hecho: si la línea no está, el modelo no sabe si se consultó y dio cero o
// si nadie la consultó, y ya contestó "no hay" sobre plata real por eso.
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

// El modelo pidió un agrupado: se respeta. Pisárselo con la dirección sería
// contestarle otra pregunta. Lo que lo mantiene seguro es que tampoco lleva
// total (ver TestGroupedTotalLine_RefusesTransfers).
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

// El camino común no cambia de forma.
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
