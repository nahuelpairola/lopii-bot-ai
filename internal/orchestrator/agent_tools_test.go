package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAgentTools_AllWellFormed(t *testing.T) {
	tools := AgentTools()
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

	if kinds[KindWrite] != 1 {
		t.Errorf("%d write tools, want exactly 1 (record_movements)", kinds[KindWrite])
	}
	if !seen[ToolRecordMovements] {
		t.Errorf("%s missing", ToolRecordMovements)
	}
	if !seen[ToolReplyHelp] || !seen[ToolAskRewrite] {
		t.Error("reply_help and ask_rewrite must exist: without them a message with nothing to do cannot satisfy tool_choice:required")
	}
}

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
	for _, want := range []string{"type", "amount", "currency", "date", "description"} {
		if _, ok := props[want]; !ok {
			t.Errorf("record_movements perdio %q, que es parte del hecho economico", want)
		}
	}
}

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

func TestCorrectMovement_WhenAnnouncesEveryFieldItAccepts(t *testing.T) {
	var when, params string
	for _, tool := range AgentTools() {
		if tool.Name == ToolCorrectMovement {
			when, params = tool.When, string(tool.Parameters)
		}
	}
	if when == "" {
		t.Fatal("correct_movement no está en el toolbox")
	}
	for campo, palabra := range map[string]string{
		"category": "categoría",
		"account":  "cuenta",
		"date":     "fecha",
	} {
		if !strings.Contains(params, `"`+campo+`"`) {
			t.Errorf("el schema perdió el campo %q", campo)
		}
		if !strings.Contains(strings.ToLower(when), palabra) {
			t.Errorf("el When no menciona %q: el modelo no va a mapearle ese pedido", palabra)
		}
	}
}

func TestAgentTools_CorrectMovementTakesADateLocator(t *testing.T) {
	var tool AgentTool
	for _, tl := range AgentTools() {
		if tl.Name == ToolCorrectMovement {
			tool = tl
		}
	}
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
		t.Fatalf("schema ilegible: %v", err)
	}
	for _, field := range []string{"date_from", "date_to"} {
		p, ok := schema.Properties[field]
		if !ok {
			t.Fatalf("%s no está declarado en el schema", field)
		}
		if p.Description == "" {
			t.Errorf("%s sin descripción: el modelo no puede saber cuándo usarlo", field)
		}
	}
	for _, r := range schema.Required {
		if r == "date_from" || r == "date_to" {
			t.Errorf("%s no puede ser required", r)
		}
	}
}
