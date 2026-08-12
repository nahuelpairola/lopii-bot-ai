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
		"", AgentTools(), "")

	for _, want := range []string{
		"2026-07-31",   // the date rule's anchor
		"Mercado Pago", // the accounts block
		"el destino decide el tipo",
		"van en POSITIVO",
		"MISMO group",
		"America/Argentina/Buenos_Aires",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}

	// Y lo que YA NO puede estar. La taxonomía se pagaba en CADA ronda del loop
	// —incluso en "¿cuánto gasté en julio?", donde no hay nada que clasificar— y
	// el loop resiente cada token: medido el 2026-08-12, un turno pide ~7.000
	// contra un techo de 8.000 TPM.
	//
	// Los nombres literales de subcategoría tampoco: estaban ahí para que el
	// modelo los COPIE, y por lo tanto también para que los copie mal. Ahora los
	// pone la app desde la forma del movimiento.
	for _, gone := range []string{
		"Comida | Supermercado", // el bloque de taxonomía
		"TAXONOMÍA DISPONIBLE",
		"PENDING_REVIEW",          // la regla de taxonomía
		"Sistema | Transferencia", // los pares literales de los patrones
		"Inversiones | Dólares",
		// La REGLA DE GANANCIA entera: su único contenido era asignar este par, y
		// `record_movements` ya no tiene campo de categoría. Medido antes de
		// sacarla: 0 usos en 442 mensajes de 45 días, ~85 tokens por llamada.
		// Que un rendimiento es un income sale solo de la regla de tipo; el par lo
		// pone el clasificador, que es el único que puede — no se deduce de la
		// forma (un ingreso a una cuenta de inversión y un depósito son idénticos).
		"Rendimiento inversión",
	} {
		if strings.Contains(prompt, gone) {
			t.Errorf("el prompt del loop todavía lleva %q: clasificar salió del loop", gone)
		}
	}
}

func TestBuildAgentPrompt_TellsTheModelWhenToUseEachTool(t *testing.T) {
	// The router's tie-breakers survive as tool-selection rules: the router
	// used to pick an intent, the model now picks a tool.
	prompt := BuildAgentPrompt("2026-07-31", nil, nil, "", AgentTools(), "")
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
	// Los desempates más caros de ganar, heredados del router que ya no existe.
	// Hablan en tools, no en intents: manage_settings, no ACCOUNT_MANAGE.
	for _, want := range []string{"era, eran, fue", ToolManageSettings, "sin respaldo"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing the tie-breaker %q", want)
		}
	}
}

func TestBuildAgentPrompt_CarriesTheFourNewRules(t *testing.T) {
	prompt := BuildAgentPrompt("2026-07-31", nil, nil, "", AgentTools(), "")
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
	prompt := BuildAgentPrompt("2026-07-31", nil, nil, "¿A qué cuenta corresponde el gasto de $20.000?", AgentTools(), "")
	if !strings.Contains(prompt, "¿A qué cuenta corresponde el gasto de $20.000?") {
		t.Error("the pending question must be stated in the prompt")
	}
}

func TestBuildAgentPrompt_OmitsThePendingSectionWhenNothingIsOpen(t *testing.T) {
	prompt := BuildAgentPrompt("2026-07-31", nil, nil, "", AgentTools(), "")
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
	prompt := BuildAgentPrompt("2026-07-31", nil, nil, "", AgentTools(), "")
	if strings.Contains(prompt, "Nunca hagas una pregunta de aclaración") {
		t.Error("the read-only loop's no-questions rule must NOT survive: ask_user exists now")
	}
}
