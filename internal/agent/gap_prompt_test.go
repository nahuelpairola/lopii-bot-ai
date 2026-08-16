package agent

import (
	"strconv"
	"strings"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
)

// Los prompts de gap-fill cuando UN mensaje deja varias filas sin categoría.
//
// El bug reportado: dos filas en PENDING_REVIEW producían dos preguntas
// IDÉNTICAS y sin fila, así que el usuario no sabía por cuál de sus dos
// movimientos le estaban preguntando.
//
// Esto era un eval con Groq real (tag llm_eval): la llamada al modelo servía
// sólo para FABRICAR dos filas en PENDING_REVIEW. Las aserciones siempre fueron
// sobre la copy. Con el resultado escrito a mano el test es gratis, no necesita
// API key, y corre en cada `go test ./...` — que es donde tiene que estar un
// guard de un bug reportado.
func TestGapPrompt_TwoGapsAskAboutDifferentRows(t *testing.T) {
	// Lo que devolvía el modelo con una taxonomía deliberadamente pobre: ni
	// "super" ni "nafta" tenían dónde ir, así que las dos filas caían en
	// PENDING_REVIEW.
	result := orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "expense", Amount: "5000", Currency: "ARS", Description: "super",
			Category: "PENDING_REVIEW", Subcategory: "PENDING_REVIEW", Date: "2026-07-24"},
		{Type: "expense", Amount: "3000", Currency: "ARS", Description: "nafta",
			Category: "PENDING_REVIEW", Subcategory: "PENDING_REVIEW", Date: "2026-07-24"},
	}}

	data := buildCreateSeed(result, nil, nil)
	gaps := conversation.DecodeStringSlice(data, conversation.KeyPendingCategoryGaps)
	if len(gaps) != 2 {
		t.Fatalf("las dos filas tienen que abrir gap, hay %d", len(gaps))
	}
	rows := movement.DecodeMovementRows(data)

	prompt0 := flow.MsgAskCategory(data)
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
	data[conversation.KeyMovements] = movement.EncodeMovementRows(rows)
	subPrompt := flow.MsgAskSubcategory(data)
	if !strings.Contains(subPrompt, rows[idx0].Amount) {
		t.Errorf("subcategory prompt missing row %d's amount: %q", idx0, subPrompt)
	}
	if !strings.Contains(subPrompt, "Mascotas") {
		t.Errorf("subcategory prompt missing chosen category: %q", subPrompt)
	}

	// Advance to row 1 — this is the exact failure mode from the bug report:
	// two consecutive category prompts that read identically.
	data[conversation.KeyPendingCategoryGaps] = conversation.EncodeStringSlice(gaps[1:])
	prompt1 := flow.MsgAskCategory(data)
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
