package messaging

import (
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
)

func newAccountCreateTestEngine() (*conversation.Engine, *fakeStateStore) {
	store := &fakeStateStore{}
	engine := conversation.NewEngine(store)
	engine.Register(NewAccountCreateFlow())
	return engine, store
}

func TestAccountCreateFlow_HappyPath(t *testing.T) {
	engine, _ := newAccountCreateTestEngine()
	const userID = uint64(1)

	if _, err := engine.Start(userID, accountCreateFlowName); err != nil {
		t.Fatalf("Start: %v", err)
	}

	result, found, err := engine.Handle(userID, conversation.Input{Text: "Jubilación"})
	if err != nil || !found || result.Finished {
		t.Fatalf("ask_name step: result=%+v found=%v err=%v", result, found, err)
	}

	result, found, err = engine.Handle(userID, conversation.Input{CallbackData: currency.ARS.String()})
	if err != nil || !found || result.Finished {
		t.Fatalf("ask_currency step: result=%+v found=%v err=%v", result, found, err)
	}

	result, found, err = engine.Handle(userID, conversation.Input{Text: "50000"})
	if err != nil || !found || result.Finished {
		t.Fatalf("ask_balance step: result=%+v found=%v err=%v", result, found, err)
	}

	result, found, err = engine.Handle(userID, conversation.Input{CallbackData: "confirm"})
	if err != nil || !found || !result.Finished {
		t.Fatalf("confirm step: result=%+v found=%v err=%v", result, found, err)
	}
	if result.Data["account_name"] != "Jubilación" {
		t.Errorf("account_name = %v, want %q", result.Data["account_name"], "Jubilación")
	}
	if result.Data["account_currency"] != currency.ARS.String() {
		t.Errorf("account_currency = %v, want %q", result.Data["account_currency"], currency.ARS.String())
	}
	if result.Data["account_balance"] != "50000" {
		t.Errorf("account_balance = %v, want %q", result.Data["account_balance"], "50000")
	}
	if result.Data["cancelled"] == "true" {
		t.Error("cancelled should not be set on a normal confirm")
	}
}

func TestAccountCreateFlow_CancelAtName(t *testing.T) {
	engine, _ := newAccountCreateTestEngine()
	const userID = uint64(1)
	engine.Start(userID, accountCreateFlowName)

	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "cancel"})
	if err != nil || !result.Finished {
		t.Fatalf("cancel at ask_name: result=%+v err=%v", result, err)
	}
	if result.Data["cancelled"] != "true" {
		t.Errorf("cancelled = %v, want \"true\"", result.Data["cancelled"])
	}
}

func TestAccountCreateFlow_CancelAtCurrency(t *testing.T) {
	engine, _ := newAccountCreateTestEngine()
	const userID = uint64(1)
	engine.Start(userID, accountCreateFlowName)
	engine.Handle(userID, conversation.Input{Text: "FCI"})

	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "cancel"})
	if err != nil || !result.Finished {
		t.Fatalf("cancel at ask_currency: result=%+v err=%v", result, err)
	}
	if result.Data["cancelled"] != "true" {
		t.Errorf("cancelled = %v, want \"true\"", result.Data["cancelled"])
	}
}

func TestAccountCreateFlow_CancelAtBalance(t *testing.T) {
	engine, _ := newAccountCreateTestEngine()
	const userID = uint64(1)
	engine.Start(userID, accountCreateFlowName)
	engine.Handle(userID, conversation.Input{Text: "FCI"})
	engine.Handle(userID, conversation.Input{CallbackData: currency.USD.String()})

	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "cancel"})
	if err != nil || !result.Finished {
		t.Fatalf("cancel at ask_balance: result=%+v err=%v", result, err)
	}
	if result.Data["cancelled"] != "true" {
		t.Errorf("cancelled = %v, want \"true\"", result.Data["cancelled"])
	}
}

func TestAccountCreateFlow_CancelAtConfirm(t *testing.T) {
	engine, _ := newAccountCreateTestEngine()
	const userID = uint64(1)
	engine.Start(userID, accountCreateFlowName)
	engine.Handle(userID, conversation.Input{Text: "FCI"})
	engine.Handle(userID, conversation.Input{CallbackData: currency.USD.String()})
	engine.Handle(userID, conversation.Input{Text: "0"})

	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "cancel"})
	if err != nil || !result.Finished {
		t.Fatalf("cancel at confirm: result=%+v err=%v", result, err)
	}
	if result.Data["cancelled"] != "true" {
		t.Errorf("cancelled = %v, want \"true\"", result.Data["cancelled"])
	}
}

func TestAccountCreateFlow_BackFromCurrencyToName(t *testing.T) {
	engine, store := newAccountCreateTestEngine()
	const userID = uint64(1)
	engine.Start(userID, accountCreateFlowName)
	engine.Handle(userID, conversation.Input{Text: "FCI"})

	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "back"})
	if err != nil || result.Finished {
		t.Fatalf("back from ask_currency: result=%+v err=%v", result, err)
	}
	if store.stepName != stepAccountCreateAskName {
		t.Errorf("stepName = %q, want %q", store.stepName, stepAccountCreateAskName)
	}
}

func TestAccountCreateFlow_BackFromBalanceToCurrency(t *testing.T) {
	engine, store := newAccountCreateTestEngine()
	const userID = uint64(1)
	engine.Start(userID, accountCreateFlowName)
	engine.Handle(userID, conversation.Input{Text: "FCI"})
	engine.Handle(userID, conversation.Input{CallbackData: currency.ARS.String()})

	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "back"})
	if err != nil || result.Finished {
		t.Fatalf("back from ask_balance: result=%+v err=%v", result, err)
	}
	if store.stepName != stepAccountCreateAskCurrency {
		t.Errorf("stepName = %q, want %q", store.stepName, stepAccountCreateAskCurrency)
	}
}

func TestAccountCreateFlow_BackFromConfirmToBalance(t *testing.T) {
	engine, store := newAccountCreateTestEngine()
	const userID = uint64(1)
	engine.Start(userID, accountCreateFlowName)
	engine.Handle(userID, conversation.Input{Text: "FCI"})
	engine.Handle(userID, conversation.Input{CallbackData: currency.ARS.String()})
	engine.Handle(userID, conversation.Input{Text: "100"})

	result, _, err := engine.Handle(userID, conversation.Input{CallbackData: "back"})
	if err != nil || result.Finished {
		t.Fatalf("back from confirm: result=%+v err=%v", result, err)
	}
	if store.stepName != stepAccountCreateAskBalance {
		t.Errorf("stepName = %q, want %q", store.stepName, stepAccountCreateAskBalance)
	}
}

func TestAccountCreateFlow_EmptyName_Retries(t *testing.T) {
	engine, store := newAccountCreateTestEngine()
	const userID = uint64(1)
	engine.Start(userID, accountCreateFlowName)

	result, found, err := engine.Handle(userID, conversation.Input{Text: "   "})
	if err != nil || !found {
		t.Fatalf("Handle(empty name): found=%v err=%v", found, err)
	}
	if result.Finished {
		t.Fatal("an empty name should not advance the flow")
	}
	if store.stepName != stepAccountCreateAskName {
		t.Errorf("stepName = %q, want %q", store.stepName, stepAccountCreateAskName)
	}
}

func TestAccountCreateFlow_InvalidBalance_Retries(t *testing.T) {
	engine, store := newAccountCreateTestEngine()
	const userID = uint64(1)
	engine.Start(userID, accountCreateFlowName)
	engine.Handle(userID, conversation.Input{Text: "FCI"})
	engine.Handle(userID, conversation.Input{CallbackData: currency.ARS.String()})

	result, found, err := engine.Handle(userID, conversation.Input{Text: "not-a-number"})
	if err != nil || !found {
		t.Fatalf("Handle(invalid balance): found=%v err=%v", found, err)
	}
	if result.Finished {
		t.Fatal("an invalid balance should not advance the flow")
	}
	if store.stepName != stepAccountCreateAskBalance {
		t.Errorf("stepName = %q, want %q", store.stepName, stepAccountCreateAskBalance)
	}
}
