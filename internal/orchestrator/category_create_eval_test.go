//go:build llm_eval

package orchestrator

import (
	"context"
	"os"
	"testing"
)

func TestCategoryCreateEval_MatchesExisting(t *testing.T) {
	key := evalKey(t)
	o := New(Config{
		APIKey:         key,
		BaseURL:        evalBaseURL(),
		CreateModel:    os.Getenv("GROQ_CREATE_MODEL"),
		TimeoutSeconds: 30,
	})
	taxonomy := []TaxonomyEntry{
		{Category: "Otros", Subcategory: "Regalos / donaciones", Description: "Regalos a terceros y donaciones."},
		{Category: "Alimentación", Subcategory: "Supermercado", Description: "Compras de supermercado."},
	}
	res, err := o.ClassifyCategoryCreate(context.Background(), "quiero una categoría para los regalos que hago", taxonomy)
	if err != nil {
		t.Fatal(err)
	}
	if res.Match == nil {
		t.Fatalf("expected a match against the existing Regalos entry, got proposal=%+v", res.Proposal)
	}
	if res.Match.Category != "Otros" || res.Match.Subcategory != "Regalos / donaciones" {
		t.Errorf("match = %+v, want Otros › Regalos / donaciones", res.Match)
	}
}

func TestCategoryCreateEval_ProposesWhenAbsent(t *testing.T) {
	key := evalKey(t)
	o := New(Config{
		APIKey:         key,
		BaseURL:        evalBaseURL(),
		CreateModel:    os.Getenv("GROQ_CREATE_MODEL"),
		TimeoutSeconds: 30,
	})
	taxonomy := []TaxonomyEntry{
		{Category: "Alimentación", Subcategory: "Supermercado", Description: "Compras de supermercado."},
		{Category: "Transporte", Subcategory: "Nafta", Description: "Combustible."},
	}
	res, err := o.ClassifyCategoryCreate(context.Background(), "quiero registrar los gastos del perro", taxonomy)
	if err != nil {
		t.Fatal(err)
	}
	if res.Proposal == nil {
		t.Fatalf("expected a proposal for pet expenses, got match=%+v", res.Match)
	}
	if res.Proposal.Category != "Mascotas" {
		t.Errorf("proposal category = %q, want %q for pet expenses", res.Proposal.Category, "Mascotas")
	}
	if res.Proposal.Description == "" {
		t.Error("proposal description must not be empty")
	}
	if res.Proposal.Icon == "" {
		t.Error("proposal emoji must not be empty (the schema-field-name fix)")
	}
}
