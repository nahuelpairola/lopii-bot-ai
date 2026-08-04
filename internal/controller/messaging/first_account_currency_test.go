package messaging

import (
	"testing"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/subcategory"
)

// El alta lazy de la primera cuenta pregunta por UNA moneda —
// firstAccountCurrency decide cuál— pero opera sobre TODAS las filas del
// mensaje. Estos tests fijan que lo que se crea y el saldo que se le pone
// correspondan a la moneda que se preguntó, y no a la primera fila que pase.
//
// El bug que cubren no era cosmético: el saldo de apertura es un movimiento
// real, y como el balance de una cuenta es la suma de sus movimientos, una
// apertura mal calculada no se corrige nunca sola.

// openingSubRepo devuelve el repo de subcategorías mínimo que
// insertOpeningMovement necesita (Sistema | Saldo inicial).
func openingSubRepo() *fakeSubcategoryRepoFull {
	return &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		subcategory.CategorySystem + "|" + subcategory.SubOpeningBalance: newSubForTest(7,
			subcategory.CategorySystem, subcategory.SubOpeningBalance),
	}}
}

// firstAccountData arma el Data que createFirstAccount lee: las filas, el
// nombre que tipeó el usuario y el saldo que declaró.
func firstAccountData(rows []movementRow, name, balance string) conversation.Data {
	data := conversation.Data{
		conversation.UserIDKey: uint64(1),
		keyMovements:           encodeMovementRows(rows),
		keyFirstAccountName:    name,
	}
	if balance != "" {
		data[keyFirstAccountBalance] = balance
	}
	return data
}

// Un mensaje con una fila en pesos y una en dólares, de alguien que YA tiene
// cuenta en pesos. La pregunta fue por la de dólares, así que la de pesos no
// se toca: su gasto cae en la default que ya existe.
//
// Antes se creaba una segunda cuenta en pesos con el mismo nombre, vacía, y el
// gasto en pesos iba a parar ahí en vez de a la cuenta real del usuario.
func TestCreateFirstAccount_SkipsCurrenciesThatAlreadyHaveADefault(t *testing.T) {
	existing := acct(1, currency.ARS, true)
	accRepo := &fakeAccountRepoFull{
		byCurrency: map[currency.Currency]*account.Account{currency.ARS: &existing},
		byUserID:   []account.Account{existing},
	}
	movRepo := &fakeMovementRepoFull{}
	c := &controller{subcategories: openingSubRepo(), accounts: accRepo, movements: movRepo}

	rows := []movementRow{
		{Type: "expense", Amount: "5000", Currency: "ARS"},
		{Type: "income", Amount: "200", Currency: "USD"},
	}
	data := firstAccountData(rows, "Broker", "500")

	if got := firstAccountCurrency(data, hasDefaultFor(accRepo, data)); got != "USD" {
		t.Fatalf("la pregunta fue por %q, want USD", got)
	}

	idx, err := c.loadAccountIndex(1)
	if err != nil {
		t.Fatal(err)
	}
	skip, err := c.createFirstAccount(data, rows, idx)
	if err != nil {
		t.Fatalf("createFirstAccount: %v", err)
	}

	if len(accRepo.inserted) != 1 {
		t.Fatalf("cuentas creadas = %d, want 1 (solo la de dólares): %+v", len(accRepo.inserted), accRepo.inserted)
	}
	if got := accRepo.inserted[0].Currency; got != currency.USD {
		t.Errorf("moneda de la cuenta creada = %s, want USD", got)
	}
	if rows[0].AccountID != "" {
		t.Errorf("la fila en pesos quedó atada a %q; tiene que caer en la default que ya existe", rows[0].AccountID)
	}
	if rows[1].AccountID == "" {
		t.Error("la fila en dólares quedó sin cuenta")
	}

	// El saldo declarado (500) es el de la cuenta en dólares, y ya incluye el
	// ingreso de 200 de este mismo mensaje: apertura = 500 - 200. El gasto en
	// pesos NO entra en esa cuenta.
	if len(movRepo.batches) != 1 {
		t.Fatalf("aperturas insertadas = %d, want 1", len(movRepo.batches))
	}
	opening := movRepo.batches[0][0]
	if !opening.Amount.Equal(decimal.RequireFromString("300")) {
		t.Errorf("apertura = %s, want 300 (500 declarado - 200 de ingreso en USD)", opening.Amount)
	}
	if opening.Currency != currency.USD {
		t.Errorf("moneda de la apertura = %s, want USD", opening.Currency)
	}
	if skip {
		t.Error("skipBalanceCheck = true; la única cuenta creada abrió con saldo")
	}
}

// Dos filas de la MISMA moneda abren UNA cuenta, no una por fila.
func TestCreateFirstAccount_SameCurrencyTwiceCreatesOneAccount(t *testing.T) {
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{}
	c := &controller{subcategories: openingSubRepo(), accounts: accRepo, movements: movRepo}

	rows := []movementRow{
		{Type: "expense", Amount: "500", Currency: "ARS"},
		{Type: "expense", Amount: "300", Currency: "ARS"},
	}
	data := firstAccountData(rows, "Galicia", "1.000")

	idx, err := c.loadAccountIndex(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.createFirstAccount(data, rows, idx); err != nil {
		t.Fatalf("createFirstAccount: %v", err)
	}

	if len(accRepo.inserted) != 1 {
		t.Fatalf("cuentas creadas = %d, want 1: %+v", len(accRepo.inserted), accRepo.inserted)
	}
	if rows[0].AccountID != rows[1].AccountID || rows[0].AccountID == "" {
		t.Errorf("las dos filas tienen que ir a la misma cuenta: %q y %q", rows[0].AccountID, rows[1].AccountID)
	}
	// apertura = 1000 declarado - (-800 de los dos gastos) = 1800
	opening := movRepo.batches[0][0]
	if !opening.Amount.Equal(decimal.RequireFromString("1800")) {
		t.Errorf("apertura = %s, want 1800 (1000 declarado - (-800))", opening.Amount)
	}
}

// Usuario sin ninguna cuenta y un mensaje en dos monedas: se abre una por
// moneda (el default es por moneda, y sin las dos el lote no se puede
// insertar), pero el saldo declarado se aplica SOLO a la moneda por la que se
// preguntó. La otra abre en cero — nadie declaró un saldo para ella.
func TestCreateFirstAccount_ZeroAccountsMixed_BalanceOnlyToTheAskedCurrency(t *testing.T) {
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{}
	c := &controller{subcategories: openingSubRepo(), accounts: accRepo, movements: movRepo}

	rows := []movementRow{
		{Type: "expense", Amount: "5000", Currency: "ARS"},
		{Type: "income", Amount: "200", Currency: "USD"},
	}
	data := firstAccountData(rows, "Mi plata", "20.000")

	if got := firstAccountCurrency(data, hasDefaultFor(accRepo, data)); got != "ARS" {
		t.Fatalf("la pregunta fue por %q, want ARS (la primera fila sin default)", got)
	}

	idx, err := c.loadAccountIndex(1)
	if err != nil {
		t.Fatal(err)
	}
	skip, err := c.createFirstAccount(data, rows, idx)
	if err != nil {
		t.Fatalf("createFirstAccount: %v", err)
	}

	if len(accRepo.inserted) != 2 {
		t.Fatalf("cuentas creadas = %d, want 2 (una por moneda): %+v", len(accRepo.inserted), accRepo.inserted)
	}
	for _, a := range accRepo.inserted {
		if !a.IsDefault {
			t.Errorf("la cuenta %s no quedó como default de su moneda", a.Currency)
		}
	}
	if len(movRepo.batches) != 1 {
		t.Fatalf("aperturas = %d, want 1 (solo la moneda preguntada)", len(movRepo.batches))
	}
	opening := movRepo.batches[0][0]
	if opening.Currency != currency.ARS {
		t.Errorf("la apertura fue a %s; el saldo declarado era el de la cuenta en pesos", opening.Currency)
	}
	// apertura = 20000 declarado - (-5000 del gasto en pesos) = 25000.
	// El ingreso de 200 dólares NO entra: es otra cuenta y otra moneda.
	if !opening.Amount.Equal(decimal.RequireFromString("25000")) {
		t.Errorf("apertura = %s, want 25000 (20000 - (-5000)); los 200 USD no se mezclan", opening.Amount)
	}
	if !skip {
		t.Error("skipBalanceCheck = false; la cuenta en dólares abrió sin saldo y el primer gasto la deja en negativo por construcción")
	}
}

// resolveAndInsertMovements se REINTENTA sobre el mismo Data cuando el gate de
// saldo insuficiente parkea y el usuario confirma. El segundo paso no puede
// volver a crear la cuenta ni su apertura: serían una cuenta duplicada y plata
// contada dos veces.
func TestResolveAndInsertMovements_RetryDoesNotRecreateTheFirstAccount(t *testing.T) {
	subRepo := openingSubRepo()
	subRepo.byCategoryAndSub["Alimentación|Supermercado"] = newSubForTest(1, "Alimentación", "Supermercado")
	accRepo := &fakeAccountRepoFull{}
	// La cuenta nueva toma el id 100 en el fake; se le presetea el saldo
	// post-apertura para que el gate de saldo no se meta en este test.
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{100: "10500"}}
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	rows := []movementRow{
		{Type: "expense", Amount: "500", Currency: "ARS", Category: "Alimentación", Subcategory: "Supermercado", Date: "2026-07-02"},
	}
	data := conversation.Data{
		conversation.UserIDKey: uint64(1),
		keyMovements:           encodeMovementRows(rows),
		keyPendingCategoryGaps: encodeStringSlice(nil),
		keyPendingAccountGaps:  encodeStringSlice(nil),
		keyFirstAccountName:    "Galicia",
		keyFirstAccountBalance: "10.000",
	}

	if _, err := c.resolveAndInsertMovements(data); err != nil {
		t.Fatalf("primera pasada: %v", err)
	}
	if len(accRepo.inserted) != 1 {
		t.Fatalf("primera pasada creó %d cuentas, want 1", len(accRepo.inserted))
	}
	openings := len(movRepo.batches)

	// Segunda pasada sobre el MISMO data, como hace el gate al confirmar.
	setFlag(data, keySkipBalanceCheck)
	if _, err := c.resolveAndInsertMovements(data); err != nil {
		t.Fatalf("reintento: %v", err)
	}

	if len(accRepo.inserted) != 1 {
		t.Errorf("el reintento creó una cuenta de más: %d en total, want 1", len(accRepo.inserted))
	}
	// El reintento inserta los movimientos otra vez (eso lo decide el gate),
	// pero NO una segunda apertura: sería saldo inventado.
	secondOpenings := 0
	for _, batch := range movRepo.batches[openings:] {
		for _, m := range batch {
			if m.SubcategoryID == 7 { // Sistema | Saldo inicial
				secondOpenings++
			}
		}
	}
	if secondOpenings != 0 {
		t.Errorf("el reintento escribió %d aperturas de más", secondOpenings)
	}
}

// La moneda es parte del neteo: un monto en otra moneda no compensa el saldo
// declarado de ésta. Sumarlos es el bug que abría la cuenta en dólares con el
// valor de un gasto en pesos.
func TestFirstAccountNetDelta_IgnoresOtherCurrencies(t *testing.T) {
	rows := []movementRow{
		{Type: "expense", Amount: "5000", Currency: "ARS"},
		{Type: "income", Amount: "200", Currency: "USD"},
	}

	if got := firstAccountNetDelta(rows, "USD"); !got.Equal(decimal.RequireFromString("200")) {
		t.Errorf("netDelta(USD) = %s, want 200", got)
	}
	if got := firstAccountNetDelta(rows, "ARS"); !got.Equal(decimal.RequireFromString("-5000")) {
		t.Errorf("netDelta(ARS) = %s, want -5000", got)
	}
}
