package messaging

import (
	"testing"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

func openingSubRepo() *fakeSubcategoryRepoFull {
	return &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		subcategory.CategorySystem + "|" + subcategory.SubOpeningBalance: newSubForTest(7,
			subcategory.CategorySystem, subcategory.SubOpeningBalance),
	}}
}

func firstAccountData(rows []movement.MovementRow, name, balance string) conversation.Data {
	data := conversation.Data{
		conversation.UserIDKey:           uint64(1),
		conversation.KeyMovements:        movement.EncodeMovementRows(rows),
		conversation.KeyFirstAccountName: name,
	}
	if balance != "" {
		data[conversation.KeyFirstAccountBalance] = balance
	}
	return data
}

func TestCreateFirstAccount_SkipsCurrenciesThatAlreadyHaveADefault(t *testing.T) {
	existing := acct(1, currency.ARS, true)
	accRepo := &fakeAccountRepoFull{
		byCurrency: map[currency.Currency]*account.Account{currency.ARS: &existing},
		byUserID:   []account.Account{existing},
	}
	movRepo := &fakeMovementRepoFull{}
	c := &controller{subcategories: openingSubRepo(), accounts: accRepo, movements: movRepo}

	rows := []movement.MovementRow{
		{Type: "expense", Amount: "5000", Currency: "ARS"},
		{Type: "income", Amount: "200", Currency: "USD"},
	}
	data := firstAccountData(rows, "Broker", "500")

	if got := flow.FirstAccountCurrency(data, flow.HasDefaultFor(accRepo, data)); got != "USD" {
		t.Fatalf("la pregunta fue por %q, want USD", got)
	}

	idx, err := flow.LoadAccountIndex(c, 1)
	if err != nil {
		t.Fatal(err)
	}
	skip, err := flow.CreateFirstAccount(c, data, rows, idx)
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

func TestCreateFirstAccount_SameCurrencyTwiceCreatesOneAccount(t *testing.T) {
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{}
	c := &controller{subcategories: openingSubRepo(), accounts: accRepo, movements: movRepo}

	rows := []movement.MovementRow{
		{Type: "expense", Amount: "500", Currency: "ARS"},
		{Type: "expense", Amount: "300", Currency: "ARS"},
	}
	data := firstAccountData(rows, "Galicia", "1.000")

	idx, err := flow.LoadAccountIndex(c, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := flow.CreateFirstAccount(c, data, rows, idx); err != nil {
		t.Fatalf("createFirstAccount: %v", err)
	}

	if len(accRepo.inserted) != 1 {
		t.Fatalf("cuentas creadas = %d, want 1: %+v", len(accRepo.inserted), accRepo.inserted)
	}
	if rows[0].AccountID != rows[1].AccountID || rows[0].AccountID == "" {
		t.Errorf("las dos filas tienen que ir a la misma cuenta: %q y %q", rows[0].AccountID, rows[1].AccountID)
	}
	opening := movRepo.batches[0][0]
	if !opening.Amount.Equal(decimal.RequireFromString("1800")) {
		t.Errorf("apertura = %s, want 1800 (1000 declarado - (-800))", opening.Amount)
	}
}

func TestCreateFirstAccount_ZeroAccountsMixed_BalanceOnlyToTheAskedCurrency(t *testing.T) {
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{}
	c := &controller{subcategories: openingSubRepo(), accounts: accRepo, movements: movRepo}

	rows := []movement.MovementRow{
		{Type: "expense", Amount: "5000", Currency: "ARS"},
		{Type: "income", Amount: "200", Currency: "USD"},
	}
	data := firstAccountData(rows, "Mi plata", "20.000")

	if got := flow.FirstAccountCurrency(data, flow.HasDefaultFor(accRepo, data)); got != "ARS" {
		t.Fatalf("la pregunta fue por %q, want ARS (la primera fila sin default)", got)
	}

	idx, err := flow.LoadAccountIndex(c, 1)
	if err != nil {
		t.Fatal(err)
	}
	skip, err := flow.CreateFirstAccount(c, data, rows, idx)
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
	if !opening.Amount.Equal(decimal.RequireFromString("25000")) {
		t.Errorf("apertura = %s, want 25000 (20000 - (-5000)); los 200 USD no se mezclan", opening.Amount)
	}
	if !skip {
		t.Error("skipBalanceCheck = false; la cuenta en dólares abrió sin saldo y el primer gasto la deja en negativo por construcción")
	}
}

func TestResolveAndInsertMovements_RetryDoesNotRecreateTheFirstAccount(t *testing.T) {
	subRepo := openingSubRepo()
	subRepo.byCategoryAndSub["Alimentación|Supermercado"] = newSubForTest(1, "Alimentación", "Supermercado")
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{100: "10500"}}
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	rows := []movement.MovementRow{
		{Type: "expense", Amount: "500", Currency: "ARS", Category: "Alimentación", Subcategory: "Supermercado", Date: "2026-07-02"},
	}
	data := conversation.Data{
		conversation.UserIDKey:              uint64(1),
		conversation.KeyMovements:           movement.EncodeMovementRows(rows),
		conversation.KeyPendingCategoryGaps: conversation.EncodeStringSlice(nil),
		conversation.KeyPendingAccountGaps:  conversation.EncodeStringSlice(nil),
		conversation.KeyFirstAccountName:    "Galicia",
		conversation.KeyFirstAccountBalance: "10.000",
	}

	if _, err := flow.ResolveAndInsertMovements(c, data); err != nil {
		t.Fatalf("primera pasada: %v", err)
	}
	if len(accRepo.inserted) != 1 {
		t.Fatalf("primera pasada creó %d cuentas, want 1", len(accRepo.inserted))
	}
	openings := len(movRepo.batches)

	conversation.SetFlag(data, conversation.KeySkipBalanceCheck)
	if _, err := flow.ResolveAndInsertMovements(c, data); err != nil {
		t.Fatalf("reintento: %v", err)
	}

	if len(accRepo.inserted) != 1 {
		t.Errorf("el reintento creó una cuenta de más: %d en total, want 1", len(accRepo.inserted))
	}
	secondOpenings := 0
	for _, batch := range movRepo.batches[openings:] {
		for _, m := range batch {
			if m.SubcategoryID == 7 {
				secondOpenings++
			}
		}
	}
	if secondOpenings != 0 {
		t.Errorf("el reintento escribió %d aperturas de más", secondOpenings)
	}
}

func TestFirstAccountNetDelta_IgnoresOtherCurrencies(t *testing.T) {
	rows := []movement.MovementRow{
		{Type: "expense", Amount: "5000", Currency: "ARS"},
		{Type: "income", Amount: "200", Currency: "USD"},
	}

	if got := flow.FirstAccountNetDelta(rows, "USD"); !got.Equal(decimal.RequireFromString("200")) {
		t.Errorf("netDelta(USD) = %s, want 200", got)
	}
	if got := flow.FirstAccountNetDelta(rows, "ARS"); !got.Equal(decimal.RequireFromString("-5000")) {
		t.Errorf("netDelta(ARS) = %s, want -5000", got)
	}
}
