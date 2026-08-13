package orchestrator

import (
	"encoding/json"
	"testing"
)

func TestAgentTools_AllWellFormed(t *testing.T) {
	tools := AgentTools()
	// 12 desde la etapa 5: las cinco de configuración colapsaron en
	// manage_settings, y answer_query se sumó. Cinco tools casi iguales que
	// hacían lo mismo —parkear a un wizard— eran justo donde este modelo elige
	// mal.
	if len(tools) != 12 {
		t.Fatalf("%d tools, want 12", len(tools))
	}

	seen := make(map[string]bool, len(tools))
	kinds := map[AgentToolKind]int{}
	for _, tool := range tools {
		if tool.Name == "" {
			t.Error("a tool has no name")
		}
		if seen[tool.Name] {
			t.Errorf("duplicate tool name %q", tool.Name)
		}
		seen[tool.Name] = true

		if tool.Description == "" {
			t.Errorf("%s has no description — the model picks tools by it", tool.Name)
		}
		if tool.Kind != KindRead && tool.Kind != KindWrite && tool.Kind != KindAction {
			t.Errorf("%s has kind %q, want read/write/action", tool.Name, tool.Kind)
		}
		kinds[tool.Kind]++

		var schema map[string]any
		if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
			t.Errorf("%s parameters are not valid JSON: %v", tool.Name, err)
			continue
		}
		if schema["type"] != "object" {
			t.Errorf("%s schema type = %v, want object", tool.Name, schema["type"])
		}
	}

	// Exactly one write exists, and it is the one that inserts money.
	if kinds[KindWrite] != 1 {
		t.Errorf("%d write tools, want exactly 1 (record_movements)", kinds[KindWrite])
	}
	if !seen[ToolRecordMovements] {
		t.Errorf("%s missing", ToolRecordMovements)
	}
	// The two that keep tool_choice:"required" viable on round 0.
	if !seen[ToolReplyHelp] || !seen[ToolAskRewrite] {
		t.Error("reply_help and ask_rewrite must exist: without them a message with nothing to do cannot satisfy tool_choice:required")
	}
}

// TestAgentTools_RecordMovementsNeverAsksForTheCategoryPair
//
// Este test comparaba `record_movements` contra `createTool` campo por campo,
// para atajar la deriva accidental entre los dos caminos de CREATE. Ya no hay
// dos: la etapa 5 borró create.go, que era lo que el comentario anterior
// anunciaba ("el camino viejo SÍ, hasta que la etapa 5 lo borre").
//
// Lo que sobrevive es la mitad que sigue teniendo sujeto: el loop NO puede
// pedir la categoría. Si volviera a aparecer en el schema, el modelo la
// llenaría —tiene con qué— y estaríamos pagando dos veces por clasificar, una
// de ellas con el modelo equivocado y sin la taxonomía delante.
func TestAgentTools_RecordMovementsNeverAsksForTheCategoryPair(t *testing.T) {
	var agentParams json.RawMessage
	for _, tool := range AgentTools() {
		if tool.Name == ToolRecordMovements {
			agentParams = tool.Parameters
		}
	}
	var parsed map[string]any
	if err := json.Unmarshal(agentParams, &parsed); err != nil {
		t.Fatalf("record_movements schema: %v", err)
	}
	movements := parsed["properties"].(map[string]any)["movements"].(map[string]any)
	props := movements["items"].(map[string]any)["properties"].(map[string]any)

	for _, gone := range []string{"category", "subcategory"} {
		if _, still := props[gone]; still {
			t.Errorf("record_movements todavia pide %q: eso lo decide el clasificador", gone)
		}
	}
	// Y lo que SÍ tiene que seguir pidiendo, que es el hecho económico.
	for _, want := range []string{"type", "amount", "currency", "date", "description"} {
		if _, ok := props[want]; !ok {
			t.Errorf("record_movements perdio %q, que es parte del hecho economico", want)
		}
	}
}

// TestAgentTools_ReadToolsKeepTheirQuerySchemas pins the five read tools to the
// argument shapes their executors already parse (queryToolArgs in
// messaging/query.go). The nullable unions are deliberate: tool-calling models
// emit explicit null for arguments they do not set, and Groq validates
// server-side, so a plain "string" 400s before the executor ever runs.
func TestAgentTools_ReadToolsKeepTheirQuerySchemas(t *testing.T) {
	required := map[string][]string{
		ToolSumMovements:  {"from", "to", "currency"},
		ToolListMovements: {"from", "to", "currency"},
	}

	for _, tool := range AgentTools() {
		want, checked := required[tool.Name]
		if !checked {
			continue
		}
		var schema struct {
			Required []string `json:"required"`
		}
		if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
			t.Fatalf("%s: %v", tool.Name, err)
		}
		if len(schema.Required) != len(want) {
			t.Errorf("%s required = %v, want %v", tool.Name, schema.Required, want)
			continue
		}
		for i, r := range want {
			if schema.Required[i] != r {
				t.Errorf("%s required = %v, want %v", tool.Name, schema.Required, want)
				break
			}
		}
	}
}
