package messaging

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)

func uint64Ptr(v uint64) *uint64 { return &v }

type fakeSubcategoryRepoFull struct {
	byCategoryAndSub map[string]*subcategory.Subcategory
	all              []subcategory.Subcategory
	categories       []string
}

func (r *fakeSubcategoryRepoFull) FindByCategoryAndSubcategory(category, sub string) (*subcategory.Subcategory, error) {
	s, ok := r.byCategoryAndSub[category+"|"+sub]
	if !ok {
		return nil, subcategory.ErrSubcategoryNotFound
	}
	return s, nil
}
func (r *fakeSubcategoryRepoFull) FindAllForUser(userID uint64) ([]subcategory.Subcategory, error) {
	return r.all, nil
}
func (r *fakeSubcategoryRepoFull) DistinctCategoriesForUser(userID uint64) ([]string, error) {
	return r.categories, nil
}

type fakeAccountRepoFull struct {
	byCurrency map[currency.Currency]*account.Account
	byUserID   []account.Account
	inserted   []account.Account
	balances   map[uint64]string
}

func (r *fakeAccountRepoFull) Insert(a *account.Account) error {
	a.ID = uint(len(r.inserted) + 100)
	r.inserted = append(r.inserted, *a)
	return nil
}
func (r *fakeAccountRepoFull) FindDefaultByCurrency(userID uint64, c currency.Currency) (*account.Account, error) {
	a, ok := r.byCurrency[c]
	if !ok {
		return nil, errors.New("not found")
	}
	return a, nil
}
func (r *fakeAccountRepoFull) FindByUserID(userID uint64) ([]account.Account, error) {
	return r.byUserID, nil
}

type fakeMovementRepoFull struct {
	inserted       []movement.Movement
	balances       map[uint64]string
	replacedOldIDs []uint
	replaced       []movement.Movement
}

func (r *fakeMovementRepoFull) InsertBatch(ms []movement.Movement) error {
	r.inserted = ms
	return nil
}
func (r *fakeMovementRepoFull) SumAmountForAccount(accountID uint64) (decimal.Decimal, error) {
	s, ok := r.balances[accountID]
	if !ok {
		return decimal.Zero, nil
	}
	d, _ := decimal.NewFromString(s)
	return d, nil
}
func (r *fakeMovementRepoFull) ReplaceMovements(oldIDs []uint, newMovements []movement.Movement) error {
	r.replacedOldIDs = oldIDs
	r.replaced = newMovements
	return nil
}

func newSubForTest(id uint, category, sub string) *subcategory.Subcategory {
	s := &subcategory.Subcategory{Category: category, Subcategory: sub}
	s.ID = id
	return s
}

func TestResolveAndInsertMovements_SimpleSingleMovement_NilTransactionID(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Alimentación|Café": newSubForTest(1, "Alimentación", "Café"),
	}}
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{}

	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	result := orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", PaymentMethod: "cash", Description: "Café", Date: "2026-07-02"},
	}}
	data := buildCreateSeed(result)
	data[conversation.UserIDKey] = uint64(1)

	inserted, err := c.resolveAndInsertMovements(data)
	if err != nil {
		t.Fatalf("resolveAndInsertMovements: %v", err)
	}
	if len(movRepo.inserted) != 1 {
		t.Fatalf("got %d inserted, want 1", len(movRepo.inserted))
	}
	if movRepo.inserted[0].TransactionID != nil {
		t.Error("a single movement should have a nil TransactionID")
	}
	if len(inserted) != 1 {
		t.Errorf("resolveAndInsertMovements should return the inserted rows")
	}
}

func TestResolveAndInsertMovements_Compound_SharesTransactionID(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Inversiones|Compra USD": newSubForTest(2, "Inversiones", "Compra USD"),
	}}
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{}
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	result := orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "expense", Amount: "140000", Currency: "ARS", Category: "Inversiones", Subcategory: "Compra USD", PaymentMethod: "transfer", Description: "Compra USD", Date: "2026-07-02"},
		{Type: "transfer", Amount: "100", Currency: "USD", Category: "Inversiones", Subcategory: "Compra USD", PaymentMethod: "transfer", Description: "Compra USD", Date: "2026-07-02", AccountID: uint64Ptr(7)},
	}}
	data := buildCreateSeed(result)
	data[conversation.UserIDKey] = uint64(1)

	if _, err := c.resolveAndInsertMovements(data); err != nil {
		t.Fatalf("resolveAndInsertMovements: %v", err)
	}
	if len(movRepo.inserted) != 2 {
		t.Fatalf("got %d inserted, want 2", len(movRepo.inserted))
	}
	if movRepo.inserted[0].TransactionID == nil || movRepo.inserted[1].TransactionID == nil {
		t.Fatal("both rows of a compound transaction should have a TransactionID")
	}
	if *movRepo.inserted[0].TransactionID != *movRepo.inserted[1].TransactionID {
		t.Error("both rows should share the same TransactionID")
	}
}

func TestResolveAndInsertMovements_PendingAccountCreation(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Inversiones|FCI": newSubForTest(3, "Inversiones", "FCI"),
	}}
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{}
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	rows := []movementRow{
		{Type: "transfer", Amount: "120000", Currency: "ARS", AccountID: accountPendingCreate, AccountNameGuess: "FCI", Category: "Inversiones", Subcategory: "FCI", Date: "2026-07-02"},
	}
	data := conversation.Data{
		conversation.UserIDKey:  uint64(1),
		"mode":                  "create",
		"old_movement_ids":      encodeStringSlice(nil),
		"movements":             encodeMovementRows(rows),
		"pending_category_gaps": encodeStringSlice(nil),
		"pending_account_gaps":  encodeStringSlice(nil),
	}

	if _, err := c.resolveAndInsertMovements(data); err != nil {
		t.Fatalf("resolveAndInsertMovements: %v", err)
	}
	if len(accRepo.inserted) != 1 {
		t.Fatalf("expected 1 account created, got %d", len(accRepo.inserted))
	}
	if accRepo.inserted[0].Name != "FCI" {
		t.Errorf("created account name = %q, want FCI", accRepo.inserted[0].Name)
	}
	if movRepo.inserted[0].AccountID == nil {
		t.Error("the movement should reference the newly created account")
	}
}

func TestResolveAndInsertMovements_FCIRedemption_GainAboveBalance(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Inversiones|FCI":               newSubForTest(3, "Inversiones", "FCI"),
		"Sistema|Rendimiento inversión": newSubForTest(9, "Sistema", "Rendimiento inversión"),
	}}
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{7: "80000"}} // fund has 80000 in it
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	rows := []movementRow{
		{Type: "transfer", Amount: "-100000", Currency: "ARS", AccountID: "7", Category: "Inversiones", Subcategory: "FCI", Date: "2026-07-02"},
		{Type: "transfer", Amount: "100000", Currency: "ARS", AccountID: "10", Category: "Inversiones", Subcategory: "FCI", Date: "2026-07-02"},
	}
	data := conversation.Data{
		conversation.UserIDKey:  uint64(1),
		"mode":                  "create",
		"old_movement_ids":      encodeStringSlice(nil),
		"movements":             encodeMovementRows(rows),
		"pending_category_gaps": encodeStringSlice(nil),
		"pending_account_gaps":  encodeStringSlice(nil),
	}

	inserted, err := c.resolveAndInsertMovements(data)
	if err != nil {
		t.Fatalf("resolveAndInsertMovements: %v", err)
	}
	if len(inserted) != 3 {
		t.Fatalf("expected 2 transfer legs + 1 gain income movement, got %d", len(inserted))
	}
	gain := inserted[2]
	if gain.Type != movement.Income {
		t.Errorf("third movement type = %v, want income", gain.Type)
	}
	wantGain, _ := decimal.NewFromString("20000")
	if !gain.Amount.Equal(wantGain) {
		t.Errorf("gain amount = %v, want 20000 (100000 redeemed - 80000 balance)", gain.Amount)
	}
}

func TestResolveAndInsertMovements_FCIRedemption_PartialNoGain(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Inversiones|FCI": newSubForTest(3, "Inversiones", "FCI"),
	}}
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{7: "500000"}} // much more than being withdrawn
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	rows := []movementRow{
		{Type: "transfer", Amount: "-100000", Currency: "ARS", AccountID: "7", Category: "Inversiones", Subcategory: "FCI", Date: "2026-07-02"},
		{Type: "transfer", Amount: "100000", Currency: "ARS", AccountID: "10", Category: "Inversiones", Subcategory: "FCI", Date: "2026-07-02"},
	}
	data := conversation.Data{
		conversation.UserIDKey:  uint64(1),
		"mode":                  "create",
		"old_movement_ids":      encodeStringSlice(nil),
		"movements":             encodeMovementRows(rows),
		"pending_category_gaps": encodeStringSlice(nil),
		"pending_account_gaps":  encodeStringSlice(nil),
	}

	inserted, err := c.resolveAndInsertMovements(data)
	if err != nil {
		t.Fatalf("resolveAndInsertMovements: %v", err)
	}
	if len(inserted) != 2 {
		t.Fatalf("a partial redemption should insert only the 2 transfer legs, got %d", len(inserted))
	}
}

func TestResolveAndInsertMovements_UpdateMode_CallsReplaceMovements(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Alimentación|Café": newSubForTest(1, "Alimentación", "Café"),
	}}
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{}
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	rows := []movementRow{
		{Type: "expense", Amount: "3500", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", Description: "Café", Date: "2026-07-02"},
	}
	data := conversation.Data{
		conversation.UserIDKey:  uint64(1),
		"mode":                  "update",
		"old_movement_ids":      encodeStringSlice([]string{"42"}),
		"movements":             encodeMovementRows(rows),
		"pending_category_gaps": encodeStringSlice(nil),
		"pending_account_gaps":  encodeStringSlice(nil),
	}

	if _, err := c.resolveAndInsertMovements(data); err != nil {
		t.Fatalf("resolveAndInsertMovements: %v", err)
	}
	if movRepo.inserted != nil {
		t.Error("update mode should never call InsertBatch")
	}
	if len(movRepo.replacedOldIDs) != 1 || movRepo.replacedOldIDs[0] != 42 {
		t.Errorf("replacedOldIDs = %v, want [42]", movRepo.replacedOldIDs)
	}
	if len(movRepo.replaced) != 1 {
		t.Fatalf("expected 1 replaced movement, got %d", len(movRepo.replaced))
	}
}
