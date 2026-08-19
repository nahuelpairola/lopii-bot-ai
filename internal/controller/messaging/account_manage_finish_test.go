package messaging

import (
	"context"
	"testing"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
)

func resolvedContains(m *fakeMetricRepo, outcome string) bool {
	for _, o := range m.resolved {
		if o == outcome {
			return true
		}
	}
	return false
}

// (f) rename OK → Rename applied, account_renamed resolved.
func TestFinishAccountManage_Rename_Success(t *testing.T) {
	accRepo := &fakeAccountRepoFull{}
	metrics := &fakeMetricRepo{}
	c := &controller{accounts: accRepo, metrics: metrics}

	data := conversation.Data{
		conversation.UserIDKey: uint64(1),
		"operation":            "rename",
		"account_id":           "5",
		"account_name":         "Wallet",
		"account_currency":     "ARS",
		"new_name":             "FCI",
	}
	c.finishAccountManageFlow(context.Background(), nil, 0, data)

	if accRepo.renamedID != 5 || accRepo.renamedName != "FCI" {
		t.Errorf("renamed = %d/%q, want 5/FCI", accRepo.renamedID, accRepo.renamedName)
	}
	if !resolvedContains(metrics, outcomeAccountRenamed) {
		t.Errorf("outcomes = %v, want to contain %q", metrics.resolved, outcomeAccountRenamed)
	}
}

// (g) rename collision → ErrAccountAlreadyExists, no success outcome.
func TestFinishAccountManage_Rename_Collision(t *testing.T) {
	accRepo := &fakeAccountRepoFull{renameErr: account.ErrAccountAlreadyExists}
	metrics := &fakeMetricRepo{}
	c := &controller{accounts: accRepo, metrics: metrics}

	data := conversation.Data{
		conversation.UserIDKey: uint64(1),
		"operation":            "rename",
		"account_id":           "5",
		"account_name":         "Wallet",
		"account_currency":     "ARS",
		"new_name":             "FCI",
	}
	c.finishAccountManageFlow(context.Background(), nil, 0, data)

	if resolvedContains(metrics, outcomeAccountRenamed) {
		t.Error("a name collision must not report a successful rename")
	}
}

// (h) cancelled → cancelled outcome, no writes.
func TestFinishAccountManage_Cancelled_NoWrite(t *testing.T) {
	accRepo := &fakeAccountRepoFull{}
	metrics := &fakeMetricRepo{}
	c := &controller{accounts: accRepo, metrics: metrics}

	data := conversation.Data{conversation.UserIDKey: uint64(1), "cancelled": "true"}
	c.finishAccountManageFlow(context.Background(), nil, 0, data)

	if accRepo.renamedID != 0 {
		t.Error("a cancelled flow must not rename anything")
	}
	if !resolvedContains(metrics, outcomeAccountManageCancelled) {
		t.Errorf("outcomes = %v, want to contain %q", metrics.resolved, outcomeAccountManageCancelled)
	}
}

// (i) create_new → starts the create flow with the seeded message.
func TestFinishAccountManage_CreateNew_StartsCreate(t *testing.T) {
	orch := &fakeFullOrchestrator{}
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow.NewAccountCreateFlow())
	c := &controller{orchestrator: orch, engine: engine, metrics: &fakeMetricRepo{}}

	data := conversation.Data{
		conversation.UserIDKey: uint64(1),
		"operation":            "create_new",
		"message":              "cuenta nueva de cedears",
	}
	c.finishAccountManageFlow(context.Background(), nil, 0, data)

	if store.flowName != flow.AccountCreateFlowName {
		t.Errorf("started flow = %q, want %q", store.flowName, flow.AccountCreateFlowName)
	}
}
