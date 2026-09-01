package orchestrator

import (
	"encoding/json"
	"testing"
)

func TestToolSchemas_OptionalFieldsAllowNull(t *testing.T) {
	params := map[string]json.RawMessage{
		"updateTool":         updateTool.Parameters,
		"onboardingTool":     onboardingTool.Parameters,
		"accountManageTool":  accountManageTool.Parameters,
		"categoryCreateTool": categoryCreateTool.Parameters,
		"classifierTool":     classifierTool.Parameters,
	}
	for _, tool := range AgentTools() {
		params["agent:"+tool.Name] = tool.Parameters
	}

	for name, raw := range params {
		var schema map[string]any
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("%s: parameters not valid JSON: %v", name, err)
		}
		for _, v := range walkOptionalScalars(schema, name) {
			t.Errorf("%s: optional field %q has type %v — must include \"null\" or the model's null 400s", name, v.path, v.typ)
		}
	}
}

type nullViolation struct {
	path string
	typ  any
}

func walkOptionalScalars(schema map[string]any, path string) []nullViolation {
	var out []nullViolation

	required := map[string]bool{}
	if req, ok := schema["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				required[s] = true
			}
		}
	}

	props, _ := schema["properties"].(map[string]any)
	for pname, praw := range props {
		p, ok := praw.(map[string]any)
		if !ok {
			continue
		}
		child := path + "." + pname
		if !required[pname] && !typeAllowsNull(p["type"]) {
			out = append(out, nullViolation{path: child, typ: p["type"]})
		}
		if items, ok := p["items"].(map[string]any); ok {
			out = append(out, walkOptionalScalars(items, child+"[]")...)
		}
		if _, ok := p["properties"]; ok {
			out = append(out, walkOptionalScalars(p, child)...)
		}
	}
	return out
}

func typeAllowsNull(t any) bool {
	arr, ok := t.([]any)
	if !ok {
		return false
	}
	for _, v := range arr {
		if s, ok := v.(string); ok && s == "null" {
			return true
		}
	}
	return false
}
