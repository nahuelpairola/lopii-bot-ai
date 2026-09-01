package orchestrator

import (
	"strings"
	"testing"
)

func TestBuildAgentPrompt_CarriesTheLoadBearingRules(t *testing.T) {
	prompt := BuildAgentPrompt("2026-07-31",
		[]AccountOption{{ID: 1, Name: "Mercado Pago", Currency: "ARS"}},
		[]TaxonomyEntry{{Category: "Comida", Subcategory: "Supermercado"}},
		"", AgentTools(), "")

	for _, want := range []string{
		"2026-07-31",
		"Mercado Pago",
		"el destino decide el tipo",
		"van en POSITIVO",
		"MISMO group",
		"hora de Argentina",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}

	for _, gone := range []string{
		"Comida | Supermercado",
		"TAXONOMÍA DISPONIBLE",
		"PENDING_REVIEW",
		"Sistema | Transferencia",
		"Inversiones | Dólares",
		"Rendimiento inversión",
	} {
		if strings.Contains(prompt, gone) {
			t.Errorf("el prompt del loop todavía lleva %q: clasificar salió del loop", gone)
		}
	}
}

func TestBuildAgentPrompt_TellsTheModelWhenToUseEachTool(t *testing.T) {
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

func TestBuildAgentPrompt_DropsTheNeverAskRule(t *testing.T) {
	prompt := BuildAgentPrompt("2026-07-31", nil, nil, "", AgentTools(), "")
	if strings.Contains(prompt, "Nunca hagas una pregunta de aclaración") {
		t.Error("the read-only loop's no-questions rule must NOT survive: ask_user exists now")
	}
}
