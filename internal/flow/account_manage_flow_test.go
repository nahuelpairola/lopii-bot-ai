package flow

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

func TestAccountManageFlow_SeededMatch_SkipsPick(t *testing.T) {
	engine, store := newAccountManageTestEngine()
	const userID = uint64(1)
	if _, err := engine.StartWithData(userID, AccountManageFlowName, manageSeed(true)); err != nil {
		t.Fatalf("StartWithData: %v", err)
	}
	if store.stepName != StepAccountManageMenu {
		t.Errorf("stepName = %q, want %q (pick should be skipped)", store.stepName, StepAccountManageMenu)
	}
}

func TestAccountManageFlow_Pick_SelectsCandidate(t *testing.T) {
	engine, store := newAccountManageTestEngine()
	const userID = uint64(1)
	engine.StartWithData(userID, AccountManageFlowName, manageSeed(false))
	if store.stepName != StepAccountManagePick {
		t.Fatalf("initial stepName = %q, want %q", store.stepName, StepAccountManagePick)
	}
	result, found, err := engine.Handle(userID, conversation.Input{CallbackData: "pick_0"})
	if err != nil || !found || result.Finished {
		t.Fatalf("pick_0: result=%+v found=%v err=%v", result, found, err)
	}
	if store.stepName != StepAccountManageMenu {
		t.Errorf("stepName = %q, want %q", store.stepName, StepAccountManageMenu)
	}
	if store.data["account_id"] != "10" || store.data["account_name"] != "Wallet" || store.data["account_currency"] != "ARS" {
		t.Errorf("picked fields = %v/%v/%v, want 10/Wallet/ARS",
			store.data["account_id"], store.data["account_name"], store.data["account_currency"])
	}
}

func TestAccountManageFlow_Pick_CreateNew(t *testing.T) {
	engine, _ := newAccountManageTestEngine()
	const userID = uint64(1)
	engine.StartWithData(userID, AccountManageFlowName, manageSeed(false))
	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: OptionManageCreate})
	if err != nil || !result.Finished {
		t.Fatalf("create-new: result=%+v err=%v", result, err)
	}
	if result.Data["operation"] != "create_new" {
		t.Errorf("operation = %v, want create_new", result.Data["operation"])
	}
}

func TestAccountManageFlow_Pick_Cancel(t *testing.T) {
	engine, _ := newAccountManageTestEngine()
	const userID = uint64(1)
	engine.StartWithData(userID, AccountManageFlowName, manageSeed(false))
	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "cancel"})
	if err != nil || !result.Finished {
		t.Fatalf("cancel at pick: result=%+v err=%v", result, err)
	}
	if result.Data["cancelled"] != "true" {
		t.Errorf("cancelled = %v, want true", result.Data["cancelled"])
	}
}

func TestAccountManageFlow_Rename_HappyPath(t *testing.T) {
	engine, store := newAccountManageTestEngine()
	const userID = uint64(1)
	engine.StartWithData(userID, AccountManageFlowName, manageSeed(true))

	engine.Handle(userID, conversation.Input{CallbackData: OptionManageRename})
	if store.stepName != StepAccountManageAskName {
		t.Fatalf("after op_rename stepName = %q, want %q", store.stepName, StepAccountManageAskName)
	}
	result, _, _ := engine.Handle(userID, conversation.Input{Text: "   "})
	if result.Finished || store.stepName != StepAccountManageAskName {
		t.Fatalf("empty name should retry: finished=%v step=%q", result.Finished, store.stepName)
	}
	engine.Handle(userID, conversation.Input{Text: "FCI"})
	if store.stepName != StepAccountManageConfirmRename {
		t.Fatalf("after name stepName = %q, want %q", store.stepName, StepAccountManageConfirmRename)
	}
	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "confirm"})
	if err != nil || !result.Finished {
		t.Fatalf("confirm rename: result=%+v err=%v", result, err)
	}
	if result.Data["operation"] != "rename" || result.Data["new_name"] != "FCI" {
		t.Errorf("operation/new_name = %v/%v, want rename/FCI", result.Data["operation"], result.Data["new_name"])
	}
}

func TestAccountManageFlow_Rename_BackFromConfirm(t *testing.T) {
	engine, store := newAccountManageTestEngine()
	const userID = uint64(1)
	engine.StartWithData(userID, AccountManageFlowName, manageSeed(true))
	engine.Handle(userID, conversation.Input{CallbackData: OptionManageRename})
	engine.Handle(userID, conversation.Input{Text: "FCI"})

	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "back"})
	if err != nil || result.Finished {
		t.Fatalf("back from confirm rename: result=%+v err=%v", result, err)
	}
	if store.stepName != StepAccountManageAskName {
		t.Errorf("stepName = %q, want %q", store.stepName, StepAccountManageAskName)
	}
}

func TestAccountManageFlow_Adjust_HappyPath(t *testing.T) {
	engine, store := newAccountManageTestEngine()
	const userID = uint64(1)
	engine.StartWithData(userID, AccountManageFlowName, manageSeed(true))

	engine.Handle(userID, conversation.Input{CallbackData: OptionManageAdjust})
	if store.stepName != StepAccountManageAskTotal {
		t.Fatalf("after op_adjust stepName = %q, want %q", store.stepName, StepAccountManageAskTotal)
	}
	result, _, _ := engine.Handle(userID, conversation.Input{Text: "abc"})
	if result.Finished || store.stepName != StepAccountManageAskTotal {
		t.Fatal("non-numeric total should retry")
	}
	result, _, _ = engine.Handle(userID, conversation.Input{Text: "-5"})
	if result.Finished || store.stepName != StepAccountManageAskTotal {
		t.Fatal("negative total should retry")
	}
	engine.Handle(userID, conversation.Input{Text: "52000"})
	if store.stepName != StepAccountManageConfirmAdjust {
		t.Fatalf("after total stepName = %q, want %q", store.stepName, StepAccountManageConfirmAdjust)
	}
	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "confirm"})
	if err != nil || !result.Finished {
		t.Fatalf("confirm adjust: result=%+v err=%v", result, err)
	}
	if result.Data["operation"] != "adjust" || result.Data["new_total"] != "52000" {
		t.Errorf("operation/new_total = %v/%v, want adjust/52000", result.Data["operation"], result.Data["new_total"])
	}
}

func TestAccountManageFlow_Adjust_ZeroAccepted(t *testing.T) {
	engine, store := newAccountManageTestEngine()
	const userID = uint64(1)
	engine.StartWithData(userID, AccountManageFlowName, manageSeed(true))
	engine.Handle(userID, conversation.Input{CallbackData: OptionManageAdjust})
	engine.Handle(userID, conversation.Input{Text: "0"})
	if store.stepName != StepAccountManageConfirmAdjust {
		t.Fatalf("total 0 should advance to confirm, stepName = %q", store.stepName)
	}
	result, _, _ := engine.Handle(userID, conversation.Input{CallbackData: "confirm"})
	if !result.Finished || result.Data["new_total"] != "0" {
		t.Errorf("confirm with 0: finished=%v new_total=%v", result.Finished, result.Data["new_total"])
	}
}

func TestAccountManageFlow_Default_HappyPath(t *testing.T) {
	engine, _ := newAccountManageTestEngine()
	const userID = uint64(1)
	engine.StartWithData(userID, AccountManageFlowName, manageSeed(true))
	engine.Handle(userID, conversation.Input{CallbackData: OptionManageDefault})
	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "confirm"})
	if err != nil || !result.Finished {
		t.Fatalf("confirm default: result=%+v err=%v", result, err)
	}
	if result.Data["operation"] != "default" {
		t.Errorf("operation = %v, want default", result.Data["operation"])
	}
}

func TestAccountManageFlow_Menu_Cancel(t *testing.T) {
	engine, _ := newAccountManageTestEngine()
	const userID = uint64(1)
	engine.StartWithData(userID, AccountManageFlowName, manageSeed(true))
	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "cancel"})
	if err != nil || !result.Finished {
		t.Fatalf("cancel at menu: result=%+v err=%v", result, err)
	}
	if result.Data["cancelled"] != "true" {
		t.Errorf("cancelled = %v, want true", result.Data["cancelled"])
	}
}

func TestAccountManageFlow_CancelInsideTextSteps(t *testing.T) {
	const userID = uint64(1)

	engine, _ := newAccountManageTestEngine()
	engine.StartWithData(userID, AccountManageFlowName, manageSeed(true))
	engine.Handle(userID, conversation.Input{CallbackData: OptionManageRename})
	result, _, _ := engine.Handle(userID, conversation.Input{CallbackData: "cancel"})
	if !result.Finished || result.Data["cancelled"] != "true" {
		t.Fatalf("cancel in name step: finished=%v cancelled=%v", result.Finished, result.Data["cancelled"])
	}

	engine2, _ := newAccountManageTestEngine()
	engine2.StartWithData(userID, AccountManageFlowName, manageSeed(true))
	engine2.Handle(userID, conversation.Input{CallbackData: OptionManageAdjust})
	result, _, _ = engine2.Handle(userID, conversation.Input{CallbackData: "cancel"})
	if !result.Finished || result.Data["cancelled"] != "true" {
		t.Fatalf("cancel in total step: finished=%v cancelled=%v", result.Finished, result.Data["cancelled"])
	}
}
