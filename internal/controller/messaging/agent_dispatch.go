package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
	"lopiibot.com/internal/trace"
)

// budgetSlack son las vueltas de gracia por encima de la cantidad de preguntas
// abiertas al parkear. Dos: una para una respuesta que no sirvió, otra para el
// reintento. A la tercera se descarta — insistir más es hacerle perder el tiempo
// al usuario con algo que el bot no va a entender.
const budgetSlack = 2

// parkAgentActions persiste lo que el loop dejó abierto, en el orden en que el
// ejecutor las juntó (Run ya las ordenó por clase).
//
// El presupuesto se congela acá y no se recalcula al drenar: sale de la cantidad
// de preguntas abiertas EN ESTE MOMENTO. Ver pendingaction.PendingAction.
func (c *controller) parkAgentActions(ctx context.Context, userID uint64, actions []parkedAction) error {
	for i, a := range actions {
		payload, err := json.Marshal(a.Payload)
		if err != nil {
			return fmt.Errorf("park %s: payload: %w", a.Tool, err)
		}
		questions, err := json.Marshal(a.Questions)
		if err != nil {
			return fmt.Errorf("park %s: questions: %w", a.Tool, err)
		}
		row := &pendingaction.PendingAction{
			UserID:    userID,
			Tool:      a.Tool,
			Payload:   payload,
			Questions: questions,
			Budget:    len(a.Questions) + budgetSlack,
			Position:  i,
			TraceID:   trace.ID(ctx),
		}
		if err := c.actions.Insert(row); err != nil {
			return fmt.Errorf("park %s: %w", a.Tool, err)
		}
	}
	return nil
}

// drainNextAgentAction abre la próxima acción parkeada del usuario: o le
// pregunta lo que falta, o la retoma directo si ya está resuelta.
//
// Se llama al terminar un flujo, así que hay a lo sumo UNA acción abierta a la
// vez — de ahí que la métrica se resuelva al drenar y no al parkear: si se
// registrara al parkear, dos acciones de un mismo mensaje dejarían dos pendientes
// vivos y el WIP=1 de intent_events se rompería.
func (c *controller) drainNextAgentAction(ctx context.Context, b *bot.Bot, chatID int64, userID uint64) error {
	if c.actions == nil {
		return nil
	}
	action, err := c.actions.NextForUser(userID)
	if errors.Is(err, pendingaction.ErrNoPendingAction) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("drain: next action: %w", err)
	}

	var questions []pendingaction.OpenQuestion
	if err := json.Unmarshal(action.Questions, &questions); err != nil {
		return fmt.Errorf("drain: questions: %w", err)
	}
	if hasOpenQuestion(questions) {
		return c.openAskUser(ctx, b, chatID, userID, action, action.Budget)
	}
	return c.resumeAgentAction(ctx, b, chatID, userID, action)
}

// openAskUser arranca (o vuelve a arrancar) el flujo de preguntas para una
// acción. El presupuesto viaja en Data y no en la fila: sobrevive a que se
// reabra la misma pregunta sin tocar la DB.
func (c *controller) openAskUser(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, action *pendingaction.PendingAction, budget int) error {
	if budget <= 0 {
		return c.discardAgentAction(ctx, b, chatID, userID, action)
	}
	seed := conversation.Data{
		keyActionID:      strconv.FormatUint(action.ID, 10),
		keyOpenQuestions: string(action.Questions),
		keyAskBudget:     strconv.Itoa(budget),
	}
	prompt, err := c.engine.StartWithData(userID, askUserFlowName, seed)
	if err != nil {
		return fmt.Errorf("drain: start ask_user: %w", err)
	}
	c.sendPrompt(ctx, b, chatID, prompt)
	return nil
}

// finishAskUserFlow corre cuando el usuario terminó de contestar. Decide entre
// tres finales: cancelar, descartar por presupuesto, o retomar la acción.
func (c *controller) finishAskUserFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	userID := data.UserID()
	action, err := c.openAction(userID, data)
	if err != nil {
		slog.ErrorContext(ctx, "ask_user finished with no matching action", "err", err, "user_id", userID)
		c.sendText(ctx, b, chatID, msgSomethingBroke)
		return
	}

	if flag(data, keyCancelled) {
		c.dropAgentAction(ctx, b, chatID, userID, action, msgUpdateCancelled)
		return
	}
	if flag(data, keyAskDiscarded) {
		if err := c.discardAgentAction(ctx, b, chatID, userID, action); err != nil {
			slog.ErrorContext(ctx, "discard parked action failed", "err", err)
		}
		return
	}

	answers := decodeOpenQuestions(data)
	payload, resolved := applyAnswers(action, answers)
	if !resolved {
		// La respuesta no cerró la pregunta (texto libre que no nombra ninguno de
		// los candidatos). Se vuelve a preguntar con lo que quede de presupuesto.
		if err := c.openAskUser(ctx, b, chatID, userID, action, askBudget(data)); err != nil {
			slog.ErrorContext(ctx, "reopen ask_user failed", "err", err)
			c.sendText(ctx, b, chatID, msgSomethingBroke)
		}
		return
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		slog.ErrorContext(ctx, "marshal resolved payload failed", "err", err)
		c.sendText(ctx, b, chatID, msgSomethingBroke)
		return
	}
	action.Payload = raw
	if err := c.resumeAgentAction(ctx, b, chatID, userID, action); err != nil {
		slog.ErrorContext(ctx, "resume parked action failed", "err", err, "tool", action.Tool)
		c.sendText(ctx, b, chatID, msgSomethingBroke)
	}
}

// openAction devuelve la acción que el ask_user abierto estaba resolviendo.
// Con WIP=1 es siempre la próxima de la cola, pero se verifica el id igual: si
// no coincide, algo se desincronizó y actuar sería actuar sobre otra cosa.
func (c *controller) openAction(userID uint64, data conversation.Data) (*pendingaction.PendingAction, error) {
	if c.actions == nil {
		return nil, errors.New("no pending action repository")
	}
	action, err := c.actions.NextForUser(userID)
	if err != nil {
		return nil, err
	}
	if strconv.FormatUint(action.ID, 10) != stringOrEmpty(data[keyActionID]) {
		return nil, fmt.Errorf("ask_user was answering action %s, queue head is %d", stringOrEmpty(data[keyActionID]), action.ID)
	}
	return action, nil
}

// applyAnswers vuelca las respuestas al payload. Devuelve resolved=false cuando
// alguna quedó sin poder interpretarse.
func applyAnswers(action *pendingaction.PendingAction, answers []pendingaction.OpenQuestion) (agentPayload, bool) {
	var payload agentPayload
	if err := json.Unmarshal(action.Payload, &payload); err != nil {
		return payload, false
	}
	for _, q := range answers {
		switch q.Key {
		case questionKeyCandidate:
			// Las opciones salieron en el mismo orden que Candidates, así que la
			// posición de la etiqueta ES el índice del candidato.
			idx := indexOf(q.Options, q.Answer)
			if idx < 0 {
				return payload, false
			}
			payload.Chosen = idx
		case questionKeyChange:
			// Se CONCATENA, no se reemplaza: el texto original suele traer a cuál
			// se refiere ("el café"), y la respuesta trae el valor nuevo ("2000").
			// Con cualquiera de los dos solo, ResolveUpdate se queda corto.
			payload.Change = strings.TrimSpace(payload.Change + " " + q.Answer)
			payload.AskedWhatToChange = true
		}
	}
	// La pregunta de "qué cambiar" se parkea con el candidato YA elegido, así
	// que esto la da por resuelta apenas contesta algo. Exigir además un Change
	// no vacío dejaría sin resolver a DELETE, que nunca lleva uno.
	return payload, payload.Chosen >= 0
}

func indexOf(options []string, want string) int {
	for i, o := range options {
		if o == want {
			return i
		}
	}
	return -1
}

// resumeAgentAction entrega la acción resuelta al gate que ya existe. Ni el
// confirm de corrección ni el de borrado se tocan: el loop cambia CÓMO se llega
// hasta ahí, no qué pasa después.
func (c *controller) resumeAgentAction(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, action *pendingaction.PendingAction) error {
	var payload agentPayload
	if err := json.Unmarshal(action.Payload, &payload); err != nil {
		return fmt.Errorf("resume: payload: %w", err)
	}

	// El candidato se valida ANTES de borrar: un payload corrupto no se tira en
	// silencio, se deja en la cola y falla ruidoso. Un CREATE se saltea el
	// chequeo porque no tiene candidatos — lo que viaja es el seed a medio
	// resolver.
	var chosen candidateGroup
	if action.Tool != orchestrator.ToolRecordMovements {
		var err error
		if chosen, err = chosenCandidate(payload); err != nil {
			return err
		}
	}

	// Se borra ANTES de abrir el gate: si el gate falla, el usuario vuelve a
	// escribir — pero una acción que quedó en la cola bloquearía la siguiente
	// para siempre.
	if err := c.actions.Delete(action.ID); err != nil {
		return fmt.Errorf("resume: delete action: %w", err)
	}

	switch action.Tool {
	case orchestrator.ToolRecordMovements:
		// Quien sabe preguntar categoría/subcategoría/cuenta es movement_create,
		// con sus pickers. El loop cambia cómo se llega hasta acá.
		seed := conversation.Data{}
		for k, v := range payload.Seed {
			seed[k] = v
		}
		// Dos parkeos distintos vuelven por acá: el que tiene gaps y el que
		// chocó contra el saldo. La copy del faltante es lo único que los
		// separa — la pone parkFundsGate y nadie más.
		flow := movementCreateFlowName
		if _, gated := payload.Seed[keyGatePrompt]; gated {
			flow = movementNegativeConfirmFlowName
		}
		return c.startFlow(ctx, b, chatID, userID, flow, seed, "drain: start "+flow)
	case orchestrator.ToolCorrectMovement:
		return c.proceedToUpdateConfirm(ctx, b, chatID, userID, payload.Change, chosen.TransactionID, chosen.OldIDs, chosen.Rows, payload.AskedWhatToChange)
	case orchestrator.ToolDeleteMovements:
		seed := conversation.Data{
			keyCandidateGroups: encodeCandidateGroupList([]candidateGroup{chosen}),
			keyResolvedIndex:   "0",
		}
		return c.startFlow(ctx, b, chatID, userID, movementDeleteFlowName, seed, "drain: start movement_delete flow")
	default:
		return fmt.Errorf("resume: tool %q has no resume path", action.Tool)
	}
}

// chosenCandidate saca el candidato elegido. El chequeo de rango vive acá y no
// arriba porque sólo aplica a las tools que TIENEN candidatos: un CREATE nunca
// los tiene, y el chequeo genérico lo rechazaba antes de llegar a su rama.
func chosenCandidate(payload agentPayload) (candidateGroup, error) {
	if payload.Chosen < 0 || payload.Chosen >= len(payload.Candidates) {
		return candidateGroup{}, fmt.Errorf("resume: candidate %d out of range (%d)", payload.Chosen, len(payload.Candidates))
	}
	return payload.Candidates[payload.Chosen], nil
}

// discardAgentAction tira la acción entera y NOMBRA lo que se cayó. Tirar un
// movimiento en silencio es exactamente la falla que todo esto viene a evitar.
func (c *controller) discardAgentAction(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, action *pendingaction.PendingAction) error {
	slog.InfoContext(ctx, "parked action discarded: budget exhausted",
		"tool", action.Tool, "user_id", userID, "budget", action.Budget)
	c.resolveMetric(ctx, userID, outcomeCreateFailed)
	c.dropAgentAction(ctx, b, chatID, userID, action, msgAgentActionDiscarded(describeAction(action)))
	return nil
}

// dropAgentAction saca la acción de la cola, avisa, y sigue con la que venga
// atrás — si no, una cancelación dejaría el resto de la cola trabado.
func (c *controller) dropAgentAction(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, action *pendingaction.PendingAction, message string) {
	if err := c.actions.Delete(action.ID); err != nil {
		slog.ErrorContext(ctx, "delete parked action failed", "err", err)
	}
	c.sendText(ctx, b, chatID, message)
	if err := c.drainNextAgentAction(ctx, b, chatID, userID); err != nil {
		slog.ErrorContext(ctx, "drain after drop failed", "err", err)
	}
}

func msgAgentActionDiscarded(what string) string {
	return fmt.Sprintf(msgAgentActionDiscardedTemplate, what)
}

// describeAction rinde la acción en palabras del usuario, para poder decirle qué
// se cayó. Una corrección lleva el texto que él mismo escribió.
func describeAction(action *pendingaction.PendingAction) string {
	var payload agentPayload
	_ = json.Unmarshal(action.Payload, &payload)
	if payload.Change != "" {
		return "«" + payload.Change + "»"
	}
	if action.Tool == orchestrator.ToolDeleteMovements {
		return "el borrado que me pediste"
	}
	return "lo que me pediste"
}
