package orchestrator

import (
	"strings"
	"testing"
)

// routerTieBreakersBefore es el bloque EXACTO que routerSystemPrompt llevaba
// escrito a mano antes de extraerlo. El router es un camino vivo y su prompt
// está tuneado contra producción: este refactor no puede moverle una coma.
//
// Si este test falla después de tocar tieBreakerRules, la respuesta no es
// actualizar la constante de abajo — es que el cambio altera el ruteo en
// producción y necesita su propio eval.
const routerTieBreakersBefore = `- Monto o ítem solo → CREATE.
- Reintegro/regalo que alude a algo previo → UPDATE.
- Rendimiento de inversión → CREATE.
- No confundir CREATE con DELETE.
- Copulativo en pasado sobre un monto (era, eran, fue) → UPDATE, aunque no diga "en realidad".
- Ajustar/corregir el saldo o monto de una CUENTA → ACCOUNT_MANAGE (UPDATE es solo sobre un movimiento).
- "¿Qué puedo hacer?" / "¿cómo funcionás?" → HELP (no QUERY).`

func TestRouterTieBreakers_TextIsUnchanged(t *testing.T) {
	if got := routerTieBreakers(); got != routerTieBreakersBefore {
		t.Errorf("el bloque de desempates del router cambió.\n--- ahora ---\n%s\n--- antes ---\n%s", got, routerTieBreakersBefore)
	}
}

func TestRouterSystemPrompt_StillCarriesTheTieBreakers(t *testing.T) {
	if !strings.Contains(routerSystemPrompt, routerTieBreakersBefore) {
		t.Errorf("routerSystemPrompt perdió el bloque de desempates:\n%s", routerSystemPrompt)
	}
}

// TestAgentTieBreakers_SpeakInTools es la mitad que justifica la extracción:
// la misma regla, con los nombres de las tools en vez de los intents.
func TestAgentTieBreakers_SpeakInTools(t *testing.T) {
	got := agentTieBreakers(AgentTools())

	for _, want := range []string{ToolRecordMovements, ToolCorrectMovement, ToolDeleteMovements, ToolManageAccount, ToolReplyHelp} {
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

	for _, absent := range []string{ToolRecordMovements, ToolManageAccount, ToolSumMovements} {
		if strings.Contains(got, absent) {
			t.Errorf("sobrevivió una regla sobre %s, que no se manda:\n%s", absent, got)
		}
	}
	// La que importa para una corrección sigue viva: no nombra ninguna ausente.
	if !strings.Contains(got, "era, eran, fue") {
		t.Errorf("se cayó el copulativo en pasado, que es el desempate de UPDATE:\n%s", got)
	}
}

// TestTieBreakers_NoPlaceholderSurvives: un marcador sin reemplazar llegaría al
// modelo como "{{record}}" y sería basura silenciosa en el prompt.
func TestTieBreakers_NoPlaceholderSurvives(t *testing.T) {
	for name, block := range map[string]string{"router": routerTieBreakers(), "agent": agentTieBreakers(AgentTools())} {
		if strings.Contains(block, "{{") || strings.Contains(block, "}}") {
			t.Errorf("quedó un marcador sin reemplazar en el bloque %s:\n%s", name, block)
		}
	}
}
