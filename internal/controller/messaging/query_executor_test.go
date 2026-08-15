package messaging

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/constants"
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

func newQueryController(m *fakeQueryMovements, a *fakeQueryAccounts, s *fakeQuerySubcats) *controller {
	return &controller{movements: m, accounts: a, subcategories: s}
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
	exec := newQueryController(&fakeQueryMovements{}, &fakeQueryAccounts{}, s).buildQueryExecutor(1)

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
	exec := newQueryController(m, a, &fakeQuerySubcats{}).buildQueryExecutor(1)

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
	exec := newQueryController(m, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)

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
	exec := newQueryController(m, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)

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
		exec := newQueryController(m, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)

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
	exec := newQueryController(m, a, &fakeQuerySubcats{}).buildQueryExecutor(1)

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
	exec := newQueryController(m, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)

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
	exec := newQueryController(m, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)

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
	exec := newQueryController(m, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)

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
	exec := newQueryController(m, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)

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
	exec := newQueryController(m, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)

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
	exec := newQueryController(m, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)

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
	exec := newQueryController(m, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)

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
	exec := newQueryController(m, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)

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
	exec := newQueryController(&fakeQueryMovements{}, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)
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
		for _, tool := range queryTools {
			if strings.Contains(msg, tool.Name) {
				t.Errorf("%s nombra la herramienta %q; el modelo la imita y el turno se cae:\n%s", name, tool.Name, msg)
			}
		}
	}
}
