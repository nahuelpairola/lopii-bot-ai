package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"lopiibot.com/internal/orchestrator"
)

// startAgentLoop resuelve un mensaje con el loop unificado.
//
// En esta etapa lo alcanzan sólo UPDATE y DELETE. El router sigue vivo y sigue
// decidiendo qué intents llegan hasta acá: ese es el truco de las etapas, y es
// lo que hace que esto se pueda bisectar. Los otros ocho casos no se tocan.
func (c *controller) startAgentLoop(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) error {
	// El loop tarda más que una sola llamada, y el silencio se lee como colgado.
	c.sendTyping(ctx, b, chatID)

	// El toolbox y el prompt salen de la MISMA lista: el prompt no puede nombrar
	// una tool que no se manda. Ver wiredAgentTools.
	tools := wiredAgentTools()
	prompt, err := c.buildAgentSystemPrompt(userID, tools)
	if err != nil {
		c.sendText(ctx, b, chatID, msgCouldNotLoad)
		return err
	}

	// Best-effort, igual que en QUERY: si el historial no carga, se corre sin él.
	turns, _ := c.chatHistory.Recent(userID)
	history := make([]orchestrator.QueryTurn, len(turns))
	for i, t := range turns {
		history[i] = orchestrator.QueryTurn{Question: t.Question, Answer: t.Answer}
	}

	executor := newAgentExecutor(c, userID, text)
	answer, err := c.orchestrator.Run(ctx, prompt, text, history, tools, executor.execute)
	if err != nil {
		// Un 429 DESPUÉS de escribir no se encola: el drenaje volvería a correr el
		// mensaje y la plata quedaría registrada dos veces. Se informa el éxito
		// parcial y se corta ahí. Spec 8.2.
		if executor.wrote {
			c.resolveMetric(ctx, userID, outcomeCreateInserted, collectMovementIDs(executor.inserted)...)
			c.sendText(ctx, b, chatID, msgPartialSuccessAfterWrite)
			slog.WarnContext(ctx, "agent loop failed after a write: not queued", "user_id", userID, "err", err)
			return nil
		}
		// El 429 encola el mensaje para reintentarlo: ahí el intent_event tiene
		// que seguir pendiente, porque la historia no terminó.
		if handled, oerr := c.handleGroqError(ctx, b, chatID, userID, text, err); handled {
			return oerr
		}
		c.resolveMetric(ctx, userID, outcomeUpdateFailed)
		slog.ErrorContext(ctx, "agent loop failed", "user_id", userID, "err", err)
		c.sendText(ctx, b, chatID, msgSomethingBroke)
		return fmt.Errorf("agent loop: %w", err)
	}

	// La copia nuestra (ayuda, pedir reescritura) le gana a la narración del
	// modelo: es texto tuneado y tiene que salir textual.
	if executor.reply != "" {
		c.sendText(ctx, b, chatID, executor.reply)
	} else if narration := strings.TrimSpace(answer); narration != "" {
		c.sendText(ctx, b, chatID, narration)
	}

	if len(executor.parked) > 0 {
		if err := c.parkAgentActions(ctx, userID, executor.parked); err != nil {
			c.resolveMetric(ctx, userID, outcomeUpdateFailed)
			slog.ErrorContext(ctx, "park agent actions failed", "user_id", userID, "err", err)
			c.sendText(ctx, b, chatID, msgSomethingBroke)
			return err
		}
	}
	c.resolveAgentLoopMetric(ctx, userID, executor)

	// Destapa la cola acá mismo: este mensaje no abrió ningún flujo, así que no
	// va a haber un terminal que dispare el drenaje más tarde.
	return c.drainNextAgentAction(ctx, b, chatID, userID)
}

// resolveAgentLoopMetric cierra el intent_event del turno.
//
// Sin esto el evento queda 'pending' y el PRÓXIMO mensaje del usuario lo pisa a
// 'abandoned'. El portón de la etapa es "update_confirmed sube y abandoned no
// sube": dejarlo pendiente hace que cada turno del loop que no parkea nada
// cuente como un abandono, y el portón daría negativo aunque todo funcione.
func (c *controller) resolveAgentLoopMetric(ctx context.Context, userID uint64, ex *agentExecutor) {
	if len(ex.parked) > 0 {
		// Hay algo abierto: lo resuelve el gate cuando el usuario decida. Ese es
		// el WIP=1 — un solo pendiente vivo por vez.
		return
	}
	switch {
	case ex.reply == msgHelp:
		c.resolveMetric(ctx, userID, outcomeHelpShown)
	case ex.reply == msgAskRewrite:
		c.resolveMetric(ctx, userID, outcomeUnclear)
	case ex.noCandidates:
		c.resolveMetric(ctx, userID, outcomeNoCandidates)
	default:
		// El loop narró sin hacer nada. Es un fracaso, y tiene que verse como
		// tal: es justo el caso que hay que poder contar.
		c.resolveMetric(ctx, userID, outcomeUpdateFailed)
	}
}

// buildAgentSystemPrompt arma el prompt unificado con las cuentas y la taxonomía
// del usuario. pendingQuestion va vacío: cuando hay una pregunta abierta el que
// está a cargo es ask_user, no este camino.
func (c *controller) buildAgentSystemPrompt(userID uint64, tools []orchestrator.AgentTool) (string, error) {
	subs, err := c.subcategories.FindAllForUser(userID)
	if err != nil {
		return "", fmt.Errorf("agent loop: find subcategories: %w", err)
	}
	taxonomy := make([]orchestrator.TaxonomyEntry, 0, len(subs))
	for _, s := range subs {
		taxonomy = append(taxonomy, orchestrator.TaxonomyEntry{Category: s.Category, Subcategory: s.Subcategory, Description: s.Description})
	}

	accs, err := c.accounts.FindByUserID(userID)
	if err != nil {
		return "", fmt.Errorf("agent loop: find accounts: %w", err)
	}
	accountOptions := make([]orchestrator.AccountOption, 0, len(accs))
	for _, a := range accs {
		accountOptions = append(accountOptions, orchestrator.AccountOption{ID: uint64(a.ID), Name: a.Name, Currency: a.Currency.String()})
	}

	return orchestrator.BuildAgentPrompt(time.Now().Format("2006-01-02"), accountOptions, taxonomy, "", tools), nil
}

// sendTyping avisa que el bot está pensando. Best-effort: que falle el aviso no
// puede tumbar el pedido real.
func (c *controller) sendTyping(ctx context.Context, b *bot.Bot, chatID int64) {
	if b == nil {
		return
	}
	if _, err := b.SendChatAction(ctx, &bot.SendChatActionParams{ChatID: chatID, Action: models.ChatActionTyping}); err != nil {
		slog.DebugContext(ctx, "send chat action failed", "err", err)
	}
}
