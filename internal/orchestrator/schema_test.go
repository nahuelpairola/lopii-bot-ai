package orchestrator

import (
	"encoding/json"
	"testing"
)

// TestToolSchemas_OptionalFieldsAllowNull locks the invariant that every
// property NOT listed in its object's `required` must accept JSON null in its
// `type` (a union containing "null"). Groq validates the model's tool-call
// against the schema we send; a bare-scalar optional 400s the moment the model
// emits null for it (as gpt-oss-20b does for an absent merchant). This test
// fails on any schema — present or future — that forgets the null-union.
func TestToolSchemas_OptionalFieldsAllowNull(t *testing.T) {
	// Las de un solo tiro. createTool y deleteTool salieron con la etapa 5.
	params := map[string]json.RawMessage{
		"updateTool":         updateTool.Parameters,
		"onboardingTool":     onboardingTool.Parameters,
		"accountManageTool":  accountManageTool.Parameters,
		"categoryCreateTool": categoryCreateTool.Parameters,
		"classifierTool":     classifierTool.Parameters,
	}
	// Y TODAS las del loop, que antes no se revisaban. Es donde más duele: el
	// 2026-08-12 un campo mal declarado en correct_movement devolvió un 400 duro
	// y se comió el turno entero, sin que la app llegara a ver nada.
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

// walkOptionalScalars returns every property, at any depth, that is not in its
// object's `required` yet whose `type` does not permit null. Recurses into
// nested objects and array `items`.
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
		// Recurse: nested object, or array of objects.
		if items, ok := p["items"].(map[string]any); ok {
			out = append(out, walkOptionalScalars(items, child+"[]")...)
		}
		if _, ok := p["properties"]; ok {
			out = append(out, walkOptionalScalars(p, child)...)
		}
	}
	return out
}

// typeAllowsNull reports whether a JSON-schema `type` value permits null: a
// union array containing "null". A bare string type ("string") never does.
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
