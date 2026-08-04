package messaging

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/orchestrator"
)

// sendText is a small helper that guards every b.SendMessage call with a
// nil check — b is nil in unit tests that exercise these entry points
// directly (see free_text_test.go), matching the same guard pattern
// already used throughout movement_update_flow.go/movement_delete_flow.go.
func (c *controller) sendText(ctx context.Context, b *bot.Bot, chatID int64, text string) {
	if b == nil {
		return
	}
	b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text})
}

// handleFreeText is the entry point for any message with no flow already in
// progress: Call 1 (the router) decides which intent it is, and this dispatches
// to the matching start*. Los start* viven por dominio: start_movement.go,
// start_account.go, start_category.go.
//
// QUERY es la excepción: no abre ningún flow, va derecho al agent loop de
// query.go.
func (c *controller) handleFreeText(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) error {
	result, err := c.orchestrator.ClassifyIntent(ctx, text)
	if err != nil {
		if handled, oerr := c.handleGroqError(ctx, b, chatID, userID, text, err); handled {
			return oerr
		}
		slog.ErrorContext(ctx, "intent classification failed", "user_id", userID, "err", err)
		c.sendText(ctx, b, chatID, msgSomethingBroke)
		return fmt.Errorf("classify intent: %w", err)
	}

	slog.InfoContext(ctx, "intent classified",
		"user_id", userID,
		"intent", string(result.Intent),
	)

	c.logIntent(ctx, userID, text, result.Intent)

	switch result.Intent {
	case orchestrator.IntentQuery:
		answered, qErr := c.handleQuery(ctx, b, chatID, userID, text)
		if answered {
			c.resolveMetric(ctx, userID, outcomeQueryAnswered)
		} else {
			if qErr != nil {
				slog.ErrorContext(ctx, "query failed", "user_id", userID, "err", qErr)
			}
			c.resolveMetric(ctx, userID, outcomeQueryFailed)
		}
		return qErr
	case orchestrator.IntentCreate:
		// El interruptor existe porque las etapas 2 y 3 despliegan juntas: si
		// create_inserted cae, apagarlo devuelve CREATE al camino viejo sin tocar
		// la etapa 2 y sin redeploy. Ver config.Agent.RouteCreateToLoop.
		if c.routeCreateToLoop {
			return c.startAgentLoop(ctx, b, chatID, userID, text)
		}
		return c.startMovementCreate(ctx, b, chatID, userID, text)
	// UPDATE y DELETE son los dos intents que van por el loop unificado en la
	// etapa 2 — los dos más rotos (9,5% y 62%), y el conjunto más chico que
	// ejercita parking, cola, un gate de plata y respuestas de texto libre.
	// Los otros ocho siguen por su camino de siempre.
	case orchestrator.IntentUpdate, orchestrator.IntentDelete:
		return c.startAgentLoop(ctx, b, chatID, userID, text)
	case orchestrator.IntentAccountManage:
		return c.startAccountManage(ctx, b, chatID, userID, text)
	case orchestrator.IntentCreateCategory:
		return c.startSubcategorySetup(ctx, b, chatID, userID, text)
	case orchestrator.IntentCategoryManage:
		return c.startCategoryManage(ctx, b, chatID, userID)
	case orchestrator.IntentReminderSet:
		return c.startReminderSetup(ctx, b, chatID, userID)
	case orchestrator.IntentHelp:
		c.sendText(ctx, b, chatID, msgHelp)
		return nil
	case orchestrator.IntentUnclear:
		c.sendText(ctx, b, chatID, msgAskRewrite)
		return nil
	default:
		c.sendText(ctx, b, chatID, msgSomethingBroke)
	}
	return nil
}
