package orchestrator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func accountManageServer(args string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"` + args + `"}}]}}]}`))
	}))
}

func TestResolveAccountManage_MatchedAccount(t *testing.T) {
	server := accountManageServer(`{\"matched_account_id\":3,\"wants_new_account\":false}`)
	defer server.Close()

	o := New(Config{BaseURL: server.URL, CreateModel: "test-model", TimeoutSeconds: 5})
	accounts := []AccountOption{{ID: 3, Name: "Galicia", Currency: "ARS"}}

	res, err := o.ResolveAccountManage(context.Background(), "renombrá Galicia", accounts)
	if err != nil {
		t.Fatalf("ResolveAccountManage: %v", err)
	}
	if res.MatchedAccountID == nil || *res.MatchedAccountID != 3 {
		t.Errorf("MatchedAccountID = %v, want 3", res.MatchedAccountID)
	}
	if res.WantsNewAccount {
		t.Error("WantsNewAccount = true, want false")
	}
}

func TestResolveAccountManage_WantsNewAccount(t *testing.T) {
	server := accountManageServer(`{\"matched_account_id\":null,\"wants_new_account\":true}`)
	defer server.Close()

	o := New(Config{BaseURL: server.URL, CreateModel: "test-model", TimeoutSeconds: 5})

	res, err := o.ResolveAccountManage(context.Background(), "quiero una cuenta nueva", nil)
	if err != nil {
		t.Fatalf("ResolveAccountManage: %v", err)
	}
	if !res.WantsNewAccount {
		t.Error("WantsNewAccount = false, want true")
	}
	if res.MatchedAccountID != nil {
		t.Errorf("MatchedAccountID = %v, want nil", res.MatchedAccountID)
	}
}

func TestResolveAccountManage_Unclear(t *testing.T) {
	server := accountManageServer(`{\"matched_account_id\":null,\"wants_new_account\":false}`)
	defer server.Close()

	o := New(Config{BaseURL: server.URL, CreateModel: "test-model", TimeoutSeconds: 5})
	accounts := []AccountOption{{ID: 1, Name: "Wallet", Currency: "ARS"}}

	res, err := o.ResolveAccountManage(context.Background(), "cambiá el monto", accounts)
	if err != nil {
		t.Fatalf("ResolveAccountManage: %v", err)
	}
	if res.MatchedAccountID != nil {
		t.Errorf("MatchedAccountID = %v, want nil", res.MatchedAccountID)
	}
	if res.WantsNewAccount {
		t.Error("WantsNewAccount = true, want false")
	}
}
