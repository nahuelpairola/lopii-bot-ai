package messaging

import (
	"context"
	"strings"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)

// Los finishes de CATEGORY_MANAGE viven en flow (category_finish.go). Estos
// delegadores conservan los nombres de borde mientras los tests y
// handleFlowFinished los usen.
func (c *controller) finishCategoryManagePickFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	flow.FinishCategoryManagePickFlow(ctx, c, b, chatID, data)
}

func (c *controller) finishCategoryManageTargetFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	flow.FinishCategoryManageTargetFlow(ctx, c, b, chatID, data)
}

func (c *controller) proceedToCategoryTarget(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) error {
	return flow.ProceedToCategoryTarget(ctx, c, b, chatID, data)
}

// suggestMergeTarget le pregunta al LLM a qué subcategoría existente se parece
// la que el usuario quiere sacar, reusando ClassifyCategoryCreate: ya hace
// exactamente esa pregunta ("¿esto que me describís ya existe?").
//
// Devuelve nil ante cualquier duda — error, timeout, propuesta en vez de match,
// o un match que resuelve al propio origen. nil significa "sin sugerencia", y
// el flujo cae al picker manual. Nunca bloquea.
//
// Vive en el borde a propósito: es la única parte de CATEGORY_MANAGE que toca
// el LLM, y flow no conoce al orchestrator. flow la alcanza via runner
// (SuggestMergeTarget).
func (c *controller) suggestMergeTarget(ctx context.Context, userID, sourceID uint64, data conversation.Data) *subcategory.Subcategory {
	subs, err := c.subcategories.FindAllForUser(userID)
	if err != nil {
		return nil
	}

	taxonomy := make([]orchestrator.TaxonomyEntry, 0, len(subs))
	var sourceDescription string
	for _, s := range subs {
		if subcategory.IsReserved(s.Category) {
			continue
		}
		if uint64(s.ID) == sourceID {
			sourceDescription = s.Description
			continue // sin esta exclusión el LLM se matchearía a sí mismo
		}
		taxonomy = append(taxonomy, orchestrator.TaxonomyEntry{
			Category: s.Category, Subcategory: s.Subcategory, Description: s.Description,
		})
	}

	descriptions, _ := c.movements.TopDescriptionsBySubcategory(userID, sourceID, flow.TopDescriptionsForSuggestion)
	text := mergeSuggestionText(
		conversation.StringOrEmpty(data[conversation.KeySourceCategory]),
		conversation.StringOrEmpty(data[conversation.KeySourceSubcategory]),
		sourceDescription,
		descriptions,
	)

	res, err := c.orchestrator.ClassifyCategoryCreate(ctx, text, taxonomy)
	if err != nil || res.Match == nil {
		return nil
	}
	found, err := c.subcategories.FindByCategoryAndSubcategory(userID, res.Match.Category, res.Match.Subcategory)
	if err != nil || found == nil || uint64(found.ID) == sourceID {
		return nil // alucinación, se propuso a sí misma, o no resolvió a nada
	}
	return found
}

// mergeSuggestionText arma lo que ve el LLM. Los comercios entran como contexto
// de la MISMA llamada, no como una clasificación aparte: clasificar movimientos
// daría una respuesta por movimiento, y esta operación es por subcategoría,
// todo o nada.
func mergeSuggestionText(category, subcategoryName, description string, samples []string) string {
	text := category + " / " + subcategoryName
	if description != "" {
		text += " — " + description
	}
	if len(samples) > 0 {
		text += " — gastos en: " + strings.Join(samples, ", ")
	}
	return text
}
