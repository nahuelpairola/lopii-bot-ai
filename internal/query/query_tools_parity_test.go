package query

import (
	"encoding/json"
	"reflect"
	"testing"

	"lopiibot.com/internal/orchestrator"
)

func TestQueryTools_ParametersMatchAgentTools(t *testing.T) {
	agent := map[string]json.RawMessage{}
	for _, tool := range orchestrator.AgentTools() {
		agent[tool.Name] = tool.Parameters
	}

	checked := 0
	for _, tool := range Tools {
		other, ok := agent[tool.Name]
		if !ok {
			continue
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

	if checked < 4 {
		t.Fatalf("se compararon %d tools; se esperaban al menos 4 compartidas (list_categories, sum_movements, list_movements, account_balance)", checked)
	}
}
