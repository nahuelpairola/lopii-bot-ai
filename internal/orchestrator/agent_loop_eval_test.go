//go:build llm_eval

package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

type agentEvalCase struct {
	id        string
	msg       string
	history   []QueryTurn
	wantFirst string
	forbidden string
	why       string
}

var agentEvalCases = []agentEvalCase{
	{
		id: "le_erre_eran_1500", msg: "Le erre eran 1500",
		wantFirst: ToolCorrectMovement, forbidden: ToolRecordMovements,
		why: "corrección sin referente explícito: hay que buscar, no registrar",
	},
	{
		id: "panaderia_era_2k", msg: "La panaderia era 2k",
		wantFirst: ToolCorrectMovement, forbidden: ToolRecordMovements,
		why: "el caso del acento, ya arreglado en f0; acá se mide la elección de tool",
	},
	{
		id: "corregir_ultimo", msg: "Corregir monto último movimiento",
		wantFirst: ToolCorrectMovement, forbidden: ToolRecordMovements,
		why: "id 203: hoy contesta 'no tengo movimientos de ese día', que es falso",
	},
	{
		id: "asado_eran_15mil", msg: "Perdon, el asado eran 15 mil",
		wantFirst: ToolCorrectMovement, forbidden: ToolRecordMovements,
		why: "corrección con monto nuevo y referente textual",
	},

	{
		id: "modificar_cuenta_wallet", msg: "Quiero modificar el monto de la cuenta Wallet ARS",
		wantFirst: ToolManageAccount, forbidden: ToolCorrectMovement,
		why: "es una CUENTA, no un movimiento",
	},
	{
		id: "actualizar_mercado", msg: "Actualizar monto de mercado a $891867.82",
		wantFirst: ToolManageAccount, forbidden: ToolRecordMovements,
		why: "no dice 'cuenta' en ningún lado; hoy ni el router ni redirectTargetFor lo agarran",
	},
	{
		id: "renombrar_balanz", msg: "Quiero cambiar el nombre del Fondo común de inversión Balanz por FCI",
		wantFirst: ToolManageAccount, forbidden: ToolRecordMovements,
		why: "renombrar una cuenta, sin la palabra 'cuenta'",
	},

	{
		id:  "cafe_era_en_el_bar",
		msg: "El café era en el bar",
		history: []QueryTurn{
			{Question: "3 mil café", Answer: "Registré: ☕ Ocio y salidas › Salir a comer — $3.000 · café · Mercado Pago (hoy)"},
		},
		wantFirst: ToolCorrectMovement, forbidden: ToolRecordMovements,
		why: "id 210 registró un duplicado — peor que no hacer nada. Con historial es trivial",
	},
	{
		id:  "es_cafe_en_bar",
		msg: "Es café en bar",
		history: []QueryTurn{
			{Question: "3 mil café", Answer: "Registré: ☕ Ocio y salidas › Salir a comer — $3.000 · café · Mercado Pago (hoy)"},
		},
		wantFirst: ToolCorrectMovement, forbidden: ToolRecordMovements,
		why: "id 209: hoy cae en UNCLEAR",
	},

	{
		id: "control_gasto_simple", msg: "gasté 500 en el súper",
		wantFirst: ToolRecordMovements, forbidden: ToolCorrectMovement,
		why: "el camino del 74%: no puede volverse una corrección",
	},
	{
		id: "control_query", msg: "cuánto gasté esta semana",
		wantFirst: ToolSumMovements, forbidden: ToolRecordMovements,
		why: "una consulta no puede registrar nada",
	},
}

func evalTools() []AgentTool {
	want := map[string]bool{
		ToolRecordMovements: true,
		ToolCorrectMovement: true,
		ToolDeleteMovements: true,
		ToolReplyHelp:       true,
		ToolAskRewrite:      true,
		ToolSumMovements:    true,
	}
	var out []AgentTool
	for _, t := range AgentTools() {
		if want[t.Name] {
			out = append(out, t)
		}
	}
	return out
}

const fakeCandidates = `[{"transaction_id":"tx-1","fecha":"hoy","detalle":"café · $3.000 · Ocio y salidas | Salir a comer"}]`

func TestAgentLoopEval(t *testing.T) {
	key := evalKey(t)

	_, taxonomy := seededTaxonomy(t)
	accounts := []AccountOption{
		{ID: 1, Name: "Mercado Pago", Currency: "ARS"},
		{ID: 2, Name: "Wallet ARS", Currency: "ARS"},
		{ID: 3, Name: "Fondo común de inversión Balanz", Currency: "ARS"},
	}
	tools := evalTools()
	prompt := BuildAgentPrompt("2026-07-31", accounts, taxonomy, "", tools, "")
	t.Logf("prompt del eval: %d runas (~%d tokens estimados) · %d tools (de %d definidas)",
		len([]rune(prompt)), len([]rune(prompt))/4, len(tools), len(AgentTools()))

	model := os.Getenv("GROQ_AGENT_MODEL")
	if model == "" {
		model = "openai/gpt-oss-20b"
	}
	o := New(Config{
		APIKey:         key,
		BaseURL:        evalBaseURL(),
		AgentModel:     model,
		TimeoutSeconds: 60,
	})

	ran := 0
	for _, tc := range agentEvalCases {
		t.Run(tc.id, func(t *testing.T) {
			if ran > 0 {
				time.Sleep(62 * time.Second)
			}
			ran++
			var called []string
			execute := func(name string, _ json.RawMessage) (string, error) {
				called = append(called, name)
				switch name {
				case ToolFindMovementsToCorrect:
					return fakeCandidates, nil
				case ToolRecordMovements:
					return "registrados: 1 movimiento", nil
				case ToolSumMovements, ToolListMovements, ToolAccountBalance:
					return `{"total":"192025","currency":"ARS"}`, nil
				case ToolListCategories:
					return "Ocio y salidas | Salir a comer", nil
				default:
					return "pendiente: la app se encarga", nil
				}
			}

			narration, err := o.Run(context.Background(), prompt, tc.msg, tc.history, tools, execute)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if len(called) == 0 {
				t.Fatalf("no llamó ninguna tool — tool_choice:required debería impedirlo. Narró: %q", narration)
			}

			t.Logf("tools: %s | narración: %s", strings.Join(called, " → "), truncate(narration))

			if called[0] != tc.wantFirst {
				t.Errorf("primera tool = %s, quería %s (%s)", called[0], tc.wantFirst, tc.why)
			}
			for _, c := range called {
				if c == tc.forbidden {
					t.Errorf("llamó %s, que es justo lo que no debe (%s)", tc.forbidden, tc.why)
				}
			}
		})
	}
}

func TestAgentDateAnchorEval(t *testing.T) {
	key := evalKey(t)

	_, taxonomy := seededTaxonomy(t)
	accounts := []AccountOption{{ID: 1, Name: "Mercado Pago", Currency: "ARS"}}
	tools := evalTools()
	const hoy = "miércoles 2026-08-19"

	model := os.Getenv("GROQ_AGENT_MODEL")
	if model == "" {
		model = "openai/gpt-oss-20b"
	}
	o := New(Config{APIKey: key, BaseURL: evalBaseURL(), AgentModel: model, TimeoutSeconds: 60})

	cases := []struct {
		id         string
		msg        string
		today      string
		wantNoDate bool
		wantFrom   string
	}{
		{id: "el_lunes_no_lleva_fecha", today: hoy, msg: "La compra de locro del lunes ponela en salidas restaurante", wantNoDate: true},
		{id: "la_semana_pasada_no_lleva_fecha", today: hoy, msg: "El gasto de la semana pasada ponelo en otra categoría", wantNoDate: true},
		{id: "el_4_de_agosto_si_lleva_fecha", today: hoy, msg: "El débito del 4 de agosto ponelo en Servicios", wantFrom: "2026-08-04"},
	}

	for i, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			if i > 0 {
				time.Sleep(62 * time.Second)
			}
			var args struct {
				DateFrom string `json:"date_from"`
				DateTo   string `json:"date_to"`
			}
			prompt := BuildAgentPrompt(tc.today, accounts, taxonomy, "", tools, "")
			var seen bool
			execute := func(name string, raw json.RawMessage) (string, error) {
				if name == ToolCorrectMovement {
					seen = true
					if err := json.Unmarshal(raw, &args); err != nil {
						t.Errorf("args ilegibles: %v — %s", err, raw)
					}
					t.Logf("args: %s", raw)
				}
				return "pendiente: la app se encarga", nil
			}

			_, err := o.Run(context.Background(), prompt, tc.msg, nil, tools, execute)
			if !seen {
				t.Fatalf("no llamó %s (err=%v)", ToolCorrectMovement, err)
			}

			if tc.wantNoDate {
				if args.DateFrom != "" || args.DateTo != "" {
					t.Errorf("una referencia relativa no lleva fecha, y vino date_from=%q date_to=%q — el modelo la calcula mal y manda la búsqueda a otra ventana", args.DateFrom, args.DateTo)
				}
				return
			}
			if args.DateFrom != tc.wantFrom {
				t.Errorf("date_from = %q, quería %q", args.DateFrom, tc.wantFrom)
			}
		})
	}
}

func TestAgentPromptSize(t *testing.T) {
	accounts := []AccountOption{{ID: 1, Name: "Mercado Pago", Currency: "ARS"}}
	tools := AgentTools()
	unified := BuildAgentPrompt("2026-07-31", accounts, nil, "", tools, "")

	bloques := []struct {
		nombre string
		texto  string
	}{
		{"tools (cuándo usar)", buildToolsBlock(tools)},
		{"desempates", agentTieBreakers(tools)},
		{"reglas de monto", amountRules},
		{"patrones de movimiento", agentPatternRules},
		{"cuentas", buildAccountsBlock(accounts)},
	}

	total := len([]rune(unified))
	var sumado int
	t.Logf("prompt del agente: %d runas", total)
	for _, b := range bloques {
		n := len([]rune(b.texto))
		sumado += n
		t.Logf("  %-24s %5d runas (%4.1f%%)", b.nombre, n, 100*float64(n)/float64(total))
	}
	t.Logf("  %-24s %5d runas (%4.1f%%) <- prosa fija de la plantilla",
		"resto", total-sumado, 100*float64(total-sumado)/float64(total))

	var schemas int
	for _, tool := range tools {
		n := len([]rune(tool.Description)) + len(tool.Parameters)
		schemas += n
		t.Logf("  schema %-17s %5d runas", tool.Name, n)
	}
	t.Logf("TOTAL por request: %d runas (~%d tokens a 4 runas/token)",
		total+schemas, (total+schemas)/4)
}

func truncate(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len([]rune(s)) > 60 {
		return string([]rune(s)[:57]) + "..."
	}
	return s
}
