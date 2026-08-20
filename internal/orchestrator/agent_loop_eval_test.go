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

// This is the cheapest honest answer to "does the unified loop actually fix
// UPDATE?", and it is available BEFORE stage 2 exists.
//
// Run, BuildAgentPrompt and AgentTools all landed in stage 1, and Run takes its
// executor as a closure — so the real prompt and the real 15 tool schemas can be
// driven against real production failures with a FAKE executor returning canned
// results. No database, no ask_user, no parked actions.
//
// What it measures is TOOL SELECTION, which is exactly where UPDATE fails today:
// the router picks the wrong intent, or UpdateResult cannot say "I found it but
// I am missing the amount". It does not measure end-to-end correctness — the
// executor is a stub — and it is not meant to.
//
// The corpus is verbatim from the production export (spec §2.2-2.4). Every one
// of these FAILED for a real user.
//
//	GROQ_APIKEY=... GROQ_BASE_URL=https://api.groq.com/openai/v1 \
//	GROQ_AGENT_MODEL=openai/gpt-oss-20b \
//	go test -tags llm_eval ./internal/orchestrator/ -run TestAgentLoopEval -v -timeout 30m

type agentEvalCase struct {
	id      string
	msg     string
	history []QueryTurn
	// wantFirst is the tool the model must reach for first.
	wantFirst string
	// forbidden is the tool that must NOT be called — usually the one that
	// produced the real-world damage.
	forbidden string
	why       string
}

// NOTA sobre las expectativas, corregidas el 2026-08-12.
//
// Nueve casos esperaban find_movements_to_correct, que es SOLO UNA CONSTANTE:
// nunca estuvo en AgentTools(), asi que jamas se le manda al modelo. Esos
// nueve eran imposibles de pasar por construccion, y nadie lo noto porque el
// eval no corria (sin key, y despues 429 por el techo de TPM). Un suite que no
// puede pasar es peor que no tener suite.
//
// La expectativa correcta es correct_movement, que ademas es el contrato de la
// etapa 5: la app resuelve el candidato ("no busques cual: la app lo busca
// sola"), el modelo solo dice QUE cambio quiere.
var agentEvalCases = []agentEvalCase{
	// --- Class A: weak referent / no new value (9 real failures) ---
	// Cause: UpdateResult carries one bit, Resolved. "I found which one but I
	// am missing the new amount" is not representable. The loop can just ask.
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

	// --- Class B: account messages the router sent to UPDATE (6 real failures) ---
	// Cause: the router cannot see the accounts. The last two do not even
	// contain the word "cuenta", so redirectTargetFor misses them too.
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

	// --- Class C: needs history (sequence B, ids 208-211) ---
	// 210 is the worst outcome in the whole export: it RECORDED a duplicate.
	// Without history the message is unintelligible; with history it is trivial.
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

	// --- Control: a plain CREATE must stay a plain CREATE ---
	// The 74% path. If the loop starts second-guessing it, the whole thing is
	// a regression no matter what it fixes.
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

// evalTools es el subconjunto que el eval manda: las que el ejecutor cablea
// hoy, mas las dos de lectura que los casos necesitan para elegir bien.
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

// fakeCandidates is what find_movements_to_correct returns in this eval: one
// plausible recent movement, enough for the model to proceed to correct_movement
// without a real DB.
const fakeCandidates = `[{"transaction_id":"tx-1","fecha":"hoy","detalle":"café · $3.000 · Ocio y salidas | Salir a comer"}]`

func TestAgentLoopEval(t *testing.T) {
	key := evalKey(t)

	_, taxonomy := seededTaxonomy(t)
	accounts := []AccountOption{
		{ID: 1, Name: "Mercado Pago", Currency: "ARS"},
		{ID: 2, Name: "Wallet ARS", Currency: "ARS"},
		{ID: 3, Name: "Fondo común de inversión Balanz", Currency: "ARS"},
	}
	// El toolbox de las 14 NO ENTRA en el techo de Groq, y esto lo mide en vez de
	// discutirlo: medido el 2026-08-12, un turno con las 14 pide 8.040 tokens
	// contra un limite de 8.000 y Groq lo rechaza con 413 antes de razonar nada.
	// (prompt + schemas ~5.040, mas los 3.000 de maxAgentCompletionTokens, que
	// Groq RESERVA aunque no se usen.)
	//
	// Asi que el eval corre con el toolbox que la etapa 5 va a shippear, que es
	// el unico que se puede medir. Es tambien la validacion mas dura de la tesis
	// de la etapa: sin recortar, el loop unificado no existe.
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

	// Pacing obligatorio, medido el 2026-08-12: cada turno pide ~6.949 tokens
	// contra un techo de 8.000 TPM, asi que el loop entra UNA VEZ POR MINUTO.
	// Sin esto el eval se auto-estrangula y todos los casos menos el primero
	// fallan por 429 en vez de por la eleccion de tool -- que es lo que mide.
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
					// Every action tool parks in the real thing; here it just
					// resolves so the loop can narrate and end.
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

// TestAgentDateAnchorEval mide lo único que el eval de arriba no mira: la FECHA
// que el modelo pone en el locator.
//
// Medido el 2026-08-19 sobre el caso real ("la compra de locro del lunes", un
// miércoles 19, con el lunes en el 17): 20b sin el día de la semana en el prompt
// dijo el 15; CON el día, el 14; y con la tabla de fechas de los últimos 7 días
// escrita en el prompto —o sea con "lunes 2026-08-17" delante— dijo el 14 igual.
// El 120b no mandó fecha ninguna. Este modelo no fecha un día de la semana, y
// no es cuestión de prompt: ignora el dato aunque lo tenga.
//
// Así que el contrato es al revés: una referencia relativa va SIN fecha, y la
// app la resuelve por la ventana de created_at, donde el matcheo textual la
// encuentra. Eso es lo que se mide acá.
//
// Con el schema pidiéndolo, "la semana pasada" y las fechas explícitas dan
// bien. "El lunes" NO: sigue mandando una fecha inventada aunque la descripción
// de date_from lo nombre como ejemplo de lo que no hay que completar. Ese caso
// queda ROJO A PROPÓSITO — es la medición del techo del modelo, no un pendiente.
// Arreglarlo pide que Go descarte la fecha cuando el texto nombra un día de la
// semana, y se decidió no hacerlo (2026-08-19).
//
// El "hoy" va fijo, así que el caso es el mismo corra cuando corra.
//
//	GROQ_APIKEY=... go test -tags llm_eval ./internal/orchestrator/ -run TestAgentDateAnchorEval -v -timeout 10m
func TestAgentDateAnchorEval(t *testing.T) {
	key := evalKey(t)

	_, taxonomy := seededTaxonomy(t)
	accounts := []AccountOption{{ID: 1, Name: "Mercado Pago", Currency: "ARS"}}
	tools := evalTools()
	// Miércoles. El lunes anterior es el 17; la semana pasada, lun 10 a dom 16.
	const hoy = "miércoles 2026-08-19"

	model := os.Getenv("GROQ_AGENT_MODEL")
	if model == "" {
		model = "openai/gpt-oss-20b"
	}
	o := New(Config{APIKey: key, BaseURL: evalBaseURL(), AgentModel: model, TimeoutSeconds: 60})

	cases := []struct {
		id    string
		msg   string
		today string
		// wantNoDate: la referencia es relativa, así que las dos fechas tienen
		// que venir vacías. Una fecha inventada acá manda la búsqueda a una
		// ventana donde el movimiento no está, y el usuario ve un picker de
		// movimientos ajenos.
		wantNoDate bool
		wantFrom   string // fecha explícita: transcribirla sí sabe
	}{
		// ROJO A PROPÓSITO: ver el comentario de arriba. Mide el techo del modelo.
		{id: "el_lunes_no_lleva_fecha", today: hoy, msg: "La compra de locro del lunes ponela en salidas restaurante", wantNoDate: true},
		{id: "la_semana_pasada_no_lleva_fecha", today: hoy, msg: "El gasto de la semana pasada ponelo en otra categoría", wantNoDate: true},
		{id: "el_4_de_agosto_si_lleva_fecha", today: hoy, msg: "El débito del 4 de agosto ponelo en Servicios", wantFrom: "2026-08-04"},
	}

	for i, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			if i > 0 {
				time.Sleep(62 * time.Second) // mismo pacing que TestAgentLoopEval
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

			// Un error DESPUÉS de la tool call no invalida la medición: lo que se
			// mide es el argumento, y la segunda ronda (la narración) se come un
			// 429 con sólo mirarla de reojo.
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

// TestAgentPromptSize es gratis —no llama a la API— y contesta la pregunta de
// costo con el desglose, que es lo único accionable: sin saber qué bloque pesa,
// "achicar el prompt" es adivinar.
//
// Antes comparaba contra el prompt de CREATE. Ese camino lo borró la etapa 5, y
// con él la comparación: hoy el loop no reemplaza a nadie, es el único que hay.
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
	// Los schemas van en CADA request, igual que el prompt: el loop reenvía la
	// lista entera en cada ronda.
	t.Logf("TOTAL por request: %d runas (~%d tokens a 4 runas/token)",
		total+schemas, (total+schemas)/4)
}

// truncate acorta una narración para el log del eval. Vivía en el eval del
// router; se muda acá porque ese archivo murió con el router.
func truncate(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len([]rune(s)) > 60 {
		return string([]rune(s)[:57]) + "..."
	}
	return s
}
