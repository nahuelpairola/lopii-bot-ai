package query

import (
	"encoding/json"
	"reflect"
	"testing"

	"lopiibot.com/internal/orchestrator"
)

// Los schemas de las read tools están copiados A MANO en dos archivos:
// internal/query/query.go (Tools, lo que consume AnswerQuery) y
// orchestrator/agent_tools.go (el toolbox del agente). Sólo un comentario los
// mantenía sincronizados.
//
// Si se cambia uno y se olvida el otro, la falla es SILENCIOSA y ancha: el
// agente le ofrece al modelo un parámetro que el ejecutor ya no parsea, el
// filtro no se aplica, y la consulta contesta sobre todo el rango. No hay 400,
// no hay log, y ningún otro test lo ve. Es el mismo modo de falla que A3
// arregló para la cuenta inexistente —número grande, plausible, sin señal—
// pero por otra puerta.
//
// Se comparan sólo los Parameters: el AgentTool del agente además lleva When y
// Kind, que query no usa y no tiene por qué llevar.
func TestQueryTools_ParametersMatchAgentTools(t *testing.T) {
	agent := map[string]json.RawMessage{}
	for _, tool := range orchestrator.AgentTools() {
		agent[tool.Name] = tool.Parameters
	}

	checked := 0
	for _, tool := range Tools {
		other, ok := agent[tool.Name]
		if !ok {
			continue // no toda tool de query vive también en el toolbox del agente
		}
		var mine, theirs any
		if err := json.Unmarshal(tool.Parameters, &mine); err != nil {
			t.Fatalf("%s: el schema de query no parsea: %v", tool.Name, err)
		}
		if err := json.Unmarshal(other, &theirs); err != nil {
			t.Fatalf("%s: el schema de orchestrator no parsea: %v", tool.Name, err)
		}
		if !reflect.DeepEqual(mine, theirs) {
			t.Errorf("%s: los dos schemas divergieron.\ninternal/query/query.go:\n%s\n\norchestrator/agent_tools.go:\n%s",
				tool.Name, tool.Parameters, other)
		}
		checked++
	}

	// Sin esto el test se vuelve verde por vacío el día que alguien renombre una
	// tool en un archivo y no en el otro: el mapa no matchea, el for no compara
	// nada, y el test "pasa".
	if checked < 4 {
		t.Fatalf("se compararon %d tools; se esperaban al menos 4 compartidas (list_categories, sum_movements, list_movements, account_balance)", checked)
	}
}
