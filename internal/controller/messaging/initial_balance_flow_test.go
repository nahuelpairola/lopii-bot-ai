package messaging

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

// fakeStateStore es una implementación mínima en memoria de la interfaz
// (no exportada) que espera conversation.NewEngine, para poder ejercitar
// el flow real de punta a punta sin tocar Postgres.
type fakeStateStore struct {
	flowName  string
	stepName  string
	data      conversation.Data
	updatedAt time.Time
	found     bool
}

func (s *fakeStateStore) Get(userID uint64) (string, string, conversation.Data, time.Time, bool, error) {
	return s.flowName, s.stepName, s.data, s.updatedAt, s.found, nil
}

func (s *fakeStateStore) Set(userID uint64, flowName, stepName string, data conversation.Data) error {
	s.flowName, s.stepName, s.data, s.found = flowName, stepName, data, true
	s.updatedAt = time.Now()
	return nil
}

func (s *fakeStateStore) Clear(userID uint64) error {
	s.found = false
	return nil
}

func newTestEngine() (*conversation.Engine, *fakeStateStore) {
	store := &fakeStateStore{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewInitialBalanceFlow())
	return engine, store
}

func TestInitialBalanceFlow_HappyPath(t *testing.T) {
	engine, _ := newTestEngine()
	const userID = uint64(1)

	if _, err := engine.Start(userID, initialBalanceFlowName); err != nil {
		t.Fatalf("Start: %v", err)
	}

	result, found, err := engine.Handle(userID, conversation.Input{Text: "50000"})
	if err != nil || !found || result.Finished {
		t.Fatalf("ARS step: result=%+v found=%v err=%v", result, found, err)
	}

	result, found, err = engine.Handle(userID, conversation.Input{Text: "100"})
	if err != nil || !found || result.Finished {
		t.Fatalf("USD step: result=%+v found=%v err=%v", result, found, err)
	}

	result, found, err = engine.Handle(userID, conversation.Input{CallbackData: "confirm"})
	if err != nil || !found || !result.Finished {
		t.Fatalf("confirm step: result=%+v found=%v err=%v", result, found, err)
	}
	if result.Data[balanceDataKey(currency.ARS)] != "50000" {
		t.Errorf("ars balance = %v, want 50000", result.Data[balanceDataKey(currency.ARS)])
	}
	if result.Data[balanceDataKey(currency.USD)] != "100" {
		t.Errorf("usd balance = %v, want 100", result.Data[balanceDataKey(currency.USD)])
	}
}

func TestInitialBalanceFlow_ZeroAmountAccepted(t *testing.T) {
	engine, _ := newTestEngine()
	const userID = uint64(1)
	engine.Start(userID, initialBalanceFlowName)

	if result, _, err := engine.Handle(userID, conversation.Input{Text: "0"}); err != nil || result.Finished {
		t.Fatalf("ARS=0 should advance, not finish: result=%+v err=%v", result, err)
	}
}

func TestInitialBalanceFlow_InvalidAmount_Retries(t *testing.T) {
	engine, store := newTestEngine()
	const userID = uint64(1)
	engine.Start(userID, initialBalanceFlowName)

	for _, text := range []string{"not-a-number", "-5"} {
		result, found, err := engine.Handle(userID, conversation.Input{Text: text})
		if err != nil || !found {
			t.Fatalf("Handle(%q): found=%v err=%v", text, found, err)
		}
		if result.Finished {
			t.Fatalf("Handle(%q) should not finish the flow", text)
		}
		if store.stepName != askBalanceStepName(currency.ARS) {
			t.Errorf("Handle(%q): expected to stay on %q, got %q", text, askBalanceStepName(currency.ARS), store.stepName)
		}
	}
}

func TestInitialBalanceFlow_Correct_RestartsFromARS(t *testing.T) {
	engine, store := newTestEngine()
	const userID = uint64(1)
	engine.Start(userID, initialBalanceFlowName)

	engine.Handle(userID, conversation.Input{Text: "50000"})
	engine.Handle(userID, conversation.Input{Text: "100"})

	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "retry"})
	if err != nil || result.Finished {
		t.Fatalf("retry should not finish the flow: result=%+v err=%v", result, err)
	}
	if store.stepName != askBalanceStepName(currency.ARS) {
		t.Fatalf("expected to be back at %q, got %q", askBalanceStepName(currency.ARS), store.stepName)
	}

	engine.Handle(userID, conversation.Input{Text: "999"})
	engine.Handle(userID, conversation.Input{Text: "1"})
	result, _, err = engine.Handle(userID, conversation.Input{CallbackData: "confirm"})
	if err != nil || !result.Finished {
		t.Fatalf("expected finish on second pass: result=%+v err=%v", result, err)
	}
	if result.Data[balanceDataKey(currency.ARS)] != "999" || result.Data[balanceDataKey(currency.USD)] != "1" {
		t.Errorf("stale values after correction: ars=%v usd=%v", result.Data[balanceDataKey(currency.ARS)], result.Data[balanceDataKey(currency.USD)])
	}
}

// --- insertInitialBalanceMovements ---

type fakeAccountRepo struct {
	byCurrency map[currency.Currency]*account.Account
	err        error
}

func (r *fakeAccountRepo) Insert(*account.Account) error { return nil }

func (r *fakeAccountRepo) FindDefaultByCurrency(userID uint64, c currency.Currency) (*account.Account, error) {
	if r.err != nil {
		return nil, r.err
	}
	a, ok := r.byCurrency[c]
	if !ok {
		return nil, errors.New("not found")
	}
	return a, nil
}

func (r *fakeAccountRepo) FindByUserID(userID uint64) ([]account.Account, error) {
	return nil, nil
}

func (r *fakeAccountRepo) GetAccount(id uint64) (*account.Account, error) {
	return nil, errors.New("not found")
}

type fakeSubcategoryRepo struct {
	sub *subcategory.Subcategory
	err error
}

func (r *fakeSubcategoryRepo) FindByCategoryAndSubcategory(userID uint64, category, sub string) (*subcategory.Subcategory, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.sub, nil
}

func (r *fakeSubcategoryRepo) FindAllForUser(userID uint64) ([]subcategory.Subcategory, error) {
	return nil, nil
}

func (r *fakeSubcategoryRepo) DistinctCategoriesForUser(userID uint64) ([]string, error) {
	return nil, nil
}

func (r *fakeSubcategoryRepo) IconForCategory(userID uint64, category string) string { return "📂" }
func (r *fakeSubcategoryRepo) Insert(s *subcategory.Subcategory) error               { return nil }
func (r *fakeSubcategoryRepo) Reload() error                                        { return nil }

type fakeMovementRepo struct {
	inserted []movement.Movement
	err      error
}

func (r *fakeMovementRepo) InsertBatch(ms []movement.Movement) error {
	if r.err != nil {
		return r.err
	}
	r.inserted = ms
	return nil
}

func (r *fakeMovementRepo) SumAmountForAccount(accountID uint64) (decimal.Decimal, error) {
	return decimal.Zero, nil
}

func (r *fakeMovementRepo) ReplaceMovements(oldIDs []uint, newMovements []movement.Movement) error {
	return nil
}

func (r *fakeMovementRepo) FindSimilarForUser(userID uint64, query string, since time.Time, until *time.Time) ([]movement.Movement, error) {
	return nil, nil
}

func (r *fakeMovementRepo) SoftDeleteByIDs(ids []uint) error {
	return nil
}

func testData(userID uint64, ars, usd string) conversation.Data {
	return conversation.Data{
		conversation.UserIDKey:       userID,
		balanceDataKey(currency.ARS): ars,
		balanceDataKey(currency.USD): usd,
	}
}

func TestInsertInitialBalanceMovements_Success(t *testing.T) {
	arsAccount := &account.Account{}
	arsAccount.ID = 10
	usdAccount := &account.Account{}
	usdAccount.ID = 20
	sub := &subcategory.Subcategory{}
	sub.ID = 5

	movements := &fakeMovementRepo{}
	c := &controller{
		accounts: &fakeAccountRepo{byCurrency: map[currency.Currency]*account.Account{
			currency.ARS: arsAccount,
			currency.USD: usdAccount,
		}},
		subcategories: &fakeSubcategoryRepo{sub: sub},
		movements:     movements,
	}

	if err := c.insertInitialBalanceMovements(testData(1, "50000", "100")); err != nil {
		t.Fatalf("insertInitialBalanceMovements: %v", err)
	}

	if len(movements.inserted) != 2 {
		t.Fatalf("expected 2 movements inserted, got %d", len(movements.inserted))
	}
	for _, m := range movements.inserted {
		if m.Type != movement.Transfer {
			t.Errorf("movement type = %v, want %v", m.Type, movement.Transfer)
		}
		if m.SubcategoryID != 5 {
			t.Errorf("subcategory id = %v, want 5", m.SubcategoryID)
		}
		if m.AccountID == nil {
			t.Errorf("account id should not be nil")
		}
		if m.TransactionID != nil {
			t.Errorf("transaction id should be nil for a standalone opening movement")
		}
	}
}

func TestInsertInitialBalanceMovements_SubcategoryNotFound(t *testing.T) {
	movements := &fakeMovementRepo{}
	c := &controller{
		accounts:      &fakeAccountRepo{},
		subcategories: &fakeSubcategoryRepo{err: subcategory.ErrSubcategoryNotFound},
		movements:     movements,
	}

	if err := c.insertInitialBalanceMovements(testData(1, "0", "0")); err == nil {
		t.Fatal("expected an error when the subcategory is missing")
	}
	if len(movements.inserted) != 0 {
		t.Errorf("no movement should be inserted when the subcategory lookup fails")
	}
}

func TestInsertInitialBalanceMovements_InsertFails(t *testing.T) {
	arsAccount := &account.Account{}
	usdAccount := &account.Account{}
	sub := &subcategory.Subcategory{}

	c := &controller{
		accounts: &fakeAccountRepo{byCurrency: map[currency.Currency]*account.Account{
			currency.ARS: arsAccount,
			currency.USD: usdAccount,
		}},
		subcategories: &fakeSubcategoryRepo{sub: sub},
		movements:     &fakeMovementRepo{err: errors.New("db down")},
	}

	if err := c.insertInitialBalanceMovements(testData(1, "0", "0")); err == nil {
		t.Fatal("expected InsertBatch failure to propagate")
	}
}
