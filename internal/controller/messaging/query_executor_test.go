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

// Case: "mis compras en Carrefour" — description filter + abs rendering.
func TestExec_ListMovements_DescriptionFilterAbs(t *testing.T) {
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

	out, err := exec("list_movements", json.RawMessage(`{"from":"2026-05-01","to":"2026-05-31","currency":"ARS","description":"Carrefour","limit":5}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.lastQuery.Description == nil || *m.lastQuery.Description != "Carrefour" {
		t.Errorf("description filter not set")
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

// Regresión del 2026-08-13: ante `list_movements(category="lote")` —donde "lote" no es
// una categoría sino una palabra en la descripción de tres gastos de Vivienda— el
// ejecutor devolvía "Sin movimientos en ese rango." y el modelo narró "No tenés
// registros de gastos en la categoría Lote. El total gastado es $0 ARS". Eran $30.343,74.
//
// Cero filas y cero gastos son hechos distintos, y el mensaje viejo no los distinguía.
// El nuevo no valida el filtro: le avisa al modelo que un nombre mal escrito también da
// cero y le nombra la tool con la que puede verificarlo.
func TestExec_NoRows_DoesNotAssertThereWereNoExpenses(t *testing.T) {
	m := &fakeQueryMovements{} // sin filas: ni sumRows ni listRows
	exec := newQueryController(m, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)

	for _, tool := range []string{"sum_movements", "list_movements"} {
		out, err := exec(tool, json.RawMessage(`{"from":"2026-08-01","to":"2026-08-31","currency":"ARS","category":"lote"}`))
		if err != nil {
			t.Fatalf("%s: %v", tool, err)
		}
		if !strings.Contains(out, "no existe") {
			t.Errorf("%s: el vacío tiene que avisar que un nombre inexistente también da cero, got: %s", tool, out)
		}
		// "Sin movimientos" afirma el hecho que justamente no se sabe.
		if strings.Contains(out, "Sin movimientos") {
			t.Errorf("%s: el vacío no puede afirmar que no hubo movimientos, got: %s", tool, out)
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
		`{"from":"2026-07-01","to":"2026-07-31","currency":"ARS","category":"🍔 Alimentación","subcategory":"🧘 Gimnasio"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.lastQuery.Category == nil || *m.lastQuery.Category != "Alimentación" {
		t.Errorf("category llegó al repo con el ícono adentro: %v", m.lastQuery.Category)
	}
	if m.lastQuery.Subcategory == nil || *m.lastQuery.Subcategory != "Gimnasio" {
		t.Errorf("subcategory llegó al repo con el ícono adentro: %v", m.lastQuery.Subcategory)
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
		`{"from":"2026-07-01","to":"2026-07-31","currency":"ARS","category":"🍔"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.lastQuery.Category == nil {
		t.Fatal("el filtro desapareció: la consulta corrió sin filtrar y contesta por TODO")
	}
	if *m.lastQuery.Category != "🍔" {
		t.Errorf("un filtro que no se entiende se deja como vino, got %q", *m.lastQuery.Category)
	}
}

// Un nombre real no se toca: hay categorías con espacios y barras ("Deudas /
// préstamos") y no puede recortarse nada de eso.
func TestBuildMovementQuery_LeavesARealNameAlone(t *testing.T) {
	m := &fakeQueryMovements{sumRows: []movement.CategorySum{{Label: "", Total: dec("1")}}}
	exec := newQueryController(m, &fakeQueryAccounts{}, &fakeQuerySubcats{}).buildQueryExecutor(1)

	_, err := exec("sum_movements", json.RawMessage(
		`{"from":"2026-07-01","to":"2026-07-31","currency":"ARS","category":"Deudas / préstamos"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.lastQuery.Category == nil || *m.lastQuery.Category != "Deudas / préstamos" {
		t.Errorf("un nombre sin ícono tiene que llegar intacto: %v", m.lastQuery.Category)
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

// El texto de un resultado vacío NO puede nombrar una herramienta.
//
// La narración forzada corre con tool_choice:"none" y sin schemas: ahí el modelo imita
// cualquier cosa que se parezca a una tool. Medido el 2026-08-13 con el texto que
// nombraba list_categories: gpt-oss-20b devolvió HTTP 400 "Tool choice is none, but
// model called a tool", y gpt-oss-120b —que es el queryModel de producción— le imprimió
// al usuario el texto {"tool": "list_categories", "params": {}}.
//
// Es la tercera vez en el día que el mismo mecanismo muerde por una puerta distinta, y
// por eso el test se escribe contra la lista REAL de tools y no contra un string: así
// también atrapa a quien mañana meta sum_movements en un mensaje.
func TestMsgQueryNoRows_NamesNoTool(t *testing.T) {
	for _, tool := range queryTools {
		if strings.Contains(msgQueryNoRows, tool.Name) {
			t.Errorf("msgQueryNoRows nombra la herramienta %q; el modelo la imita y el turno se cae:\n%s",
				tool.Name, msgQueryNoRows)
		}
	}
	// La distinción que el mensaje SÍ tiene que conservar: cero filas no prueba cero
	// gastos. Sin esto el modelo vuelve a afirmar "no tenés gastos" sobre un filtro
	// mal escrito, que es el bug original del 2026-08-13.
	if !strings.Contains(msgQueryNoRows, "no existe") {
		t.Errorf("el mensaje tiene que explicar que un nombre inexistente también da cero:\n%s", msgQueryNoRows)
	}
}
