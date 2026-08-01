package orchestrator

import (
	"encoding/json"
	"testing"
)

func TestAgentTools_AllWellFormed(t *testing.T) {
	tools := AgentTools()
	if len(tools) != 14 {
		t.Fatalf("%d tools, want 14 — las 15 de la §4.10 menos find_movements_to_correct, que costaba una vuelta entera del loop y no compraba nada (ver AgentTools)", len(tools))
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

// TestAgentTools_RecordMovementsKeepsCreateToolSchema is the guard that this
// stage changed no behaviour: record_movements must carry createTool's schema
// byte for byte. A "tidied" schema is a behaviour change on the money path.
func TestAgentTools_RecordMovementsKeepsCreateToolSchema(t *testing.T) {
	var fromAgent, fromCreate any
	for _, tool := range AgentTools() {
		if tool.Name == ToolRecordMovements {
			if err := json.Unmarshal(tool.Parameters, &fromAgent); err != nil {
				t.Fatalf("record_movements schema: %v", err)
			}
		}
	}
	if err := json.Unmarshal(createTool.Parameters, &fromCreate); err != nil {
		t.Fatalf("createTool schema: %v", err)
	}

	a, _ := json.Marshal(fromAgent)
	c, _ := json.Marshal(fromCreate)
	if string(a) != string(c) {
		t.Errorf("record_movements schema drifted from createTool:\n agent  = %s\n create = %s", a, c)
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
