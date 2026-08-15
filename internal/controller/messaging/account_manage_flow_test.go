package messaging

import (
	"testing"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/conversation"
)

type fakeBalanceSummer struct{ sum decimal.Decimal }

func (f fakeBalanceSummer) SumAmountForAccount(uint64) (decimal.Decimal, error) {
	return f.sum, nil
}

func newAccountManageTestEngine() (*conversation.Engine, *fakeStateStore) {
	store := &fakeStateStore{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewAccountManageFlow(fakeBalanceSummer{sum: decimal.NewFromInt(50000)}))
	return engine, store
}

func manageSeed(withAccount bool) conversation.Data {
	seed := conversation.Data{
		"message":              "x",
		"candidate_ids":        conversation.EncodeStringSlice([]string{"10", "20"}),
		"candidate_labels":     conversation.EncodeStringSlice([]string{"Wallet (ARS)", "FCI (USD)"}),
		"candidate_names":      conversation.EncodeStringSlice([]string{"Wallet", "FCI"}),
		"candidate_currencies": conversation.EncodeStringSlice([]string{"ARS", "USD"}),
	}
	if withAccount {
		seed["account_id"] = "10"
		seed["account_name"] = "Wallet"
		seed["account_currency"] = "ARS"
	}
	return seed
}

// (a) seeded with account_id → pick is skipped, initial prompt is the menu.
func TestAccountManageFlow_SeededMatch_SkipsPick(t *testing.T) {
	engine, store := newAccountManageTestEngine()
	const userID = uint64(1)
	if _, err := engine.StartWithData(userID, accountManageFlowName, manageSeed(true)); err != nil {
		t.Fatalf("StartWithData: %v", err)
	}
	if store.stepName != stepAccountManageMenu {
		t.Errorf("stepName = %q, want %q (pick should be skipped)", store.stepName, stepAccountManageMenu)
	}
}

// (b) no account_id → pick lists candidates; pick_0 sets fields + shows menu.
func TestAccountManageFlow_Pick_SelectsCandidate(t *testing.T) {
	engine, store := newAccountManageTestEngine()
	const userID = uint64(1)
	engine.StartWithData(userID, accountManageFlowName, manageSeed(false))
	if store.stepName != stepAccountManagePick {
		t.Fatalf("initial stepName = %q, want %q", store.stepName, stepAccountManagePick)
	}
	result, found, err := engine.Handle(userID, conversation.Input{CallbackData: "pick_0"})
	if err != nil || !found || result.Finished {
		t.Fatalf("pick_0: result=%+v found=%v err=%v", result, found, err)
	}
	if store.stepName != stepAccountManageMenu {
		t.Errorf("stepName = %q, want %q", store.stepName, stepAccountManageMenu)
	}
	if store.data["account_id"] != "10" || store.data["account_name"] != "Wallet" || store.data["account_currency"] != "ARS" {
		t.Errorf("picked fields = %v/%v/%v, want 10/Wallet/ARS",
			store.data["account_id"], store.data["account_name"], store.data["account_currency"])
	}
}

// (c) pick → create-new finishes with operation=create_new.
func TestAccountManageFlow_Pick_CreateNew(t *testing.T) {
	engine, _ := newAccountManageTestEngine()
	const userID = uint64(1)
	engine.StartWithData(userID, accountManageFlowName, manageSeed(false))
	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: optionManageCreate})
	if err != nil || !result.Finished {
		t.Fatalf("create-new: result=%+v err=%v", result, err)
	}
	if result.Data["operation"] != "create_new" {
		t.Errorf("operation = %v, want create_new", result.Data["operation"])
	}
}

// (d) pick → Cancelar finishes with cancelled=true.
func TestAccountManageFlow_Pick_Cancel(t *testing.T) {
	engine, _ := newAccountManageTestEngine()
	const userID = uint64(1)
	engine.StartWithData(userID, accountManageFlowName, manageSeed(false))
	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "cancel"})
	if err != nil || !result.Finished {
		t.Fatalf("cancel at pick: result=%+v err=%v", result, err)
	}
	if result.Data["cancelled"] != "true" {
		t.Errorf("cancelled = %v, want true", result.Data["cancelled"])
	}
}

// (e) menu → rename → empty name retries → valid name → confirm finishes.
func TestAccountManageFlow_Rename_HappyPath(t *testing.T) {
	engine, store := newAccountManageTestEngine()
	const userID = uint64(1)
	engine.StartWithData(userID, accountManageFlowName, manageSeed(true))

	engine.Handle(userID, conversation.Input{CallbackData: optionManageRename})
	if store.stepName != stepAccountManageAskName {
		t.Fatalf("after op_rename stepName = %q, want %q", store.stepName, stepAccountManageAskName)
	}
	// empty name → retry
	result, _, _ := engine.Handle(userID, conversation.Input{Text: "   "})
	if result.Finished || store.stepName != stepAccountManageAskName {
		t.Fatalf("empty name should retry: finished=%v step=%q", result.Finished, store.stepName)
	}
	// valid name → confirm step
	engine.Handle(userID, conversation.Input{Text: "FCI"})
	if store.stepName != stepAccountManageConfirmRename {
		t.Fatalf("after name stepName = %q, want %q", store.stepName, stepAccountManageConfirmRename)
	}
	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "confirm"})
	if err != nil || !result.Finished {
		t.Fatalf("confirm rename: result=%+v err=%v", result, err)
	}
	if result.Data["operation"] != "rename" || result.Data["new_name"] != "FCI" {
		t.Errorf("operation/new_name = %v/%v, want rename/FCI", result.Data["operation"], result.Data["new_name"])
	}
}

// (f) rename confirm → Atrás returns to the name step (does not finish).
func TestAccountManageFlow_Rename_BackFromConfirm(t *testing.T) {
	engine, store := newAccountManageTestEngine()
	const userID = uint64(1)
	engine.StartWithData(userID, accountManageFlowName, manageSeed(true))
	engine.Handle(userID, conversation.Input{CallbackData: optionManageRename})
	engine.Handle(userID, conversation.Input{Text: "FCI"})

	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "back"})
	if err != nil || result.Finished {
		t.Fatalf("back from confirm rename: result=%+v err=%v", result, err)
	}
	if store.stepName != stepAccountManageAskName {
		t.Errorf("stepName = %q, want %q", store.stepName, stepAccountManageAskName)
	}
}

// (g) menu → adjust → non-numeric/negative retry, then valid total confirms.
func TestAccountManageFlow_Adjust_HappyPath(t *testing.T) {
	engine, store := newAccountManageTestEngine()
	const userID = uint64(1)
	engine.StartWithData(userID, accountManageFlowName, manageSeed(true))

	engine.Handle(userID, conversation.Input{CallbackData: optionManageAdjust})
	if store.stepName != stepAccountManageAskTotal {
		t.Fatalf("after op_adjust stepName = %q, want %q", store.stepName, stepAccountManageAskTotal)
	}
	result, _, _ := engine.Handle(userID, conversation.Input{Text: "abc"})
	if result.Finished || store.stepName != stepAccountManageAskTotal {
		t.Fatal("non-numeric total should retry")
	}
	result, _, _ = engine.Handle(userID, conversation.Input{Text: "-5"})
	if result.Finished || store.stepName != stepAccountManageAskTotal {
		t.Fatal("negative total should retry")
	}
	engine.Handle(userID, conversation.Input{Text: "52000"})
	if store.stepName != stepAccountManageConfirmAdjust {
		t.Fatalf("after total stepName = %q, want %q", store.stepName, stepAccountManageConfirmAdjust)
	}
	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "confirm"})
	if err != nil || !result.Finished {
		t.Fatalf("confirm adjust: result=%+v err=%v", result, err)
	}
	if result.Data["operation"] != "adjust" || result.Data["new_total"] != "52000" {
		t.Errorf("operation/new_total = %v/%v, want adjust/52000", result.Data["operation"], result.Data["new_total"])
	}
}

// (g') "dejar en cero": total 0 is accepted.
func TestAccountManageFlow_Adjust_ZeroAccepted(t *testing.T) {
	engine, store := newAccountManageTestEngine()
	const userID = uint64(1)
	engine.StartWithData(userID, accountManageFlowName, manageSeed(true))
	engine.Handle(userID, conversation.Input{CallbackData: optionManageAdjust})
	engine.Handle(userID, conversation.Input{Text: "0"})
	if store.stepName != stepAccountManageConfirmAdjust {
		t.Fatalf("total 0 should advance to confirm, stepName = %q", store.stepName)
	}
	result, _, _ := engine.Handle(userID, conversation.Input{CallbackData: "confirm"})
	if !result.Finished || result.Data["new_total"] != "0" {
		t.Errorf("confirm with 0: finished=%v new_total=%v", result.Finished, result.Data["new_total"])
	}
}

// (h) menu → default → confirm finishes with operation=default.
func TestAccountManageFlow_Default_HappyPath(t *testing.T) {
	engine, _ := newAccountManageTestEngine()
	const userID = uint64(1)
	engine.StartWithData(userID, accountManageFlowName, manageSeed(true))
	engine.Handle(userID, conversation.Input{CallbackData: optionManageDefault})
	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "confirm"})
	if err != nil || !result.Finished {
		t.Fatalf("confirm default: result=%+v err=%v", result, err)
	}
	if result.Data["operation"] != "default" {
		t.Errorf("operation = %v, want default", result.Data["operation"])
	}
}

// (i) Cancelar in the menu finishes with cancelled=true.
func TestAccountManageFlow_Menu_Cancel(t *testing.T) {
	engine, _ := newAccountManageTestEngine()
	const userID = uint64(1)
	engine.StartWithData(userID, accountManageFlowName, manageSeed(true))
	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "cancel"})
	if err != nil || !result.Finished {
		t.Fatalf("cancel at menu: result=%+v err=%v", result, err)
	}
	if result.Data["cancelled"] != "true" {
		t.Errorf("cancelled = %v, want true", result.Data["cancelled"])
	}
}

// (j) Cancelar inside the name TextStep and inside the total TextStep.
func TestAccountManageFlow_CancelInsideTextSteps(t *testing.T) {
	const userID = uint64(1)

	engine, _ := newAccountManageTestEngine()
	engine.StartWithData(userID, accountManageFlowName, manageSeed(true))
	engine.Handle(userID, conversation.Input{CallbackData: optionManageRename})
	result, _, _ := engine.Handle(userID, conversation.Input{CallbackData: "cancel"})
	if !result.Finished || result.Data["cancelled"] != "true" {
		t.Fatalf("cancel in name step: finished=%v cancelled=%v", result.Finished, result.Data["cancelled"])
	}

	engine2, _ := newAccountManageTestEngine()
	engine2.StartWithData(userID, accountManageFlowName, manageSeed(true))
	engine2.Handle(userID, conversation.Input{CallbackData: optionManageAdjust})
	result, _, _ = engine2.Handle(userID, conversation.Input{CallbackData: "cancel"})
	if !result.Finished || result.Data["cancelled"] != "true" {
		t.Fatalf("cancel in total step: finished=%v cancelled=%v", result.Finished, result.Data["cancelled"])
	}
}
