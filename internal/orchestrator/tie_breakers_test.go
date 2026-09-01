package orchestrator

import (
	"strings"
	"testing"
)

func TestAgentTieBreakers_SpeakInTools(t *testing.T) {
	got := agentTieBreakers(AgentTools())

	for _, want := range []string{ToolRecordMovements, ToolCorrectMovement, ToolDeleteMovements, ToolManageSettings, ToolReplyHelp} {
		if !strings.Contains(got, want) {
			t.Errorf("los desempates del agente no nombran %s:\n%s", want, got)
		}
	}
	for _, leaked := range []string{"CREATE", "UPDATE", "DELETE", "ACCOUNT_MANAGE", "HELP"} {
		if strings.Contains(got, leaked) {
			t.Errorf("quedó el intent %q en los desempates del agente:\n%s", leaked, got)
		}
	}
	if !strings.Contains(got, "era, eran, fue") {
		t.Errorf("se perdió el copulativo en pasado:\n%s", got)
	}
}

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
	if !strings.Contains(got, "era, eran, fue") {
		t.Errorf("se cayó el copulativo en pasado, que es el desempate de UPDATE:\n%s", got)
	}
}

func TestTieBreakers_CorrectionWithNoNewValue(t *testing.T) {
	if block := agentTieBreakers(AgentTools()); !strings.Contains(block, "sin decir el valor nuevo") {
		t.Errorf("falta la regla de corrección sin valor nuevo: %s", block)
	}
}

func TestTieBreakers_NoPlaceholderSurvives(t *testing.T) {
	for name, block := range map[string]string{"agent": agentTieBreakers(AgentTools())} {
		if strings.Contains(block, "{{") || strings.Contains(block, "}}") {
			t.Errorf("quedó un marcador sin reemplazar en el bloque %s:\n%s", name, block)
		}
	}
}
