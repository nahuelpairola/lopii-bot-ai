package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messages"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
)

// startAgentLoop resuelve un mensaje con el loop unificado.
//
// Desde la etapa 5 lo alcanza TODO: handleFreeText no hace otra cosa que llamar
// acá. No hay router que filtre antes, así que este es el único lugar donde se
// decide qué se hace con un mensaje, y lo decide el loop eligiendo herramienta.
func startAgentLoop(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64, text string) error {
	// El loop tarda más que una sola llamada, y el silencio se lee como colgado.
	_ = chat.Typing(ctx)

	// El toolbox y el prompt salen de la MISMA lista: el prompt no puede nombrar
	// una tool que no se manda. Ver wiredAgentTools.
	tools := wiredAgentTools()
	prompt, taxonomy, err := buildAgentSystemPrompt(svc, userID, tools)
	if err != nil {
		svc.SendText(ctx, chat, flow.MsgCouldNotLoad)
		return err
	}

	// Best-effort, igual que en QUERY: si el historial no carga, se corre sin él.
	turns, _ := svc.ChatHistoryRecent(userID)
	history := make([]orchestrator.QueryTurn, len(turns))
	for i, t := range turns {
		history[i] = orchestrator.QueryTurn{Question: t.Question, Answer: t.Answer}
	}

	executor := newAgentExecutor(ctx, svc, userID, text, taxonomy)
	answer, err := svc.Run(ctx, prompt, text, history, tools, executor.execute)

	// El intent_event se ABRE acá, después del loop, porque ya no hay router que
	// diga el intent de antemano — lo dice la primera tool que el loop eligió.
	// Log abre y Resolve cierra, y Resolve NO toca la columna intent: por eso el
	// orden es al revés que antes.
	//
	// Pero un REPLAY no abre nada: el evento del mensaje ya existe y sigue
	// pendiente, esperando justamente a que el drenaje lo resuelva. Sin esta
	// guarda cada reintento del 429 escribía un `unclear` de más — medido en vivo
	// el 2026-08-12: "10k panaderia" llevaba TRES eventos fallidos, de 0 tokens
	// cada uno, antes de siquiera procesarse. Es ruido puro en la única columna
	// que lee el portón de la etapa, y hace que el bot se vea peor cuanto más
	// apretado esté el cupo.
	if svc.IsReplaying(ctx) {
		// El replay NO abre un evento nuevo: el del mensaje ya existe y sigue
		// pendiente. Lo que sí hace es completarle el intent, que al encolarse no
		// se sabía — el cupo cortó antes de que el modelo eligiera herramienta.
		setQueuedIntent(ctx, svc, userID, intentForExecutor(executor, err))
	} else {
		logIntent(ctx, svc, userID, text, intentForExecutor(executor, err), err)
	}

	if err != nil {
		// Un 429 DESPUÉS de escribir no se encola: el drenaje volvería a correr el
		// mensaje y la plata quedaría registrada dos veces. Se informa el éxito
		// parcial y se corta ahí. Spec 8.2.
		if executor.wrote {
			resolveMetric(ctx, svc, userID, flow.OutcomeCreateInserted, collectMovementIDs(executor.inserted)...)
			svc.SendText(ctx, chat, messages.MsgPartialSuccessAfterWrite)
			slog.WarnContext(ctx, "agent loop failed after a write: not queued", "user_id", userID, "err", err)
			return nil
		}
		// El 429 encola el mensaje para reintentarlo: ahí el intent_event tiene
		// que seguir pendiente, porque la historia no terminó.
		if handled, oerr := svc.HandleGroqError(ctx, chat, userID, text, err); handled {
			return oerr
		}
		resolveMetric(ctx, svc, userID, outcomeLoopErrored)
		slog.ErrorContext(ctx, "agent loop failed", "user_id", userID, "err", err)
		svc.SendText(ctx, chat, flow.MsgSomethingBroke)
		return fmt.Errorf("agent loop: %w", err)
	}

	// Las dos tools que delegan a otro subsistema. Van antes de la narración: la
	// respuesta se la da el que atiende, no el loop.
	if executor.answerQuery {
		return svc.FinishAnswerQuery(ctx, chat, userID, text)
	}
	if executor.settingsArea != "" {
		return svc.FinishManageSettings(ctx, chat, userID, text, executor.settingsArea)
	}

	// La copia nuestra (ayuda, pedir reescritura) le gana a la narración del
	// modelo: es texto tuneado y tiene que salir textual.
	if executor.reply != "" {
		if len(executor.replyButtons) > 0 {
			svc.SendPrompt(ctx, chat, conversation.Prompt{Text: executor.reply, Buttons: executor.replyButtons})
		} else {
			svc.SendText(ctx, chat, executor.reply)
		}
	} else if narration := strings.TrimSpace(answer); narration != "" {
		svc.SendText(ctx, chat, narration)
	}

	if len(executor.parked) > 0 {
		if err := parkAgentActions(ctx, svc, userID, executor.parked); err != nil {
			resolveMetric(ctx, svc, userID, outcomeParkFailed)
			slog.ErrorContext(ctx, "park agent actions failed", "user_id", userID, "err", err)
			svc.SendText(ctx, chat, flow.MsgSomethingBroke)
			return err
		}
	}
	// El hilo compartido: hasta el 2026-08-12 este camino LEÍA chat_turns y no
	// escribía nunca — sólo query.go llamaba a Append. El paquete chathistory
	// dice que "desde la etapa 2 todos los intents comparten el mismo hilo", y no
	// era cierto: cada CREATE, UPDATE y DELETE corría con el hilo vacío.
	//
	// La resolución de referencias NO depende de esto (para eso está el bloque de
	// entidades recientes, que se reconstruye desde la base y no se desordena con
	// un replay); el hilo es para el resto del contexto conversacional.
	if reply := firstNonEmpty(executor.reply, strings.TrimSpace(answer)); reply != "" {
		if err := svc.ChatHistoryAppend(userID, text, reply); err != nil {
			slog.WarnContext(ctx, "chat history append failed", "user_id", userID, "err", err)
		}
	}

	resolveAgentLoopMetric(ctx, svc, userID, executor)

	// Destapa la cola acá mismo: este mensaje no abrió ningún flujo, así que no
	// va a haber un terminal que dispare el drenaje más tarde.
	return drainNextAgentAction(ctx, svc, chat, userID)
}

// firstNonEmpty devuelve el primero que no esté vacío.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// resolveAgentLoopMetric cierra el intent_event del turno.
//
// Sin esto el evento queda 'pending' y el PRÓXIMO mensaje del usuario lo pisa a
// 'abandoned'. El portón de la etapa es "update_confirmed sube y abandoned no
// sube": dejarlo pendiente hace que cada turno del loop que no parkea nada
// cuente como un abandono, y el portón daría negativo aunque todo funcione.
func resolveAgentLoopMetric(ctx context.Context, svc agentServices, userID uint64, ex *agentExecutor) {
	if len(ex.parked) > 0 {
		// Hay algo abierto: lo resuelve el gate cuando el usuario decida. Ese es
		// el WIP=1 — un solo pendiente vivo por vez.
		return
	}
	switch {
	case len(ex.inserted) > 0:
		// Va PRIMERO: el reply de un CREATE limpio es el recibo, que no matchea
		// ninguna de las copys de abajo y caería en el fracaso genérico.
		resolveMetric(ctx, svc, userID, flow.OutcomeCreateInserted, collectMovementIDs(ex.inserted)...)
	case ex.reply == messages.MsgHelp:
		resolveMetric(ctx, svc, userID, outcomeHelpShown)
	case ex.reply == messages.MsgAskRewrite:
		resolveMetric(ctx, svc, userID, outcomeUnclear)
	case ex.noCandidates:
		resolveMetric(ctx, svc, userID, outcomeNoCandidates)
	default:
		// El loop narró sin hacer nada. Es un fracaso, y tiene que verse como
		// tal: es justo el caso que hay que poder contar.
		resolveMetric(ctx, svc, userID, outcomeLoopDidNothing)
	}
}

// buildAgentSystemPrompt arma el prompt unificado con las cuentas y la taxonomía
// del usuario. pendingQuestion va vacío: cuando hay una pregunta abierta el que
// está a cargo es ask_user, no este camino.
//
// Devuelve también la taxonomía porque el ejecutor la necesita igual, para
// buildCreateSeed. Traerla dos veces serían dos queries por turno y —peor— dos
// listas que pueden diferir: el modelo clasificaría contra una y el gap se
// marcaría contra la otra.
func buildAgentSystemPrompt(svc agentServices, userID uint64, tools []orchestrator.AgentTool) (string, []orchestrator.TaxonomyEntry, error) {
	subs, err := svc.SubcategoriesFindAllForUser(userID)
	if err != nil {
		return "", nil, fmt.Errorf("agent loop: find subcategories: %w", err)
	}
	taxonomy := make([]orchestrator.TaxonomyEntry, 0, len(subs))
	for _, s := range subs {
		taxonomy = append(taxonomy, orchestrator.TaxonomyEntry{Category: s.Category, Subcategory: s.Subcategory, Description: s.Description})
	}

	accs, err := svc.FindUserAccounts(userID)
	if err != nil {
		return "", nil, fmt.Errorf("agent loop: find accounts: %w", err)
	}
	accountOptions := make([]orchestrator.AccountOption, 0, len(accs))
	for _, a := range accs {
		accountOptions = append(accountOptions, orchestrator.AccountOption{ID: uint64(a.ID), Name: a.Name, Currency: a.Currency.String()})
	}

	today := movement.TodayCivil()
	return orchestrator.BuildAgentPrompt(movement.WeekdayEs(today)+" "+today.Format("2006-01-02"), accountOptions, taxonomy, "", tools,
		BuildRecentEntities(svc, userID)), taxonomy, nil
}
