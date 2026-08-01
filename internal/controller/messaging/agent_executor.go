package messaging

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

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
// Candidates son grupos que resolvió LA APP con resolveCandidates, no ids que
// dictó el modelo. Chosen es el índice dentro de Candidates, o -1 cuando hay que
// preguntar cuál.
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
// groups es el resguardo que importa: lo que devolvió find_movements_to_correct,
// cacheado. La acción parkeada se arma SIEMPRE desde este cache — un id que el
// modelo se haya inventado no matchea y cae al fallback de preguntar, nunca a
// actuar sobre un movimiento cualquiera.
//
// La clave es un handle propio ("1", "2", ...) y NO el transaction_id: un
// movimiento sin agrupar lo tiene vacío, así que todos colisionarían en la misma
// entrada. De paso el modelo nunca ve un UUID, que son 36 caracteres de tokens
// que no le sirven para nada.
type agentExecutor struct {
	c      *controller
	userID uint64

	groups map[string]transactionGroup
	order  []string // handles en orden de presentación; las opciones lo siguen

	parked []parkedAction
	// reply es la respuesta que manda el controller (help / pedir reescritura),
	// no el modelo: es copy nuestra y tiene que salir textual.
	reply string
}

func newAgentExecutor(c *controller, userID uint64) *agentExecutor {
	return &agentExecutor{c: c, userID: userID, groups: map[string]transactionGroup{}}
}

func (e *agentExecutor) execute(name string, args json.RawMessage) (string, error) {
	switch name {
	case orchestrator.ToolFindMovementsToCorrect:
		return e.findMovementsToCorrect(args)
	case orchestrator.ToolCorrectMovement:
		return e.correctMovement(args)
	case orchestrator.ToolDeleteMovements:
		return e.deleteMovements(args)
	case orchestrator.ToolReplyHelp:
		e.reply = msgHelp
		return "ya le mandaste al usuario la explicación de qué podés hacer", nil
	case orchestrator.ToolAskRewrite:
		e.reply = msgAskRewrite
		return "ya le pediste al usuario que lo reescriba", nil
	default:
		// Etapas 3 y 4 cablean el resto. Decírselo es mejor que fallar: el modelo
		// puede avisarle al usuario en vez de quedarse mudo.
		return resultNotWiredYet, nil
	}
}

func (e *agentExecutor) findMovementsToCorrect(args json.RawMessage) (string, error) {
	var a struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("find_movements_to_correct args: %w", err)
	}

	groups, err := e.c.resolveCandidates(e.userID, a.Text, "", "")
	if err != nil {
		return "", fmt.Errorf("find_movements_to_correct: %w", err)
	}
	e.cache(groups)

	if len(groups) == 0 {
		return resultNoCandidates, nil
	}

	var b strings.Builder
	b.WriteString("movimientos encontrados (pasá el id a correct_movement o delete_movements):\n")
	for _, handle := range e.order {
		fmt.Fprintf(&b, "%s | %s\n", handle, candidateLabel(e.groups[handle]))
	}
	return b.String(), nil
}

func (e *agentExecutor) correctMovement(args json.RawMessage) (string, error) {
	var a struct {
		TransactionID string `json:"transaction_id"`
		Change        string `json:"change"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("correct_movement args: %w", err)
	}
	// La copia del picker es la misma que hoy: una sola fuente para el texto.
	return e.park(orchestrator.ToolCorrectMovement, a.TransactionID, a.Change, msgPickUpdateCandidate(nil))
}

func (e *agentExecutor) deleteMovements(args json.RawMessage) (string, error) {
	var a struct {
		TransactionID string `json:"transaction_id"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("delete_movements args: %w", err)
	}
	return e.park(orchestrator.ToolDeleteMovements, a.TransactionID, "", msgPickDeleteCandidate(nil))
}

// park arma la acción parkeada resolviendo el candidato SIEMPRE contra el cache.
//
// Tres salidas, en orden: el id cachea a un grupo → se parkea sin preguntas; no
// cachea a nada pero hay candidatos → se parkea con la pregunta de cuál; no hay
// ni un candidato → no se parkea nada y el modelo se lo dice al usuario.
func (e *agentExecutor) park(tool, handle, change, question string) (string, error) {
	if g, ok := e.groups[handle]; ok && handle != "" {
		e.parked = append(e.parked, parkedAction{
			Tool:    tool,
			Payload: agentPayload{Change: change, Candidates: []candidateGroup{toCandidateGroup(g)}, Chosen: 0},
		})
		return resultParked, nil
	}

	// Sin cache utilizable (el modelo no llamó a find_movements_to_correct, o
	// devolvió un id que no existe): lo resuelve la app.
	if len(e.order) == 0 {
		groups, err := e.c.resolveCandidates(e.userID, change, "", "")
		if err != nil {
			return "", fmt.Errorf("%s: resolve candidates: %w", tool, err)
		}
		e.cache(groups)
	}
	if len(e.order) == 0 {
		return resultNoCandidates, nil
	}

	candidates := make([]candidateGroup, 0, len(e.order))
	options := make([]string, 0, len(e.order))
	for _, h := range e.order {
		g := e.groups[h]
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
	return resultParked, nil
}

// cache le da a cada grupo un handle correlativo y lo guarda. Se llama a lo sumo
// dos veces por turno (buscar, y el fallback al parkear), así que un id que ya
// se entregó no cambia de significado a mitad de camino.
func (e *agentExecutor) cache(groups []transactionGroup) {
	for _, g := range groups {
		handle := strconv.Itoa(len(e.order) + 1)
		e.order = append(e.order, handle)
		e.groups[handle] = g
	}
}

func toCandidateGroup(g transactionGroup) candidateGroup {
	rows := make([]movementRow, 0, len(g.Movements))
	ids := make([]string, 0, len(g.Movements))
	for _, m := range g.Movements {
		rows = append(rows, movementToRow(m))
		ids = append(ids, fmt.Sprintf("%d", m.ID))
	}
	return candidateGroup{TransactionID: g.TransactionID, OldIDs: ids, Rows: rows}
}
