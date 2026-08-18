package orchestrator

import (
	"encoding/json"
	"strings"
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

// El `When` de correct_movement tiene que anunciar TODO lo que la tool sabe
// hacer. Su schema acepta siete campos, pero durante la etapa 5 la descripción
// sólo hablaba de plata (reemplazo, reintegro, incremento) — así que ante "El
// peaje ponelo en banco galicia" el modelo eligió ask_rewrite y pidió el monto,
// leyendo una RE-UBICACIÓN como un movimiento nuevo. Medido en vivo el
// 2026-08-12, y explica un unclear del 10/08 con el mismo verbo.
//
// Una tool que sabe hacer algo y no lo dice es una tool que no lo hace.
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
	// Los campos que el schema acepta y que NO son el monto: si el schema los
	// toma, el When los tiene que nombrar de alguna forma.
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

// correct_movement lleva un LOCALIZADOR de fecha, y va en el schema y no en el
// prompt: Groq valida los argumentos del lado del servidor, así que un campo que
// el prompt pide y el schema no declara es un campo que el modelo no puede
// mandar.
//
// date_from identifica DE CUÁL movimiento habla el mensaje ("el débito del 4 de
// agosto"). No es un cambio de fecha: si lo que se corrige ES la fecha, eso va
// en changes con field:date. La descripción tiene que decirlo, porque es la
// única confusión posible entre los dos campos.
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
	// Ninguno es obligatorio: la enorme mayoría de las correcciones no nombra
	// fecha, y pedirla obligaría al modelo a inventar uno.
	for _, r := range schema.Required {
		if r == "date_from" || r == "date_to" {
			t.Errorf("%s no puede ser required", r)
		}
	}
}
