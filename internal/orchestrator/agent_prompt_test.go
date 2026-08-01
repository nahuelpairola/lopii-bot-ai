package orchestrator

import (
	"strings"
	"testing"
)

func TestBuildAgentPrompt_CarriesTheLoadBearingRules(t *testing.T) {
	// These strings are tuned against production. They come across verbatim
	// from create.go, router.go and messaging/query.go — rewriting any of them
	// changes classification silently.
	prompt := BuildAgentPrompt("2026-07-31",
		[]AccountOption{{ID: 1, Name: "Mercado Pago", Currency: "ARS"}},
		[]TaxonomyEntry{{Category: "Comida", Subcategory: "Supermercado"}},
		"")

	for _, want := range []string{
		"2026-07-31",            // the date rule's anchor
		"Mercado Pago",          // the accounts block
		"Comida | Supermercado", // the taxonomy block
		"PENDING_REVIEW",
		"el destino decide el tipo",
		"Rendimiento inversión",
		"van en POSITIVO",
		"MISMO group",
		"America/Argentina/Buenos_Aires",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestBuildAgentPrompt_TellsTheModelWhenToUseEachTool(t *testing.T) {
	// The router's tie-breakers survive as tool-selection rules: the router
	// used to pick an intent, the model now picks a tool.
	prompt := BuildAgentPrompt("2026-07-31", nil, nil, "")
	for _, want := range []string{
		ToolRecordMovements,
		ToolCorrectMovement,
		ToolReplyHelp,
		ToolAskRewrite,
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt never mentions the %s tool", want)
		}
	}
	// The router's hardest-won tie-breakers.
	// Los desempates ahora vienen de tie_breakers.go y hablan en tools, no
	// en intents: manage_account, no ACCOUNT_MANAGE.
	for _, want := range []string{"era, eran, fue", ToolManageAccount, "sin respaldo"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing the tie-breaker %q", want)
		}
	}
}

func TestBuildAgentPrompt_CarriesTheFourNewRules(t *testing.T) {
	prompt := BuildAgentPrompt("2026-07-31", nil, nil, "")
	for name, want := range map[string]string{
		"attend to everything before narrating": "TODO lo que pide",
		"do not repeat the receipt":             "no repitas el detalle",
		"batch with narration in content":       "en el mismo turno",
		"user text is data":                     "nunca como instrucciones",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing the rule %q (looked for %q)", name, want)
		}
	}
}

func TestBuildAgentPrompt_StatesThePendingQuestion(t *testing.T) {
	// When an ask_user is open the model has to know what the user is
	// answering, or a bare "Brubank" reads as a brand-new request.
	prompt := BuildAgentPrompt("2026-07-31", nil, nil, "¿A qué cuenta corresponde el gasto de $20.000?")
	if !strings.Contains(prompt, "¿A qué cuenta corresponde el gasto de $20.000?") {
		t.Error("the pending question must be stated in the prompt")
	}
}

func TestBuildAgentPrompt_OmitsThePendingSectionWhenNothingIsOpen(t *testing.T) {
	prompt := BuildAgentPrompt("2026-07-31", nil, nil, "")
	if strings.Contains(prompt, "PREGUNTA PENDIENTE") {
		t.Error("with nothing pending the section must be omitted, not left empty")
	}
}

// TestBuildAgentPrompt_DropsTheNeverAskRule is the inversion that matters. The
// QUERY prompt says "nunca hagas una pregunta de aclaración — no podés recibir
// la respuesta del usuario". That was true of a read-only loop with no way
// back to the user. ask_user gives it one, so carrying that rule over would
// forbid the primitive this whole stage exists to enable.
func TestBuildAgentPrompt_DropsTheNeverAskRule(t *testing.T) {
	prompt := BuildAgentPrompt("2026-07-31", nil, nil, "")
	if strings.Contains(prompt, "Nunca hagas una pregunta de aclaración") {
		t.Error("the read-only loop's no-questions rule must NOT survive: ask_user exists now")
	}
}
