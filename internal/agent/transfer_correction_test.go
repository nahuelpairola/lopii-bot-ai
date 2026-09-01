package agent

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

func TestResolveAndInsertMovements_TransferAmountCorrection(t *testing.T) {
	sub := newSubForTest(5, "Transferencias", "Entre cuentas")
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Transferencias|Entre cuentas": sub,
		"Inversiones|FCI":              newSubForTest(6, "Inversiones", "FCI"),
	}}
	accRepo := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true), acct(2, currency.ARS, false)}}
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{1: "1000000", 2: "1000000"}}
	svc := &fakeServices{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	txID := uuid.New()
	leg := func(accountID uint64, amount string) movement.Movement {
		return movement.Movement{
			UserID: 1, AccountID: &accountID, SubcategoryID: 5, Subcategory: sub,
			Type: movement.Transfer, Amount: mustDecimal(t, amount), Currency: currency.ARS,
			Date: time.Now(), TransactionID: &txID,
		}
	}
	before := []movement.MovementRow{
		movementToRow(leg(1, "-10000")),
		movementToRow(leg(2, "10000")),
	}

	corrected, err := applyChanges(before, []correctionChange{{fieldAmount, opSet, "100000"}})
	if err != nil {
		t.Fatalf("applyChanges: %v", err)
	}

	after := make([]movement.MovementRow, 0, len(corrected))
	for _, r := range corrected {
		after = append(after, draftToRow(rowToDraft(r)))
	}
	after = carryTransferIdentity(before, after)

	data := conversation.Data{
		conversation.KeyMode:           modeUpdate,
		conversation.KeyOldMovementIDs: conversation.EncodeStringSlice([]string{"307", "308"}),
		conversation.KeyMovements:      movement.EncodeMovementRows(after),
		conversation.UserIDKey:         uint64(1),
	}
	if _, err := svc.ResolveAndInsertMovements(data); err != nil {
		t.Fatalf("ResolveAndInsertMovements: %v", err)
	}

	if len(movRepo.replaced) != 2 {
		t.Fatalf("replaced %d filas, want 2", len(movRepo.replaced))
	}
	sum := decimal.Zero
	for _, m := range movRepo.replaced {
		if !m.Amount.Abs().Equal(decimal.NewFromInt(100000)) {
			t.Errorf("pata en cuenta %v = %s, want |100000|", m.AccountID, m.Amount)
		}
		if m.TransactionID == nil {
			t.Errorf("pata en cuenta %v sin transaction_id: el grupo se perdió", m.AccountID)
		}
		sum = sum.Add(m.Amount)
	}
	if !sum.IsZero() {
		t.Errorf("las dos patas suman %s, want 0 (una sale, otra entra)", sum)
	}
	if movRepo.replaced[0].AccountID != nil && *movRepo.replaced[0].AccountID == 1 && !movRepo.replaced[0].Amount.IsNegative() {
		t.Error("la pata de la cuenta origen tiene que salir negativa")
	}
}
