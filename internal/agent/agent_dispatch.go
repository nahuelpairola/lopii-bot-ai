package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messages"
	"lopiibot.com/internal/messenger"
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
func parkAgentActions(ctx context.Context, svc agentServices, userID uint64, actions []parkedAction) error {
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
		if err := svc.ActionsInsert(row); err != nil {
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
func drainNextAgentAction(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64) error {
	if !svc.ActionsEnabled() {
		return nil
	}
	action, err := svc.ActionsNextForUser(userID)
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
	if flow.HasOpenQuestion(questions) {
		return openAskUser(ctx, svc, chat, userID, action, action.Budget)
	}
	return resumeAgentAction(ctx, svc, chat, userID, action)
}

// openAskUser arranca (o vuelve a arrancar) el flujo de preguntas para una
// acción. El presupuesto viaja en Data y no en la fila: sobrevive a que se
// reabra la misma pregunta sin tocar la DB.
func openAskUser(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64, action *pendingaction.PendingAction, budget int) error {
	if budget <= 0 {
		return discardAgentAction(ctx, svc, chat, userID, action)
	}
	seed := conversation.Data{
		conversation.KeyActionID:      strconv.FormatUint(action.ID, 10),
		conversation.KeyOpenQuestions: string(action.Questions),
		conversation.KeyAskBudget:     strconv.Itoa(budget),
	}
	prompt, err := svc.EngineStartWithData(userID, flow.AskUserFlowName, seed)
	if err != nil {
		return fmt.Errorf("drain: start ask_user: %w", err)
	}
	svc.SendPrompt(ctx, chat, prompt)
	return nil
}

// finishAskUserFlow corre cuando el usuario terminó de contestar. Decide entre
// tres finales: cancelar, descartar por presupuesto, o retomar la acción.
func finishAskUserFlow(ctx context.Context, svc agentServices, chat messenger.Chat, data conversation.Data) {
	userID := data.UserID()
	action, err := openAction(svc, userID, data)
	if err != nil {
		slog.ErrorContext(ctx, "ask_user finished with no matching action", "err", err, "user_id", userID)
		svc.SendText(ctx, chat, flow.MsgSomethingBroke)
		return
	}

	if conversation.Flag(data, conversation.KeyCancelled) {
		// record_movements no cae acá: parkCreate no setea Questions.
		outcome, msg := flow.OutcomeUpdateCancelled, flow.MsgUpdateCancelled
		if action.Tool == orchestrator.ToolDeleteMovements {
			outcome, msg = flow.OutcomeDeleteCancelled, flow.MsgDeleteCancelled
		}
		resolveMetric(ctx, svc, userID, outcome)
		dropAgentAction(ctx, svc, chat, userID, action, msg)
		return
	}
	if conversation.Flag(data, conversation.KeyAskDiscarded) {
		if err := discardAgentAction(ctx, svc, chat, userID, action); err != nil {
			slog.ErrorContext(ctx, "discard parked action failed", "err", err)
		}
		return
	}

	answers := flow.DecodeOpenQuestions(data)
	payload, resolved := applyAnswers(action, answers)
	if !resolved {
		// La respuesta no cerró la pregunta: es texto libre que no nombra ninguno
		// de los candidatos. Antes se volvía a preguntar LO MISMO, o sea que
		// escribir gastaba presupuesto y no cambiaba nada. Ahora se busca de
		// nuevo, sumando lo que acaba de escribir al mensaje original.
		//
		// Un error acá NO corta: se loguea y se reabre igual. Reabrir la pregunta
		// vieja es pobre, pero perder la acción es peor.
		if err := researchCandidates(svc, userID, action, answers); err != nil {
			slog.ErrorContext(ctx, "re-search candidates failed", "err", err, "user_id", userID)
		}
		if err := openAskUser(ctx, svc, chat, userID, action, flow.AskBudget(data)); err != nil {
			slog.ErrorContext(ctx, "reopen ask_user failed", "err", err)
			svc.SendText(ctx, chat, flow.MsgSomethingBroke)
		}
		return
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		slog.ErrorContext(ctx, "marshal resolved payload failed", "err", err)
		svc.SendText(ctx, chat, flow.MsgSomethingBroke)
		return
	}
	action.Payload = raw
	if err := resumeAgentAction(ctx, svc, chat, userID, action); err != nil {
		slog.ErrorContext(ctx, "resume parked action failed", "err", err, "tool", action.Tool)
		svc.SendText(ctx, chat, flow.MsgSomethingBroke)
	}
}

// openAction devuelve la acción que el ask_user abierto estaba resolviendo.
// Con WIP=1 es siempre la próxima de la cola, pero se verifica el id igual: si
// no coincide, algo se desincronizó y actuar sería actuar sobre otra cosa.
func openAction(svc agentServices, userID uint64, data conversation.Data) (*pendingaction.PendingAction, error) {
	if !svc.ActionsEnabled() {
		return nil, errors.New("no pending action repository")
	}
	action, err := svc.ActionsNextForUser(userID)
	if err != nil {
		return nil, err
	}
	if strconv.FormatUint(action.ID, 10) != conversation.StringOrEmpty(data[conversation.KeyActionID]) {
		return nil, fmt.Errorf("ask_user was answering action %s, queue head is %d", conversation.StringOrEmpty(data[conversation.KeyActionID]), action.ID)
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
			payload.ChangeAnswer = q.Answer
			// Tocar un botón nombra el CAMPO; escribir nombra el VALOR. La
			// diferencia decide si todavía queda algo por preguntar o si ya
			// hicimos todo lo que podíamos.
			if indexOf(q.Options, q.Answer) >= 0 {
				payload.PickedChangeField = true
				payload.PickedField = string(changeFieldForLabel(q.Answer))
			} else {
				payload.GaveChangeValue = true
				// Con el campo elegido por botón y el valor recién escrito, la app
				// tiene la corrección ENTERA. No hay nada que interpretar, así que
				// no se llama al modelo: se arma el cambio acá.
				//
				// El monto no pasa por acá — se escribe derecho, sin botón, y lo
				// resuelve amountOnlyCorrection.
				if payload.PickedField != "" {
					payload.Changes = []correctionChange{{
						Field: changeField(payload.PickedField), Op: opSet, Value: q.Answer,
					}}
				}
			}
		}
	}
	// La pregunta de "qué cambiar" se parkea con el candidato YA elegido, así
	// que esto la da por resuelta apenas contesta algo. Exigir además un Change
	// no vacío dejaría sin resolver a DELETE, que nunca lleva uno.
	return payload, payload.Chosen >= 0
}

// researchCandidates vuelve a buscar con el texto original MÁS lo que el usuario
// acaba de escribir, y pisa los candidatos de la acción con el resultado.
//
// Sale sin tocar nada —y sin error— cuando no hay con qué buscar o cuando la
// búsqueda no encontró nada: en los dos casos, dejar la acción como estaba y
// reabrir la pregunta vieja es mejor que vaciarle los candidatos.
func researchCandidates(svc agentServices, userID uint64, action *pendingaction.PendingAction, answers []pendingaction.OpenQuestion) error {
	var payload agentPayload
	if err := json.Unmarshal(action.Payload, &payload); err != nil {
		return fmt.Errorf("research: payload: %w", err)
	}
	answer := ""
	for _, q := range answers {
		if q.Key == questionKeyCandidate && q.Answer != "" {
			answer = q.Answer
		}
	}
	if answer == "" || payload.SearchText == "" {
		return nil
	}

	searchText := payload.SearchText + " " + answer
	groups, err := resolveCandidates(svc, userID, searchText, payload.DateFrom, payload.DateTo)
	if err != nil {
		return fmt.Errorf("research: resolve: %w", err)
	}
	if len(groups) == 0 {
		return nil
	}

	candidates := make([]flow.CandidateGroup, 0, len(groups))
	options := make([]string, 0, len(groups))
	for _, g := range groups {
		candidates = append(candidates, toCandidateGroup(g))
		options = append(options, candidateLabel(g))
	}
	payload.Candidates = candidates
	payload.Chosen = -1

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("research: marshal payload: %w", err)
	}

	question := flow.MsgPickUpdateCandidate(nil)
	if action.Tool == orchestrator.ToolDeleteMovements {
		question = flow.MsgPickDeleteCandidate(nil)
	}
	// Mismo criterio que park (agent_executor.go): si el primero no matchea
	// textualmente, la lista salió del fallback por recencia y el cartel no
	// puede decir "encontré parecidos" sobre filas que no se parecen a nada.
	if !matchesMessage(groups[0], searchText) {
		question = flow.MsgPickRecentFallback
	}
	rawQuestions, err := json.Marshal([]pendingaction.OpenQuestion{{
		Key: questionKeyCandidate, Prompt: question + " " + flow.MsgCanRetypeToSearch, Options: options,
	}})
	if err != nil {
		return fmt.Errorf("research: marshal questions: %w", err)
	}

	action.Payload = rawPayload
	action.Questions = rawQuestions
	return svc.ActionsUpdate(action)
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
func resumeAgentAction(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64, action *pendingaction.PendingAction) error {
	var payload agentPayload
	if err := json.Unmarshal(action.Payload, &payload); err != nil {
		return fmt.Errorf("resume: payload: %w", err)
	}

	// El candidato se valida ANTES de borrar: un payload corrupto no se tira en
	// silencio, se deja en la cola y falla ruidoso. Un CREATE se saltea el
	// chequeo porque no tiene candidatos — lo que viaja es el seed a medio
	// resolver.
	// Una corrección en lote no tiene candidato elegido —el cambio va sobre
	// todos— así que Chosen se queda en -1 y chosenCandidate lo rechazaría.
	var chosen flow.CandidateGroup
	if action.Tool != orchestrator.ToolRecordMovements && !isBatchCorrection(payload) {
		var err error
		if chosen, err = chosenCandidate(payload); err != nil {
			return err
		}
	}

	// Se borra ANTES de abrir el gate: si el gate falla, el usuario vuelve a
	// escribir — pero una acción que quedó en la cola bloquearía la siguiente
	// para siempre.
	if err := svc.ActionsDelete(action.ID); err != nil {
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
		flowName := flow.MovementCreateFlowName
		if _, gated := payload.Seed[conversation.KeyGatePrompt]; gated {
			flowName = flow.MovementNegativeConfirmFlowName
		}
		return svc.StartFlow(ctx, chat, userID, flowName, seed, "drain: start "+flowName)
	case orchestrator.ToolCorrectMovement:
		if len(payload.Changes) > 0 {
			groups := []flow.CandidateGroup{chosen}
			if isBatchCorrection(payload) {
				groups = payload.Candidates
			}
			return applyStructuredCorrection(ctx, svc, chat, userID, payload, groups)
		}
		// `changes` vacío = el usuario dijo QUÉ movimiento pero no QUÉ cambiarle
		// ("editá los movimientos de hoy"). Se pregunta, que es la primitiva para
		// la que se construyó el loop.
		//
		// Acá NO se llama al modelo. La segunda llamada le pedía re-emitir la fila
		// ENTERA y eso falla solo: el 2026-08-12, ante "Era pollo", devolvió las
		// once columnas menos `date` y Groq la rechazó con un 400 — la corrección
		// se perdió entera por un campo que nadie había pedido tocar. Un diff no
		// puede fallar así.
		if len(payload.Changes) == 0 && !payload.GaveChangeValue {
			return parkChangeQuestion(ctx, svc, chat, userID, payload.Change, chosen.TransactionID, chosen.OldIDs, chosen.Rows,
				ChangeAsk{pickedField: payload.PickedChangeField, gaveValue: payload.GaveChangeValue, answer: payload.ChangeAnswer, field: payload.PickedField})
		}
		return proceedToUpdateConfirm(ctx, svc, chat, userID, payload.Change, chosen.TransactionID, chosen.OldIDs, chosen.Rows, ChangeAsk{pickedField: payload.PickedChangeField, gaveValue: payload.GaveChangeValue, answer: payload.ChangeAnswer, field: payload.PickedField})
	case orchestrator.ToolDeleteMovements:
		seed := conversation.Data{
			conversation.KeyCandidateGroups: encodeCandidateGroupList([]flow.CandidateGroup{chosen}),
			conversation.KeyResolvedIndex:   "0",
		}
		return svc.StartFlow(ctx, chat, userID, flow.MovementDeleteFlowName, seed, "drain: start movement_delete flow")
	default:
		return fmt.Errorf("resume: tool %q has no resume path", action.Tool)
	}
}

// chosenCandidate saca el candidato elegido. El chequeo de rango vive acá y no
// arriba porque sólo aplica a las tools que TIENEN candidatos: un CREATE nunca
// los tiene, y el chequeo genérico lo rechazaba antes de llegar a su rama.
func chosenCandidate(payload agentPayload) (flow.CandidateGroup, error) {
	if payload.Chosen < 0 || payload.Chosen >= len(payload.Candidates) {
		return flow.CandidateGroup{}, fmt.Errorf("resume: candidate %d out of range (%d)", payload.Chosen, len(payload.Candidates))
	}
	return payload.Candidates[payload.Chosen], nil
}

// discardAgentAction tira la acción entera y NOMBRA lo que se cayó. Tirar un
// movimiento en silencio es exactamente la falla que todo esto viene a evitar.
func discardAgentAction(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64, action *pendingaction.PendingAction) error {
	slog.InfoContext(ctx, "parked action discarded: budget exhausted",
		"tool", action.Tool, "user_id", userID, "budget", action.Budget)
	resolveMetric(ctx, svc, userID, flow.OutcomeCreateFailed)
	dropAgentAction(ctx, svc, chat, userID, action, msgAgentActionDiscarded(describeAction(action)))
	return nil
}

// dropAgentAction saca la acción de la cola, avisa, y sigue con la que venga
// atrás — si no, una cancelación dejaría el resto de la cola trabado.
func dropAgentAction(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64, action *pendingaction.PendingAction, message string) {
	if err := svc.ActionsDelete(action.ID); err != nil {
		slog.ErrorContext(ctx, "delete parked action failed", "err", err)
	}
	svc.SendText(ctx, chat, message)
	if err := drainNextAgentAction(ctx, svc, chat, userID); err != nil {
		slog.ErrorContext(ctx, "drain after drop failed", "err", err)
	}
}

func msgAgentActionDiscarded(what string) string {
	return fmt.Sprintf(messages.MsgAgentActionDiscardedTemplate, what)
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
