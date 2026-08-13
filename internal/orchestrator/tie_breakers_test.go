package orchestrator

import (
	"strings"
	"testing"
)

// TestAgentTieBreakers_SpeakInTools es la mitad que justifica la extracción:
// la misma regla, con los nombres de las tools en vez de los intents.
func TestAgentTieBreakers_SpeakInTools(t *testing.T) {
	got := agentTieBreakers(AgentTools())

	for _, want := range []string{ToolRecordMovements, ToolCorrectMovement, ToolDeleteMovements, ToolManageSettings, ToolReplyHelp} {
		if !strings.Contains(got, want) {
			t.Errorf("los desempates del agente no nombran %s:\n%s", want, got)
		}
	}
	// Ningún nombre de intent debe sobrevivir: el loop no rutea intents.
	for _, leaked := range []string{"CREATE", "UPDATE", "DELETE", "ACCOUNT_MANAGE", "HELP"} {
		if strings.Contains(got, leaked) {
			t.Errorf("quedó el intent %q en los desempates del agente:\n%s", leaked, got)
		}
	}
	// Y la regla que más caro salió en producción sigue textual.
	if !strings.Contains(got, "era, eran, fue") {
		t.Errorf("se perdió el copulativo en pasado:\n%s", got)
	}
}

// TestAgentTieBreakers_DropsRulesAboutToolsThatAreNotSent: durante las etapas 2
// y 3 el toolbox es un subconjunto. Una regla que nombra una tool que no viaja
// en el request le pide al modelo que llame algo que no existe.
func TestAgentTieBreakers_DropsRulesAboutToolsThatAreNotSent(t *testing.T) {
	stage2 := []AgentTool{
		{Name: ToolCorrectMovement}, {Name: ToolDeleteMovements},
		{Name: ToolReplyHelp}, {Name: ToolAskRewrite},
	}
	got := agentTieBreakers(stage2)

	for _, absent := range []string{ToolRecordMovements, ToolManageSettings, ToolSumMovements} {
		if strings.Contains(got, absent) {
			t.Errorf("sobrevivió una regla sobre %s, que no se manda:\n%s", absent, got)
		}
	}
	// La que importa para una corrección sigue viva: no nombra ninguna ausente.
	if !strings.Contains(got, "era, eran, fue") {
		t.Errorf("se cayó el copulativo en pasado, que es el desempate de UPDATE:\n%s", got)
	}
}

// TestTieBreakers_CorrectionWithNoNewValue: "el café estaba mal" nombra el
// movimiento y no trae monto. Ruteaba UNCLEAR (intent_events 115/116, 132/133)
// porque el único desempate de corrección exigía un monto. La regla tiene que
// estar en los DOS bloques: el router decide qué llega al loop, y el loop decide
// qué tool corre.
func TestTieBreakers_CorrectionWithNoNewValue(t *testing.T) {
	if block := agentTieBreakers(AgentTools()); !strings.Contains(block, "sin decir el valor nuevo") {
		t.Errorf("falta la regla de corrección sin valor nuevo: %s", block)
	}
}

// TestTieBreakers_NoPlaceholderSurvives: un marcador sin reemplazar llegaría al
// modelo como "{{record}}" y sería basura silenciosa en el prompt.
func TestTieBreakers_NoPlaceholderSurvives(t *testing.T) {
	for name, block := range map[string]string{"agent": agentTieBreakers(AgentTools())} {
		if strings.Contains(block, "{{") || strings.Contains(block, "}}") {
			t.Errorf("quedó un marcador sin reemplazar en el bloque %s:\n%s", name, block)
		}
	}
}
