//go:build llm_eval

package messaging

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"lopiibot.com/internal/orchestrator"
)

// Real-Groq eval for the ask-category/subcategory gap-fill prompts (the
// reported bug: a compound message with several PENDING_REVIEW rows produced
// identical, row-less ask-prompts, so the user couldn't tell which movement
// was being asked about). No DB needed — orchestrator.ClassifyCreate takes
// its taxonomy/accounts as plain slices.
//
// The taxonomy fed here is deliberately sparse (neither "super" nor "nafta"
// has a real home) so Call 2 CREATE can't clear the 90%% confidence bar for
// either row, landing both in PENDING_REVIEW — reproducing the multi-gap
// scenario from the bug report.
//
// Run:
//
//	set -a; . ./.env; set +a
//	go test -tags llm_eval ./internal/controller/messaging/ -run TestGapPromptEval -v
//
// Excluded from the default `go test ./...` (no tag) — needs a real key and
// makes a real (billed) Groq call.
func TestGapPromptEval(t *testing.T) {
	key := os.Getenv("GROQ_APIKEY")
	if key == "" {
		t.Skip("GROQ_APIKEY unset — real-LLM eval skipped")
	}
	baseURL := os.Getenv("GROQ_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.groq.com/openai/v1"
	}
	model := os.Getenv("GROQ_CREATE_MODEL")
	if model == "" {
		model = "openai/gpt-oss-20b"
	}
	orch := orchestrator.New(orchestrator.Config{
		APIKey: key, BaseURL: baseURL, RouterModel: model, CreateModel: model, TimeoutSeconds: 30,
	})

	taxonomy := []orchestrator.TaxonomyEntry{
		{Category: "Mascotas", Subcategory: "Veterinaria", Description: "gastos veterinarios"},
		{Category: "Tecnología", Subcategory: "Hardware", Description: "compra de dispositivos electrónicos"},
	}
	accounts := []orchestrator.AccountOption{{ID: 1, Name: "Banco", Currency: "ARS"}}

	result, err := orch.ClassifyCreate(context.Background(), "gasté 5000 en el super y 3000 de nafta", taxonomy, accounts, "2026-07-24")
	if err != nil {
		t.Fatalf("ClassifyCreate: %v", err)
	}
	if len(result.Movements) != 2 {
		t.Fatalf("want 2 movements, got %d: %+v", len(result.Movements), result.Movements)
	}
	t.Logf("classified: %+v", result.Movements)

	data := buildCreateSeed(result, nil)
	gaps := decodeStringSlice(data, keyPendingCategoryGaps)
	if len(gaps) != 2 {
		t.Fatalf("want both rows PENDING_REVIEW (2 gaps), got %d — taxonomy wasn't sparse enough to force it: %+v", len(gaps), result.Movements)
	}
	rows := decodeMovementRows(data)

	prompt0 := msgAskCategory(data)
	idx0, _ := strconv.Atoi(gaps[0])
	if !strings.Contains(prompt0, rows[idx0].Amount) {
		t.Errorf("category prompt missing row %d's own amount %q: %q", idx0, rows[idx0].Amount, prompt0)
	}
	if !strings.Contains(prompt0, "1 de 2") {
		t.Errorf("category prompt missing position counter: %q", prompt0)
	}

	// Answer row 0's category, check its subcategory prompt references the
	// same row (not the other one).
	data["gap_active_row"] = gaps[0]
	rows[idx0].Category = "Mascotas"
	data[keyMovements] = encodeMovementRows(rows)
	subPrompt := msgAskSubcategory(data)
	if !strings.Contains(subPrompt, rows[idx0].Amount) {
		t.Errorf("subcategory prompt missing row %d's amount: %q", idx0, subPrompt)
	}
	if !strings.Contains(subPrompt, "Mascotas") {
		t.Errorf("subcategory prompt missing chosen category: %q", subPrompt)
	}

	// Advance to row 1 — this is the exact failure mode from the bug report:
	// two consecutive category prompts that read identically.
	data[keyPendingCategoryGaps] = encodeStringSlice(gaps[1:])
	prompt1 := msgAskCategory(data)
	if prompt1 == prompt0 {
		t.Fatalf("row 1's category prompt is identical to row 0's — this is the reported bug")
	}
	idx1, _ := strconv.Atoi(gaps[1])
	if !strings.Contains(prompt1, rows[idx1].Amount) {
		t.Errorf("category prompt for row 1 missing its own amount %q: %q", rows[idx1].Amount, prompt1)
	}
	if !strings.Contains(prompt1, "2 de 2") {
		t.Errorf("category prompt for row 1 missing position counter: %q", prompt1)
	}
}
