package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messages"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
)

// Lo que el ejecutor le devuelve al modelo. Son DATOS, no copy para el usuario:
// el modelo los usa para decidir si le queda algo por hacer y para narrar.
const (
	resultNoCandidates = "no encontré ningún movimiento que coincida con eso"
	resultParked       = "listo, la app sigue con eso y le pide confirmación al usuario"
	resultNotWiredYet  = "esa herramienta todavía no está disponible"

	// questionKeyCandidate es el Key de la pregunta de cuál de los candidatos. La
	// respuesta se resuelve contra agentPayload.Candidates, en el mismo orden en
	// que salieron los botones.
	questionKeyCandidate = "candidate"

	// questionKeyChange: el usuario nombró bien el movimiento pero no dijo qué
	// cambiarle ("el café estaba mal"). La respuesta se CONCATENA al change
	// original y se vuelve a resolver — no lo reemplaza, porque el texto
	// original suele traer a cuál se refiere.
	questionKeyChange = "change"
)

// parkedAction es una acción que el loop no pudo cerrar en el turno porque le
// falta algo que sólo el usuario tiene. Vive en memoria hasta que el despacho
// (agent_dispatch.go) la persiste.
type parkedAction struct {
	Tool      string
	Payload   agentPayload
	Questions []pendingaction.OpenQuestion
}

// agentPayload es lo que una acción parkeada necesita para retomarse.
//
// Candidates son grupos que resolvió LA APP con resolveCandidates. Chosen es el
// índice dentro de Candidates, o -1 cuando hay que preguntar cuál.
type agentPayload struct {
	Change     string                `json:"change,omitempty"`
	Candidates []flow.CandidateGroup `json:"candidates,omitempty"`
	Chosen     int                   `json:"chosen"`
	// Seed es el estado del CREATE que quedó incompleto: las filas ya resueltas
	// y qué falta. Viaja al flujo movement_create, que es quien sabe preguntar
	// categoría/subcategoría/cuenta con sus pickers. Sólo lo usa
	// record_movements; para correct/delete va nil.
	//
	// Ojo: pasa por JSONB, así que todo valor acá adentro tiene que ser string o
	// string-JSON. buildCreateSeed ya cumple (movement.MovementRow va codificado).
	Seed map[string]any `json:"seed,omitempty"`
	// GaveChangeValue marca que el usuario ya intentó decir el valor nuevo por
	// texto libre. Si aun así no sale una corrección, se corta: volver a
	// preguntar lo mismo es hacerlo girar.
	GaveChangeValue bool `json:"gave_change_value,omitempty"`
	// PickedChangeField marca que tocó uno de los botones ("La categoría"), o
	// sea que nombró el CAMPO y todavía falta el valor. Ahí no se corta: se
	// pregunta el valor, que es la segunda mitad de la misma pregunta.
	PickedChangeField bool `json:"picked_change_field,omitempty"`
	// PickedField es CUÁL campo eligió, no sólo que eligió uno. Con el campo y
	// el valor la app arma la corrección sola.
	PickedField string `json:"picked_field,omitempty"`
	// ChangeAnswer es lo ÚLTIMO que contestó, sin concatenar. Change lleva el
	// texto original pegado adelante ("el café estaba mal 2000") y así no
	// parsea; el atajo del monto necesita el "2000" solo.
	ChangeAnswer string `json:"change_answer,omitempty"`
	// Scope y Changes son la corrección ESTRUCTURADA: el modelo emite un diff
	// (campo, operación, valor) y la app lo aplica. Con Changes cargado el camino
	// de corrección NO vuelve a llamar al modelo — no hay nada que interpretar.
	Scope   string             `json:"scope,omitempty"`
	Changes []correctionChange `json:"changes,omitempty"`
}

// agentExecutor es el closure `execute` que Run llama por cada tool call.
//
// Run ya garantiza el orden por clase (write → read → action), así que acá NO se
// vuelve a ordenar: se despacha por nombre y listo.
//
// En esta etapa el trabajo del modelo es ELEGIR LA TOOL, nada más. A qué
// movimiento se refiere lo resuelve la app con resolveCandidates sobre el texto
// original — el mismo camino que corre hoy en producción y contra el que se
// tunearon el matcheo por tokens y el plegado de acentos. Ni un id sale del
// modelo, así que no hay id inventado posible.
type agentExecutor struct {
	// ctx es el del turno. Va en el struct, y no como parámetro, porque la
	// firma de execute la fija el loop del orchestrator y este ejecutor vive
	// exactamente un turno — el caso donde guardar un ctx es aceptable.
	//
	// No es cosmético: orchestrator.Client.record estampa trace.ID(ctx) en cada
	// fila de llm_calls. Con context.Background() —que es lo que había— las
	// llamadas del clasificador entraban con trace_id vacío, y son UNA POR
	// TURNO: la correlación de las tres capas se caía justo en la llamada nueva.
	ctx    context.Context
	c      *controller
	userID uint64
	// userText es el mensaje tal cual lo escribió el usuario. Es lo que se usa
	// para buscar candidatos y para armar la corrección — NO la paráfrasis del
	// modelo, que puede perder justo la palabra que matcheaba.
	userText string

	// taxonomy son los pares (categoría, subcategoría) que el usuario tiene de
	// verdad. buildCreateSeed la necesita para marcar gap cuando el modelo
	// inventa un par que no existe: sin eso la fila se inserta y después falla
	// al buscar la subcategoría, y el movimiento se pierde con un error genérico.
	taxonomy []orchestrator.TaxonomyEntry

	parked []parkedAction
	// reply es la respuesta que manda el controller (help / pedir reescritura),
	// no el modelo: es copy nuestra y tiene que salir textual.
	reply string
	// answerQuery: el loop decidió que esto es una consulta. La atiende el
	// controller después del turno, con el loop de query.
	answerQuery bool
	// settingsArea: cuenta | categoria | recordatorio. El loop ya leyó el
	// mensaje, así que elegir el área no cuesta una llamada extra.
	settingsArea string
	// replyButtons cuelga del recibo cuando el gate de casi-duplicado marca.
	// Van EN el recibo y no en un mensaje aparte: el gate no puede agregar un
	// mensaje ni un paso bloqueante, o deja de ser gratis ignorarlo.
	replyButtons []conversation.Button
	// noCandidates recuerda que no había nada que tocar, para que la métrica
	// diga no_candidates en vez de un fracaso genérico.
	noCandidates bool
	// wrote recuerda que este turno YA insertó movimientos. Es lo único que
	// separa un reintento seguro de un duplicado: si después de escribir el loop
	// se come un 429 y el mensaje se encola, el drenaje lo vuelve a correr y la
	// plata se registra dos veces. Spec 8.2.
	wrote bool
	// inserted son los movimientos que entraron en este turno. Van al
	// intent_event: sin los ids, create_inserted no se puede auditar contra la
	// plata que realmente se guardó.
	inserted []movement.Movement
}

func newAgentExecutor(ctx context.Context, c *controller, userID uint64, userText string, taxonomy []orchestrator.TaxonomyEntry) *agentExecutor {
	return &agentExecutor{ctx: ctx, c: c, userID: userID, userText: userText, taxonomy: taxonomy}
}

// wiredAgentTools son las únicas tools que este ejecutor sabe correr hoy. Es la
// misma lista que el switch de execute, y tiene que seguir siéndolo: la etapa 4
// la amplía a medida que cablea el resto.
//
// Mandar las 14 no es neutro. El modelo elige entre lo que ve, y con
// record_movements a la vista contestó una corrección REGISTRANDO DE NUEVO:
// medido en producción, trace 84322077, el router clasificó UPDATE y el agente
// pidió record_movements igual. En la etapa 2 se resolvió sacándola.
//
// En la etapa 3 no se puede sacar: es LA tool de CREATE, o sea el 74% del
// tráfico. Vuelve, y lo que separa corregir de registrar pasa a ser el
// desempate del copulativo en pasado ("era, eran, fue" → correct_movement), que
// viaja en el mismo prompt. Lo fija TestWiredTools_CorrectionStillPicksCorrectMovement.
//
// Y las 9 que siguen fuera son ~1.100 tokens de schema por llamada que no se pagan.
func wiredAgentTools() []orchestrator.AgentTool {
	wired := map[string]bool{
		orchestrator.ToolRecordMovements: true,
		orchestrator.ToolCorrectMovement: true,
		orchestrator.ToolDeleteMovements: true,
		orchestrator.ToolAnswerQuery:     true,
		orchestrator.ToolManageSettings:  true,
		orchestrator.ToolReplyHelp:       true,
		orchestrator.ToolAskRewrite:      true,
	}
	all := orchestrator.AgentTools()
	out := make([]orchestrator.AgentTool, 0, len(wired))
	for _, t := range all {
		if wired[t.Name] {
			out = append(out, t)
		}
	}
	return out
}

func (e *agentExecutor) execute(name string, args json.RawMessage) (string, error) {
	switch name {
	case orchestrator.ToolCorrectMovement:
		var a struct {
			Change  string             `json:"change"`
			Scope   string             `json:"scope"`
			Changes []correctionChange `json:"changes"`
		}
		// Un argumento ilegible no puede tumbar el turno: el pedido igual se
		// entiende por el nombre de la tool, y el candidato sale del texto.
		_ = json.Unmarshal(args, &a)
		defaultChangeOps(a.Changes)
		return e.park(parkRequest{
			tool: orchestrator.ToolCorrectMovement, change: a.Change,
			question: flow.MsgPickUpdateCandidate(nil), scope: a.Scope, changes: a.Changes,
		})
	case orchestrator.ToolRecordMovements:
		return e.record(args)
	case orchestrator.ToolDeleteMovements:
		return e.park(parkRequest{tool: orchestrator.ToolDeleteMovements, question: flow.MsgPickDeleteCandidate(nil)})
	case orchestrator.ToolAnswerQuery:
		// Sin argumentos: la app pasa el texto ORIGINAL. QUERY se queda en su
		// propio loop y su propio modelo a propósito — el techo de Groq es por
		// modelo, así que una consulta no le come TPM al loop unificado.
		e.answerQuery = true
		return "ya le contestaste la consulta al usuario", orchestrator.ErrAgentTurnDone
	case orchestrator.ToolManageSettings:
		var a struct {
			Area string `json:"area"`
		}
		_ = json.Unmarshal(args, &a)
		e.settingsArea = a.Area
		return "ya abriste la configuración que pidió el usuario", orchestrator.ErrAgentTurnDone
	case orchestrator.ToolReplyHelp:
		e.reply = messages.MsgHelp
		return "ya le mandaste al usuario la explicación de qué podés hacer", orchestrator.ErrAgentTurnDone
	case orchestrator.ToolAskRewrite:
		e.reply = messages.MsgAskRewrite
		return "ya le pediste al usuario que lo reescriba", orchestrator.ErrAgentTurnDone
	default:
		// Etapas 3 y 4 cablean el resto. Decírselo es mejor que fallar: el modelo
		// puede avisarle al usuario en vez de quedarse mudo.
		return resultNotWiredYet, nil
	}
}

// resultRecorded es lo que ve el MODELO, no el usuario: el recibo real lo manda
// la app (messages.MsgConfirmMovements). El prompt le pide explícitamente no repetir el
// detalle.
func resultRecorded(n int) string {
	return fmt.Sprintf("registrados: %d movimientos", n)
}

// record corre el CREATE del lado de la app. El seed, los gaps y la inserción
// son EXACTAMENTE los de start_movement.go: el loop cambia cómo se llega hasta
// acá, no qué pasa después. En particular resolveAndInsertMovements lleva
// adentro el guard (Normalize / AssignTransactionIDs / CheckBalances) y no se
// toca.
func (e *agentExecutor) record(args json.RawMessage) (string, error) {
	var result orchestrator.CreateResult
	if err := json.Unmarshal(args, &result); err != nil {
		// Acá sí importa el argumento: sin filas no hay nada que registrar, y a
		// diferencia de una corrección el texto del usuario no alcanza para
		// reconstruirlas. Se lo decimos al modelo en vez de tumbar el turno.
		return "no pude leer los movimientos, pedile al usuario que lo reescriba", nil
	}
	if len(result.Movements) == 0 {
		return "no venía ningún movimiento", nil
	}
	// El camino viejo normaliza dentro de ClassifyCreate; acá los argumentos se
	// desarman a mano, así que hay que pedirlo. Sin esto el modelo devuelve
	// "Vivienda | Luz" en el campo categoría, el par no matchea la taxonomía y
	// el gap-fill le pregunta al usuario la categoría que ya había dicho.
	// La categoría ya no viene del loop: el schema de record_movements no la
	// pide. Se asigna acá, en dos pasos — primero por la FORMA del movimiento
	// (gratis, sin modelo), y lo que quede va a una llamada dedicada en otro
	// modelo, o sea en otro techo de TPM.
	e.classify(result.Movements)

	// Normalize va DESPUÉS de clasificar, no antes: el que puede devolver
	// "Vivienda | Luz" metido en el campo categoría ahora es el clasificador,
	// no el loop. Normalizar antes dejaría el par mal formado, el par no
	// matchearía la taxonomía, y el gap-fill le preguntaría al usuario una
	// categoría que el sistema ya sabía.
	result.Normalize()

	// Las cuentas van al seed para resolver el nombre que dijo el usuario contra
	// una cuenta real ANTES de decidir que hay que preguntar. Un error de lectura
	// acá no puede inventar un gap: matchNamedAccount devuelve 0 y sigue el
	// camino de hoy.
	accounts, _ := e.c.accounts.FindByUserID(e.userID)
	seed := buildCreateSeed(result, e.taxonomy, accounts)
	seed[conversation.UserIDKey] = e.userID

	hasGaps := len(conversation.DecodeStringSlice(seed, conversation.KeyPendingCategoryGaps)) > 0 ||
		len(conversation.DecodeStringSlice(seed, conversation.KeyPendingAccountGaps)) > 0
	hasFirst := flow.NeedsFirstAccount(seed, func(cur currency.Currency) bool {
		return e.c.accounts.HasDefaultForCurrency(e.userID, cur)
	})
	if hasGaps || hasFirst {
		return e.parkCreate(seed)
	}

	inserted, err := e.c.resolveAndInsertMovements(seed)
	if err != nil {
		var short *flow.InsufficientFunds
		if errors.As(err, &short) {
			return e.parkFundsGate(seed, short)
		}
		return "", fmt.Errorf("record_movements: %w", err)
	}
	// El orden importa: wrote ANTES de cualquier retorno, para que un 429 del
	// mismo turno ya lo vea puesto y no encole el mensaje.
	e.wrote = true
	e.inserted = inserted
	e.reply = messages.MsgConfirmMovements(inserted)
	e.replyButtons = e.c.maybeNearDuplicate(e.userID, inserted)
	return resultRecorded(len(inserted)), orchestrator.ErrAgentTurnDone
}

// classify completa el par (categoría, subcategoría) de cada fila.
//
// Paso 1, por regla y sin modelo: hay pares que son función de la FORMA del
// movimiento, y la app conoce la forma. Es un DEFAULT, no una regla dura —una
// suscripción de FCI tiene la misma forma que una transferencia—, así que el
// clasificador puede pisarlo; lo que compra es que el caso abrumador no gaste
// una decisión del modelo y que el par salga bien escrito.
//
// Paso 2, una llamada para TODAS las filas que quedaron: comparten el mensaje,
// y separarlas les quitaría el contexto que se dan entre sí.
//
// Si algo falla, las filas quedan en PENDING_REVIEW y buildCreateSeed les abre
// gap: degradar a una pregunta es correcto, degradar a un dato inventado no.
func (e *agentExecutor) classify(movements []orchestrator.MovementDraft) {
	if len(movements) == 0 {
		return
	}

	// El par que se deduce de la FORMA es un DEFAULT, no una regla dura — y hasta
	// el 2026-08-12 acá había un `return` que lo volvía dura, contradiciendo el
	// comentario de StructuralPair, que dice textualmente que el clasificador
	// puede pisarlo.
	//
	// Medido en vivo: "Suscribi 3100000 a FCI" quedó en `Sistema | Transferencia`.
	// Una suscripción de FCI entre dos cuentas propias en la misma moneda es un
	// grupo de 2 patas que suma cero —la forma EXACTA de una transferencia— y no
	// es una transferencia: `Inversiones | FCI` existe como par propio. Sólo el
	// mensaje distingue una de la otra, así que el que tiene que decidir es el
	// que lee el mensaje.
	//
	// El default sigue sirviendo, y para dos cosas: cuando el clasificador falla
	// (429, timeout) y cuando duda. Sin él, un transfer sin clasificar caería en
	// PENDING_REVIEW y abriría el picker por algo que la forma ya contesta.
	structCat, structSub, hasStructural := orchestrator.StructuralPair(movements)

	rows := make([]orchestrator.ClassifyRow, 0, len(movements))
	for _, m := range movements {
		rows = append(rows, orchestrator.ClassifyRow{
			Description: m.Description,
			Type:        m.Type,
			AccountName: m.AccountNameGuess,
		})
	}
	pairs := e.c.orchestrator.ClassifyCategories(e.ctx, e.userText, rows, e.taxonomy)
	for i := range movements {
		cat, sub := "", ""
		if i < len(pairs) {
			cat, sub = pairs[i].Category, pairs[i].Subcategory
		}
		// El default entra sólo donde el clasificador no dijo nada útil.
		if hasStructural && (cat == "" || cat == constants.PendingReview) {
			cat, sub = structCat, structSub
		}
		movements[i].Category, movements[i].Subcategory = cat, sub
	}
}

// park resuelve el candidato del lado de la app y deja la acción lista.
//
// Tres salidas, y las tres cierran el turno con ErrAgentTurnDone: un solo
// candidato → se parkea elegido, sin preguntas; varios → se parkea con la
// pregunta de cuál; ninguno → no se parkea nada y sale la copy de "no encontré".
// En los tres casos el texto que ve el usuario lo escribe la app, así que pedirle
// al modelo que lo narre es una vuelta entera de prompt tirada.
// parkRequest son los datos de un parkeo. Es un struct y no seis parámetros
// sueltos porque cuatro de los seis son strings: invertir dos en una llamada
// compila igual y manda la copy del picker como el cambio pedido.
type parkRequest struct {
	tool     string
	change   string
	question string
	// scope y changes sólo los usa correct_movement. scopeAll pide que el cambio
	// caiga sobre TODOS los candidatos, no sobre uno elegido.
	scope   string
	changes []correctionChange
}

func (e *agentExecutor) park(req parkRequest) (string, error) {
	tool, change, question := req.tool, req.change, req.question
	groups, err := e.c.resolveCandidates(e.userID, e.userText, "", "")
	if err != nil {
		return "", fmt.Errorf("%s: resolve candidates: %w", tool, err)
	}
	if len(groups) == 0 {
		e.noCandidates = true
		// La misma copy que usan los dos sitios pre-loop (start_movement.go).
		e.reply = messages.MsgNoCandidatesFound
		return resultNoCandidates, orchestrator.ErrAgentTurnDone
	}

	candidates := make([]flow.CandidateGroup, 0, len(groups))
	options := make([]string, 0, len(groups))
	for _, g := range groups {
		candidates = append(candidates, toCandidateGroup(g))
		options = append(options, candidateLabel(g))
	}

	action := parkedAction{Tool: tool, Payload: agentPayload{
		Change: change, Candidates: candidates, Chosen: -1,
		Scope: req.scope, Changes: req.changes,
	}}
	switch {
	case len(candidates) == 1:
		// Un solo candidato es el camino de hoy: se confirma, no se pregunta.
		action.Payload.Chosen = 0
	case isBatchCorrection(action.Payload):
		// El cambio cae sobre TODOS los candidatos, así que no hay cuál
		// preguntar y Chosen se queda en -1. Es el caso guía del 2026-08-10
		// ("mové los movimientos del lote a proyecto hogar"): con el picker el
		// usuario elegía uno y los otros dos se quedaban donde estaban.
	default:
		action.Questions = []pendingaction.OpenQuestion{{
			Key: questionKeyCandidate, Prompt: question, Options: options,
		}}
	}
	e.parked = append(e.parked, action)
	return resultParked, orchestrator.ErrAgentTurnDone
}

// isBatchCorrection dice si el cambio cae sobre TODOS los candidatos en vez de
// sobre uno elegido. Dos condiciones, y ninguna es de adorno:
//
//   - scope=all, o sea el modelo leyó un pedido en plural;
//   - un cambio ESTRUCTURADO. Sin él la corrección la resuelve el modelo
//     devolviendo las filas ya corregidas, y pedirle ocho filas enteras para
//     tocar un campo es donde corrompe datos en silencio. Un pedido en plural
//     sin `changes` vuelve al picker: corregir de a uno es peor que hoy, pero no
//     rompe nada.
//
// Los grupos NO se fusionan: las guardas de conjunto (applyChangesToSet) miran
// cuántos son, y un lote aplanado a un grupo se les escaparía — "poné todos en
// 1500" dejaría de ser ambiguo justo cuando más lo es.
func isBatchCorrection(p agentPayload) bool {
	return p.Scope == scopeAll && len(p.Changes) > 0 && len(p.Candidates) > 1
}

// parkCreate deja el CREATE incompleto en la cola. No inserta NADA: la regla es
// todo-o-nada por lote, para que una transferencia de dos piernas no se parta.
//
// Lo que sigue después es el flujo movement_create de siempre, con sus pickers
// de categoría y cuenta. El loop cambia cómo se llega hasta ahí, no qué pasa
// después.
func (e *agentExecutor) parkCreate(seed conversation.Data) (string, error) {
	e.parked = append(e.parked, parkedAction{
		Tool:    orchestrator.ToolRecordMovements,
		Payload: agentPayload{Seed: seed, Chosen: 0},
	})
	return "pendiente: faltan datos, la app se los pide al usuario", orchestrator.ErrAgentTurnDone
}

// parkFundsGate manda el CREATE al gate de saldo negativo. Igual que hoy en
// start_movement.go: no se inserta nada, se le muestra el faltante y decide el
// usuario. El loop no relaja ni un gate (spec 4.5).
//
// conversation.KeyGatePrompt es lo que distingue este parkeo del de gaps al retomarlo: es la
// copy del faltante, y sólo la pone este camino.
func (e *agentExecutor) parkFundsGate(seed conversation.Data, short *flow.InsufficientFunds) (string, error) {
	gateSeed := conversation.CopyData(seed)
	gateSeed[conversation.KeyGatePrompt] = messages.MsgInsufficientFunds(short.Shortfalls)
	e.parked = append(e.parked, parkedAction{
		Tool:    orchestrator.ToolRecordMovements,
		Payload: agentPayload{Seed: gateSeed, Chosen: 0},
	})
	return "pendiente: el saldo no alcanza, la app le pide confirmación al usuario", orchestrator.ErrAgentTurnDone
}

func toCandidateGroup(g transactionGroup) flow.CandidateGroup {
	rows := make([]movement.MovementRow, 0, len(g.Movements))
	ids := make([]string, 0, len(g.Movements))
	for _, m := range g.Movements {
		rows = append(rows, movementToRow(m))
		ids = append(ids, strconv.FormatUint(uint64(m.ID), 10))
	}
	return flow.CandidateGroup{TransactionID: g.TransactionID, OldIDs: ids, Rows: rows}
}
