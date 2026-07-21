package messaging

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
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
	allErr           error
	categories       []string
	owned            []subcategory.Subcategory
	ownedErr         error
	deletedUserID    uint64
	deletedID        uint64
	deleteCalls      int
	deleteErr        error
}

func (r *fakeSubcategoryRepoFull) FindByCategoryAndSubcategory(userID uint64, category, sub string) (*subcategory.Subcategory, error) {
	s, ok := r.byCategoryAndSub[category+"|"+sub]
	if !ok {
		return nil, subcategory.ErrSubcategoryNotFound
	}
	return s, nil
}
func (r *fakeSubcategoryRepoFull) FindAllForUser(userID uint64) ([]subcategory.Subcategory, error) {
	return r.all, r.allErr
}
func (r *fakeSubcategoryRepoFull) DistinctCategoriesForUser(userID uint64) ([]string, error) {
	return r.categories, nil
}
func (r *fakeSubcategoryRepoFull) IconForCategory(userID uint64, category string) string {
	return "📂"
}
func (r *fakeSubcategoryRepoFull) Insert(s *subcategory.Subcategory) error { return nil }
func (r *fakeSubcategoryRepoFull) Reload() error                           { return nil }
func (r *fakeSubcategoryRepoFull) Delete(userID uint64, id uint64) error {
	r.deletedUserID, r.deletedID = userID, id
	r.deleteCalls++
	return r.deleteErr
}
func (r *fakeSubcategoryRepoFull) FindOwnedByUser(userID uint64) ([]subcategory.Subcategory, error) {
	return r.owned, r.ownedErr
}

type fakeAccountRepoFull struct {
	byCurrency   map[currency.Currency]*account.Account
	byUserID     []account.Account
	byUserIDErr  error
	byID         map[uint64]*account.Account
	inserted     []account.Account
	balances     map[uint64]string
	insertErr    error
	renamedID    uint64
	renamedName  string
	renameErr    error
	unsetCalls   []currency.Currency
	setDefaultID uint64
}

func (r *fakeAccountRepoFull) Insert(a *account.Account) error {
	if r.insertErr != nil {
		return r.insertErr
	}
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
func (r *fakeAccountRepoFull) HasDefaultForCurrency(userID uint64, c currency.Currency) bool {
	if _, ok := r.byCurrency[c]; ok {
		return true
	}
	for _, a := range r.byUserID {
		if a.Currency == c && a.IsDefault {
			return true
		}
	}
	return false
}
func (r *fakeAccountRepoFull) FindByUserID(userID uint64) ([]account.Account, error) {
	return r.byUserID, r.byUserIDErr
}
func (r *fakeAccountRepoFull) GetAccount(id uint64) (*account.Account, error) {
	a, ok := r.byID[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return a, nil
}
func (r *fakeAccountRepoFull) Rename(accountID uint64, name string) error {
	if r.renameErr != nil {
		return r.renameErr
	}
	r.renamedID, r.renamedName = accountID, name
	return nil
}
func (r *fakeAccountRepoFull) UnsetDefault(userID uint64, cur currency.Currency) error {
	r.unsetCalls = append(r.unsetCalls, cur)
	delete(r.byCurrency, cur) // mirror the real repo: after unset, no default of this currency
	return nil
}
func (r *fakeAccountRepoFull) SetDefault(accountID uint64) error {
	r.setDefaultID = accountID
	return nil
}

type fakeMovementRepoFull struct {
	inserted           []movement.Movement
	batches            [][]movement.Movement // every InsertBatch call, in order (inserted only tracks the last)
	balances           map[uint64]string
	replacedOldIDs     []uint
	replaced           []movement.Movement
	deletedIDs         []uint
	similar            []movement.Movement
	similarErr         error
	insertErr          error
	openings           []movement.AccountOpening
	reassignFrom       uint64
	reassignTo         uint64
	reassignCalls      int
	countForUser       int64
	countBySubcategory int64
	countErr           error
	reassignedFrom     uint64
	reassignedTo       uint64
	reassignedUser     uint64
	reassignSubCalls   int
	reassignSubErr     error
	topMerchants       []string
}

func (r *fakeMovementRepoFull) InsertBatch(ms []movement.Movement) error {
	if r.insertErr != nil {
		return r.insertErr
	}
	r.inserted = ms
	r.batches = append(r.batches, ms)
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
func (r *fakeMovementRepoFull) FindSimilarForUser(userID uint64, query string, since time.Time, until *time.Time) ([]movement.Movement, error) {
	return r.similar, nil
}
func (r *fakeMovementRepoFull) FindRecentlyCreatedForUser(userID uint64, since time.Time) ([]movement.Movement, error) {
	return r.similar, r.similarErr
}
func (r *fakeMovementRepoFull) SoftDeleteByIDs(ids []uint) error {
	r.deletedIDs = ids
	return nil
}
func (r *fakeMovementRepoFull) InsertAccountsWithOpenings(items []movement.AccountOpening) error {
	r.openings = items
	return nil
}
func (r *fakeMovementRepoFull) SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error) {
	return nil, nil
}
func (r *fakeMovementRepoFull) ListForUser(q movement.MovementQuery, limit int) ([]movement.Movement, error) {
	return nil, nil
}
func (r *fakeMovementRepoFull) ReassignAccount(fromID, toID uint64) error {
	r.reassignFrom, r.reassignTo = fromID, toID
	r.reassignCalls++
	return nil
}
func (r *fakeMovementRepoFull) CountForUser(userID uint64) (int64, error) {
	return r.countForUser, nil
}
func (r *fakeMovementRepoFull) CountBySubcategory(userID uint64, subcategoryID uint64) (int64, error) {
	return r.countBySubcategory, r.countErr
}
func (r *fakeMovementRepoFull) ReassignSubcategory(userID uint64, fromID uint64, toID uint64) error {
	r.reassignedFrom, r.reassignedTo, r.reassignedUser = fromID, toID, userID
	r.reassignSubCalls++
	return r.reassignSubErr
}
func (r *fakeMovementRepoFull) TopMerchantsBySubcategory(userID uint64, subcategoryID uint64, limit int) ([]string, error) {
	return r.topMerchants, nil
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
	accRepo := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{1: "1000000"}}

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
	accRepo := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true), acct(7, currency.USD, false)}}
	movRepo := &fakeMovementRepoFull{}
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	result := orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "transfer", Amount: "140000", Currency: "ARS", Category: "Inversiones", Subcategory: "Compra USD", PaymentMethod: "transfer", Description: "Compra USD", Date: "2026-07-02", AccountID: uint64Ptr(1), Group: "g1"},
		{Type: "transfer", Amount: "100", Currency: "USD", Category: "Inversiones", Subcategory: "Compra USD", PaymentMethod: "transfer", Description: "Compra USD", Date: "2026-07-02", AccountID: uint64Ptr(7), Group: "g1"},
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
	accRepo := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{1: "1000000"}}
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	rows := []movementRow{
		{Type: "transfer", Amount: "-120000", Currency: "ARS", AccountID: "1", Group: "g1", Category: "Inversiones", Subcategory: "FCI", Date: "2026-07-02"},
		{Type: "transfer", Amount: "120000", Currency: "ARS", AccountID: accountPendingCreate, AccountNameGuess: "FCI", Group: "g1", Category: "Inversiones", Subcategory: "FCI", Date: "2026-07-02"},
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
	if movRepo.inserted[1].AccountID == nil {
		t.Error("the movement should reference the newly created account")
	}
}

func TestResolveAndInsertMovements_FirstAccount_WithBalance(t *testing.T) {
	sub := newSubForTest(1, "Alimentación", "Supermercado")
	openingSub := newSubForTest(9, "Sistema", "Saldo inicial")
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Alimentación|Supermercado": sub,
		"Sistema|Saldo inicial":     openingSub,
	}}
	accRepo := &fakeAccountRepoFull{}
	// fakeAccountRepoFull.Insert assigns the first created account id 100;
	// preset its post-opening balance so the insufficient-funds gate sees it.
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{100: "99500"}}
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	rows := []movementRow{
		{Type: "expense", Amount: "500", Currency: "ARS", Category: "Alimentación", Subcategory: "Supermercado", Date: "2026-07-02"},
	}
	data := conversation.Data{
		conversation.UserIDKey:  uint64(1),
		"movements":             encodeMovementRows(rows),
		"pending_category_gaps": encodeStringSlice(nil),
		"pending_account_gaps":  encodeStringSlice(nil),
		keyFirstAccountName:     "Galicia",
		keyFirstAccountBalance:  "99.500,00",
	}

	inserted, err := c.resolveAndInsertMovements(data)
	if err != nil {
		t.Fatalf("resolveAndInsertMovements: %v", err)
	}
	if len(movRepo.batches) != 2 {
		t.Fatalf("expected 2 InsertBatch calls (opening + movement), got %d", len(movRepo.batches))
	}
	opening := movRepo.batches[0]
	if len(opening) != 1 || !opening[0].Amount.Equal(decimal.RequireFromString("99500")) {
		t.Fatalf("opening batch = %+v, want a single 99500 movement", opening)
	}
	if opening[0].SubcategoryID != uint64(openingSub.ID) {
		t.Errorf("opening subcategory id = %d, want %d (Sistema|Saldo inicial)", opening[0].SubcategoryID, openingSub.ID)
	}
	if len(inserted) != 1 || inserted[0].AccountID == nil {
		t.Fatalf("expected the expense to reference the new account, got %+v", inserted)
	}
}

func TestResolveAndInsertMovements_FirstAccount_SkipBalance(t *testing.T) {
	sub := newSubForTest(1, "Alimentación", "Supermercado")
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Alimentación|Supermercado": sub,
	}}
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{} // no balance preset: fresh account starts at 0
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	rows := []movementRow{
		{Type: "expense", Amount: "500", Currency: "ARS", Category: "Alimentación", Subcategory: "Supermercado", Date: "2026-07-02"},
	}
	data := conversation.Data{
		conversation.UserIDKey:  uint64(1),
		"movements":             encodeMovementRows(rows),
		"pending_category_gaps": encodeStringSlice(nil),
		"pending_account_gaps":  encodeStringSlice(nil),
		keyFirstAccountName:     "Galicia",
		// keyFirstAccountBalance left unset — the user answered "después".
	}

	inserted, err := c.resolveAndInsertMovements(data)
	if err != nil {
		t.Fatalf("resolveAndInsertMovements: %v (the insufficient-funds gate must be skipped)", err)
	}
	if len(movRepo.batches) != 1 {
		t.Fatalf("expected no opening batch, got %d InsertBatch calls", len(movRepo.batches))
	}
	if len(inserted) != 1 || inserted[0].AccountID == nil {
		t.Fatalf("expected the expense to reference the new account, got %+v", inserted)
	}
}

func TestResolveAndInsertMovements_FCIRedemption_GainAboveBalance(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Inversiones|FCI":               newSubForTest(3, "Inversiones", "FCI"),
		"Sistema|Rendimiento inversión": newSubForTest(9, "Sistema", "Rendimiento inversión"),
	}}
	accRepo := &fakeAccountRepoFull{
		byID: map[uint64]*account.Account{
			7: {IsDefault: false}, // dedicated FCI account, not the everyday wallet — a redemption candidate
		},
		byUserID: []account.Account{acct(7, currency.ARS, false), acct(10, currency.ARS, false)},
	}
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{7: "80000"}} // fund has 80000 in it
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	rows := []movementRow{
		{Type: "transfer", Amount: "-100000", Currency: "ARS", AccountID: "7", Group: "g1", Category: "Inversiones", Subcategory: "FCI", Date: "2026-07-02"},
		{Type: "transfer", Amount: "100000", Currency: "ARS", AccountID: "10", Group: "g1", Category: "Inversiones", Subcategory: "FCI", Date: "2026-07-02"},
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

func TestResolveAndInsertMovements_FCISubscription_DefaultAccount_NoGain(t *testing.T) {
	// Regression test: a subscription's negative leg has the exact same
	// shape as a redemption's (Transfer, negative amount, Inversiones|FCI)
	// — the old logic would have computed a bogus gain here (120000
	// "redeemed" against an 80000 balance). Marking the account as the
	// user's default (everyday wallet) must suppress the gain entirely.
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Inversiones|FCI":               newSubForTest(3, "Inversiones", "FCI"),
		"Sistema|Rendimiento inversión": newSubForTest(9, "Sistema", "Rendimiento inversión"),
	}}
	accRepo := &fakeAccountRepoFull{
		byID: map[uint64]*account.Account{
			7: {IsDefault: true}, // the everyday wallet — this is a subscription, not a redemption
		},
		byUserID: []account.Account{acct(7, currency.ARS, true), acct(10, currency.ARS, false)},
	}
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{7: "80000"}} // would trigger a false gain under the old logic
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	rows := []movementRow{
		{Type: "transfer", Amount: "-120000", Currency: "ARS", AccountID: "7", Group: "g1", Category: "Inversiones", Subcategory: "FCI", Date: "2026-07-02"},
		{Type: "transfer", Amount: "120000", Currency: "ARS", AccountID: "10", Group: "g1", Category: "Inversiones", Subcategory: "FCI", Date: "2026-07-02"},
	}
	data := conversation.Data{
		conversation.UserIDKey:  uint64(1),
		"mode":                  "create",
		"old_movement_ids":      encodeStringSlice(nil),
		"movements":             encodeMovementRows(rows),
		"pending_category_gaps": encodeStringSlice(nil),
		"pending_account_gaps":  encodeStringSlice(nil),
		"_skip_balance_check":   "true", // out of scope here: this test is about gain suppression, not the insufficient-funds gate
	}

	inserted, err := c.resolveAndInsertMovements(data)
	if err != nil {
		t.Fatalf("resolveAndInsertMovements: %v", err)
	}
	if len(inserted) != 2 {
		t.Fatalf("a subscription from the default account must never produce a gain leg, got %d movements", len(inserted))
	}
}

func TestResolveAndInsertMovements_FCIRedemption_PartialNoGain(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Inversiones|FCI": newSubForTest(3, "Inversiones", "FCI"),
	}}
	accRepo := &fakeAccountRepoFull{
		byID: map[uint64]*account.Account{
			7: {IsDefault: false},
		},
		byUserID: []account.Account{acct(7, currency.ARS, false), acct(10, currency.ARS, false)},
	}
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{7: "500000"}} // much more than being withdrawn
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	rows := []movementRow{
		{Type: "transfer", Amount: "-100000", Currency: "ARS", AccountID: "7", Group: "g1", Category: "Inversiones", Subcategory: "FCI", Date: "2026-07-02"},
		{Type: "transfer", Amount: "100000", Currency: "ARS", AccountID: "10", Group: "g1", Category: "Inversiones", Subcategory: "FCI", Date: "2026-07-02"},
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

func TestResolveAndInsertMovements_InvalidDate_ReturnsError(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Alimentación|Café": newSubForTest(1, "Alimentación", "Café"),
	}}
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{}
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	rows := []movementRow{
		{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", Date: "not-a-date"},
	}
	data := conversation.Data{
		conversation.UserIDKey:  uint64(1),
		"mode":                  "create",
		"old_movement_ids":      encodeStringSlice(nil),
		"movements":             encodeMovementRows(rows),
		"pending_category_gaps": encodeStringSlice(nil),
		"pending_account_gaps":  encodeStringSlice(nil),
	}

	if _, err := c.resolveAndInsertMovements(data); err == nil {
		t.Fatal("expected an error for a malformed date, not a silent fallback to time.Now()")
	}
	if movRepo.inserted != nil {
		t.Error("a malformed date should prevent any insert")
	}
}

func TestResolveAndInsertMovements_FCIRedemption_MissingGainSubcategory_ReturnsError(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Inversiones|FCI": newSubForTest(3, "Inversiones", "FCI"),
		// "Sistema|Rendimiento inversión" deliberately absent — a
		// misconfigured/missing reserved subcategory should surface as
		// an error, never be swallowed into "no gain".
	}}
	accRepo := &fakeAccountRepoFull{byID: map[uint64]*account.Account{
		7: {IsDefault: false},
	}}
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{7: "80000"}}
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

	if _, err := c.resolveAndInsertMovements(data); err == nil {
		t.Fatal("a missing 'Sistema|Rendimiento inversión' subcategory is a config problem and must surface as an error, not be silently swallowed")
	}
}

func TestResolveAndInsertMovements_UpdateMode_CallsReplaceMovements(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Alimentación|Café": newSubForTest(1, "Alimentación", "Café"),
	}}
	accRepo := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}
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

func TestResolveAndInsertMovements_PopulatesSubcategoryAssociation(t *testing.T) {
	sub := newSubForTest(1, "Alimentación", "Café")
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Alimentación|Café": sub,
	}}
	accRepo := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{1: "1000000"}}
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	result := orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", PaymentMethod: "cash", Description: "Café", Date: "2026-07-02"},
	}}
	data := buildCreateSeed(result)
	data[conversation.UserIDKey] = uint64(1)

	if _, err := c.resolveAndInsertMovements(data); err != nil {
		t.Fatalf("resolveAndInsertMovements: %v", err)
	}
	if movRepo.inserted[0].Subcategory == nil || movRepo.inserted[0].Subcategory.Category != "Alimentación" {
		t.Errorf("inserted movement's Subcategory = %+v, want Category=Alimentación", movRepo.inserted[0].Subcategory)
	}
}

func TestResolveAndInsertMovements_FCIRedemption_GainLegHasSubcategory(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Inversiones|FCI":               newSubForTest(3, "Inversiones", "FCI"),
		"Sistema|Rendimiento inversión": newSubForTest(9, "Sistema", "Rendimiento inversión"),
	}}
	accRepo := &fakeAccountRepoFull{
		byID: map[uint64]*account.Account{
			7: {IsDefault: false},
		},
		byUserID: []account.Account{acct(7, currency.ARS, false), acct(10, currency.ARS, false)},
	}
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{7: "80000"}}
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	rows := []movementRow{
		{Type: "transfer", Amount: "-100000", Currency: "ARS", AccountID: "7", Group: "g1", Category: "Inversiones", Subcategory: "FCI", Date: "2026-07-02"},
		{Type: "transfer", Amount: "100000", Currency: "ARS", AccountID: "10", Group: "g1", Category: "Inversiones", Subcategory: "FCI", Date: "2026-07-02"},
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
	gain := inserted[2]
	if gain.Subcategory == nil || gain.Subcategory.Subcategory != "Rendimiento inversión" {
		t.Errorf("gain leg's Subcategory = %+v, want Subcategory=Rendimiento inversión", gain.Subcategory)
	}
}

func TestResolveAndInsertMovements_CreatesBothPendingAccounts(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Inversiones|FCI": newSubForTest(3, "Inversiones", "FCI"),
	}}
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{}
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	rows := []movementRow{
		{Type: "transfer", Amount: "-50000", Currency: "ARS", AccountID: accountPendingCreate, AccountNameGuess: "Banco", Group: "g1", Category: "Inversiones", Subcategory: "FCI", Date: "2026-07-07"},
		{Type: "transfer", Amount: "50000", Currency: "ARS", AccountID: accountPendingCreate, AccountNameGuess: "Mercado Pago", Group: "g1", Category: "Inversiones", Subcategory: "FCI", Date: "2026-07-07"},
	}
	data := conversation.Data{
		conversation.UserIDKey:  uint64(1),
		"mode":                  "create",
		"old_movement_ids":      encodeStringSlice(nil),
		"movements":             encodeMovementRows(rows),
		"pending_category_gaps": encodeStringSlice(nil),
		"pending_account_gaps":  encodeStringSlice([]string{"0", "1"}),
		"_skip_balance_check":   "true", // brand-new accounts have no meaningful prior balance to test against
	}

	inserted, err := c.resolveAndInsertMovements(data)
	if err != nil {
		t.Fatalf("resolveAndInsertMovements: %v", err)
	}
	if len(accRepo.inserted) != 2 {
		t.Errorf("accounts created = %d, want 2 (Banco + Mercado Pago)", len(accRepo.inserted))
	}
	if len(inserted) != 2 {
		t.Fatalf("inserted %d movements, want 2", len(inserted))
	}
	if inserted[0].TransactionID == nil || inserted[1].TransactionID == nil || *inserted[0].TransactionID != *inserted[1].TransactionID {
		t.Errorf("both legs must share one transaction_id")
	}
	for i, m := range inserted {
		if m.AccountID == nil {
			t.Errorf("transfer leg %d has nil account_id", i)
		}
	}
}

func TestFciRedemptionGain_AttributedToFciAccount(t *testing.T) {
	fciAccID := uint64(9)
	subs := &fakeSubcategoryRepoFull{
		byCategoryAndSub: map[string]*subcategory.Subcategory{
			"Inversiones|FCI":               newSubForTest(3, "Inversiones", "FCI"),
			"Sistema|Rendimiento inversión": newSubForTest(4, "Sistema", "Rendimiento inversión"),
		},
	}
	accts := &fakeAccountRepoFull{byID: map[uint64]*account.Account{
		fciAccID: {Model: gorm.Model{ID: uint(fciAccID)}, IsDefault: false, Currency: currency.ARS},
	}}
	movs := &fakeMovementRepoFull{balances: map[uint64]string{fciAccID: "100000"}}
	c := &controller{subcategories: subs, accounts: accts, movements: movs}

	redemption := []movement.Movement{{
		Type:          movement.Transfer,
		AccountID:     &fciAccID,
		SubcategoryID: 3,
		Amount:        decimal.NewFromInt(-120000),
		Currency:      currency.ARS,
		UserID:        1,
	}}
	gain, ok, err := fciRedemptionGain(c, redemption)
	if err != nil || !ok {
		t.Fatalf("expected a gain, got ok=%v err=%v", ok, err)
	}
	if gain.AccountID == nil || *gain.AccountID != fciAccID {
		t.Errorf("gain account = %v, want FCI account %d", gain.AccountID, fciAccID)
	}
	if !gain.Amount.Equal(decimal.NewFromInt(20000)) {
		t.Errorf("gain = %s, want 20000", gain.Amount)
	}
}

func TestResolveAndInsert_IndependentExpensesNotGrouped(t *testing.T) {
	subs := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Alimentos|Supermercado": newSubForTest(5, "Alimentos", "Supermercado"),
	}}
	accts := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}
	movs := &fakeMovementRepoFull{balances: map[uint64]string{1: "1000000"}}
	c := &controller{subcategories: subs, accounts: accts, movements: movs}

	rows := []movementRow{
		{Type: "expense", Amount: "500", Currency: "ARS", Category: "Alimentos", Subcategory: "Supermercado", Date: "2026-07-07"},
		{Type: "expense", Amount: "300", Currency: "ARS", Category: "Alimentos", Subcategory: "Supermercado", Date: "2026-07-07"},
	}
	data := conversation.Data{conversation.UserIDKey: uint64(1), "mode": "create", "movements": encodeMovementRows(rows), "old_movement_ids": encodeStringSlice(nil)}

	if _, err := c.resolveAndInsertMovements(data); err != nil {
		t.Fatalf("resolveAndInsert: %v", err)
	}
	if len(movs.inserted) != 2 {
		t.Fatalf("inserted %d, want 2", len(movs.inserted))
	}
	if movs.inserted[0].TransactionID != nil || movs.inserted[1].TransactionID != nil {
		t.Error("independent expenses must not share a transaction_id")
	}
	if !movs.inserted[0].Amount.IsNegative() {
		t.Error("expense must be stored negative")
	}
}

func TestResolveAndInsert_ExpenseNeverCreatesCounterpartyAccount(t *testing.T) {
	subs := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Ocio y salidas|Restaurante": newSubForTest(6, "Ocio y salidas", "Restaurante"),
	}}
	accts := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}
	movs := &fakeMovementRepoFull{balances: map[uint64]string{1: "1000000"}}
	c := &controller{subcategories: subs, accounts: accts, movements: movs}

	// expense carrying an account_name_guess ("Pablo") + no AccountID
	rows := []movementRow{{Type: "expense", Amount: "100000", Currency: "ARS",
		Category: "Ocio y salidas", Subcategory: "Restaurante", Merchant: "Pablo",
		AccountNameGuess: "Pablo", AccountID: accountPendingCreate, Date: "2026-07-07"}}
	data := conversation.Data{conversation.UserIDKey: uint64(1), "mode": "create", "movements": encodeMovementRows(rows), "old_movement_ids": encodeStringSlice(nil)}

	if _, err := c.resolveAndInsertMovements(data); err != nil {
		t.Fatalf("resolveAndInsert: %v", err)
	}
	if len(accts.inserted) != 0 {
		t.Errorf("created %d accounts, want 0 (no 'Pablo' account for an expense)", len(accts.inserted))
	}
	if movs.inserted[0].AccountID == nil || *movs.inserted[0].AccountID != 1 {
		t.Error("expense must attribute to the default ARS account")
	}
}

// recordingTransport captures the "text" field of every Telegram sendMessage
// call, in order — lets a test assert how many messages went out and what
// each one said, without a live bot. go-telegram/bot sends multipart/form-data.
type recordingTransport struct{ texts []string }

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := req.ParseMultipartForm(1 << 20); err == nil {
		rt.texts = append(rt.texts, req.FormValue("text"))
	}
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"ok":true,"result":{"message_id":1,"chat":{"id":1},"date":0}}`))),
		Header:     make(http.Header),
	}, nil
}

func TestMovementCreate_FirstAccount_SendsDefaultAndInvite(t *testing.T) {
	sub := newSubForTest(1, "Alimentación", "Supermercado")
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Alimentación|Supermercado": sub,
	}}
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{}
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	rows := []movementRow{
		{Type: "expense", Amount: "500", Currency: "ARS", Category: "Alimentación", Subcategory: "Supermercado", Date: "2026-07-02"},
	}
	data := conversation.Data{
		conversation.UserIDKey:  uint64(1),
		"movements":             encodeMovementRows(rows),
		"pending_category_gaps": encodeStringSlice(nil),
		"pending_account_gaps":  encodeStringSlice(nil),
		keyFirstAccountName:     "Galicia",
	}

	rt := &recordingTransport{}
	b, err := bot.New("123:ABC", bot.WithSkipGetMe(), bot.WithHTTPClient(time.Second, &http.Client{Transport: rt}))
	if err != nil {
		t.Fatalf("bot.New: %v", err)
	}

	c.finishMovementCreateFlow(context.Background(), b, 1, data)

	if len(rt.texts) != 3 {
		t.Fatalf("expected 3 messages (recibo + R1 + R2), got %d: %+v", len(rt.texts), rt.texts)
	}
	if rt.texts[1] != msgFirstAccountDefault("Galicia") {
		t.Errorf("R1 = %q, want %q", rt.texts[1], msgFirstAccountDefault("Galicia"))
	}
	if rt.texts[2] != msgInviteMoreAccounts {
		t.Errorf("R2 = %q, want %q", rt.texts[2], msgInviteMoreAccounts)
	}
}
