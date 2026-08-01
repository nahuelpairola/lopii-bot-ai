package messaging

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
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

	// questionKeyCandidate es el Key de la única pregunta que esta etapa sabe
	// hacer: cuál de los candidatos. La respuesta se resuelve contra
	// agentPayload.Candidates, en el mismo orden en que salieron los botones.
	questionKeyCandidate = "candidate"
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
	Change     string           `json:"change,omitempty"`
	Candidates []candidateGroup `json:"candidates,omitempty"`
	Chosen     int              `json:"chosen"`
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

func newAgentExecutor(c *controller, userID uint64, userText string, taxonomy []orchestrator.TaxonomyEntry) *agentExecutor {
	return &agentExecutor{c: c, userID: userID, userText: userText, taxonomy: taxonomy}
}

// wiredAgentTools son las únicas tools que este ejecutor sabe correr hoy. Es la
// misma lista que el switch de execute, y tiene que seguir siéndolo: las etapas
// 3 y 4 la amplían a medida que cablean el resto.
//
// Mandar las 14 no es neutro. El modelo elige entre lo que ve, y con
// record_movements a la vista contesta una corrección REGISTRANDO DE NUEVO:
// medido en producción, trace 84322077, el router clasificó UPDATE y el agente
// pidió record_movements igual. El prompt ya nombraba "eran 2000" como caso de
// correct_movement y no alcanzó — gpt-oss-20b no lo distingue, así que la
// opción se saca en vez de pedirle que no la elija.
//
// Y es más barato: las 10 que sobran son ~1.257 tokens de schema por llamada.
func wiredAgentTools() []orchestrator.AgentTool {
	wired := map[string]bool{
		orchestrator.ToolCorrectMovement: true,
		orchestrator.ToolDeleteMovements: true,
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
			Change string `json:"change"`
		}
		// Un argumento ilegible no puede tumbar el turno: el pedido igual se
		// entiende por el nombre de la tool, y el candidato sale del texto.
		_ = json.Unmarshal(args, &a)
		return e.park(orchestrator.ToolCorrectMovement, a.Change, msgPickUpdateCandidate(nil))
	case orchestrator.ToolRecordMovements:
		return e.record(args)
	case orchestrator.ToolDeleteMovements:
		return e.park(orchestrator.ToolDeleteMovements, "", msgPickDeleteCandidate(nil))
	case orchestrator.ToolReplyHelp:
		e.reply = msgHelp
		return "ya le mandaste al usuario la explicación de qué podés hacer", orchestrator.ErrAgentTurnDone
	case orchestrator.ToolAskRewrite:
		e.reply = msgAskRewrite
		return "ya le pediste al usuario que lo reescriba", orchestrator.ErrAgentTurnDone
	default:
		// Etapas 3 y 4 cablean el resto. Decírselo es mejor que fallar: el modelo
		// puede avisarle al usuario en vez de quedarse mudo.
		return resultNotWiredYet, nil
	}
}

// resultRecorded es lo que ve el MODELO, no el usuario: el recibo real lo manda
// la app (msgConfirmMovements). El prompt le pide explícitamente no repetir el
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

	seed := buildCreateSeed(result, e.taxonomy)
	seed[conversation.UserIDKey] = e.userID

	hasGaps := len(decodeStringSlice(seed, keyPendingCategoryGaps)) > 0 ||
		len(decodeStringSlice(seed, keyPendingAccountGaps)) > 0
	hasFirst := needsFirstAccount(seed, func(cur currency.Currency) bool {
		return e.c.accounts.HasDefaultForCurrency(e.userID, cur)
	})
	if hasGaps || hasFirst {
		return e.parkCreate(seed)
	}

	inserted, err := e.c.resolveAndInsertMovements(seed)
	if err != nil {
		var short *insufficientFunds
		if errors.As(err, &short) {
			return e.parkFundsGate(seed, short)
		}
		return "", fmt.Errorf("record_movements: %w", err)
	}
	// El orden importa: wrote ANTES de cualquier retorno, para que un 429 del
	// mismo turno ya lo vea puesto y no encole el mensaje.
	e.wrote = true
	e.inserted = inserted
	e.reply = msgConfirmMovements(inserted)
	return resultRecorded(len(inserted)), orchestrator.ErrAgentTurnDone
}

// park resuelve el candidato del lado de la app y deja la acción lista.
//
// Tres salidas, y las tres cierran el turno con ErrAgentTurnDone: un solo
// candidato → se parkea elegido, sin preguntas; varios → se parkea con la
// pregunta de cuál; ninguno → no se parkea nada y sale la copy de "no encontré".
// En los tres casos el texto que ve el usuario lo escribe la app, así que pedirle
// al modelo que lo narre es una vuelta entera de prompt tirada.
func (e *agentExecutor) park(tool, change, question string) (string, error) {
	groups, err := e.c.resolveCandidates(e.userID, e.userText, "", "")
	if err != nil {
		return "", fmt.Errorf("%s: resolve candidates: %w", tool, err)
	}
	if len(groups) == 0 {
		e.noCandidates = true
		// La misma copy que usan los dos sitios pre-loop (start_movement.go).
		e.reply = msgNoCandidatesFound
		return resultNoCandidates, orchestrator.ErrAgentTurnDone
	}

	candidates := make([]candidateGroup, 0, len(groups))
	options := make([]string, 0, len(groups))
	for _, g := range groups {
		candidates = append(candidates, toCandidateGroup(g))
		options = append(options, candidateLabel(g))
	}

	action := parkedAction{Tool: tool, Payload: agentPayload{Change: change, Candidates: candidates, Chosen: -1}}
	if len(candidates) == 1 {
		// Un solo candidato es el camino de hoy: se confirma, no se pregunta.
		action.Payload.Chosen = 0
	} else {
		action.Questions = []pendingaction.OpenQuestion{{
			Key: questionKeyCandidate, Prompt: question, Options: options,
		}}
	}
	e.parked = append(e.parked, action)
	return resultParked, orchestrator.ErrAgentTurnDone
}

// parkCreate y parkFundsGate se cablean en las tareas 5 y 6. Hasta entonces el
// CREATE incompleto le dice al modelo que la tool no está lista, que es el mismo
// contrato que las tools sin cablear — nunca inserta a medias.
func (e *agentExecutor) parkCreate(seed conversation.Data) (string, error) {
	return resultNotWiredYet, nil
}

func (e *agentExecutor) parkFundsGate(seed conversation.Data, short *insufficientFunds) (string, error) {
	return resultNotWiredYet, nil
}

func toCandidateGroup(g transactionGroup) candidateGroup {
	rows := make([]movementRow, 0, len(g.Movements))
	ids := make([]string, 0, len(g.Movements))
	for _, m := range g.Movements {
		rows = append(rows, movementToRow(m))
		ids = append(ids, strconv.FormatUint(uint64(m.ID), 10))
	}
	return candidateGroup{TransactionID: g.TransactionID, OldIDs: ids, Rows: rows}
}
