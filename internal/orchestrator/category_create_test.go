package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func categoryCreateServer(t *testing.T, argsJSON string, capture *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		if capture != nil {
			*capture = req.Messages[0].Content
		}
		escaped, _ := json.Marshal(argsJSON)
		fmt.Fprintf(w, `{"choices":[{"message":{"tool_calls":[{"function":{"name":"resolve_category","arguments":%s}}]}}]}`, escaped)
	}))
}

func TestClassifyCategoryCreate_MatchPromptAndParse(t *testing.T) {
	var captured string
	srv := categoryCreateServer(t, `{"match":{"category":"Otros","subcategory":"Regalos / donaciones"},"proposal":null}`, &captured)
	defer srv.Close()

	o := New(Config{BaseURL: srv.URL, CreateModel: "m", TimeoutSeconds: 5})
	res, err := o.ClassifyCategoryCreate(context.Background(), "quiero una categoría para regalos", []TaxonomyEntry{
		{Category: "Otros", Subcategory: "Regalos / donaciones", Description: "Regalos a terceros y donaciones."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Match == nil || res.Match.Category != "Otros" || res.Match.Subcategory != "Regalos / donaciones" {
		t.Fatalf("match = %+v, want Otros › Regalos / donaciones", res.Match)
	}
	if res.Proposal != nil {
		t.Errorf("proposal should be nil on a match, got %+v", res.Proposal)
	}
	if !strings.Contains(captured, "Otros | Regalos / donaciones | Regalos a terceros y donaciones.") {
		t.Errorf("taxonomy line missing from system prompt:\n%s", captured)
	}
	if !strings.Contains(captured, `PROHIBIDO usar "Sistema" o "PENDING_REVIEW"`) {
		t.Errorf("prompt must state the Sistema/PENDING_REVIEW prohibition:\n%s", captured)
	}
}

func TestClassifyCategoryCreate_ProposalParse(t *testing.T) {
	srv := categoryCreateServer(t, `{"match":null,"proposal":{"category":"Mascotas","subcategory":"Veterinario","emoji":"🐶","description":"Gastos del veterinario y salud de mascotas."}}`, nil)
	defer srv.Close()

	o := New(Config{BaseURL: srv.URL, CreateModel: "m", TimeoutSeconds: 5})
	res, err := o.ClassifyCategoryCreate(context.Background(), "gastos del perro", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Proposal == nil || res.Proposal.Category != "Mascotas" || res.Proposal.Icon != "🐶" {
		t.Fatalf("proposal = %+v, want Mascotas/Veterinario/🐶", res.Proposal)
	}
	if res.Proposal.Description == "" {
		t.Error("proposal description must not be empty")
	}
}

func TestClassifyCategoryCreate_ErrorWhenBothNil(t *testing.T) {
	srv := categoryCreateServer(t, `{"match":null,"proposal":null}`, nil)
	defer srv.Close()

	o := New(Config{BaseURL: srv.URL, CreateModel: "m", TimeoutSeconds: 5})
	if _, err := o.ClassifyCategoryCreate(context.Background(), "algo raro", nil); err == nil {
		t.Error("expected an error when the model returns neither match nor proposal")
	}
}

func TestClassifyCategoryCreate_BothSetPrefersMatch(t *testing.T) {
	srv := categoryCreateServer(t, `{"match":{"category":"Otros","subcategory":"Regalos / donaciones"},"proposal":{"category":"Regalos","subcategory":"Regalos","emoji":"🎁","description":"x"}}`, nil)
	defer srv.Close()

	o := New(Config{BaseURL: srv.URL, CreateModel: "m", TimeoutSeconds: 5})
	res, err := o.ClassifyCategoryCreate(context.Background(), "regalos", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Match == nil || res.Proposal != nil {
		t.Errorf("when both set, keep match drop proposal; got match=%+v proposal=%+v", res.Match, res.Proposal)
	}
}
