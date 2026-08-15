package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)

// finishCategoryManagePickFlow corre cuando el usuario eligió (o no) el origen.
// Si eligió, hace el puente al flujo 2.
func (c *controller) finishCategoryManagePickFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		c.resolveMetric(ctx, data.UserID(), outcomeCategoryManageCancelled)
		c.sendText(ctx, b, chatID, msgFlowCancelled)
		return
	}
	if err := c.proceedToCategoryTarget(ctx, b, chatID, data); err != nil {
		slog.ErrorContext(ctx, "category manage: proceed to target", "err", err)
		c.sendText(ctx, b, chatID, msgSomethingBroke)
	}
}

// proceedToCategoryTarget es el puente entre los dos flujos: cuenta los
// movimientos del origen y, solo si hay alguno, pide una sugerencia de destino.
// Después arranca el flujo 2 con todo eso sembrado.
func (c *controller) proceedToCategoryTarget(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) error {
	userID := data.UserID()
	sourceID, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeySourceSubcategoryID]), 10, 64)
	if err != nil {
		return fmt.Errorf("category manage: source id inválido: %w", err)
	}

	count, err := c.movements.CountBySubcategory(userID, sourceID)
	if err != nil {
		return fmt.Errorf("category manage: contar movimientos: %w", err)
	}

	seed := conversation.Data{
		conversation.KeySourceSubcategoryID: conversation.StringOrEmpty(data[conversation.KeySourceSubcategoryID]),
		conversation.KeySourceCategory:      conversation.StringOrEmpty(data[conversation.KeySourceCategory]),
		conversation.KeySourceSubcategory:   conversation.StringOrEmpty(data[conversation.KeySourceSubcategory]),
		conversation.KeyMovementCount:       strconv.FormatInt(count, 10),
	}

	if count > 0 {
		if sug := c.suggestMergeTarget(ctx, userID, sourceID, data); sug != nil {
			seed[conversation.KeySuggestedSubcategoryID] = strconv.FormatUint(uint64(sug.ID), 10)
			seed[conversation.KeySuggestedCategory] = sug.Category
			seed[conversation.KeySuggestedSubcategory] = sug.Subcategory
		}
	}

	prompt, err := c.engine.StartWithData(userID, categoryManageTargetFlowName, seed)
	if err != nil {
		return fmt.Errorf("start category_manage_target flow: %w", err)
	}
	c.sendPrompt(ctx, b, chatID, prompt)
	return nil
}

// suggestMergeTarget le pregunta al LLM a qué subcategoría existente se parece
// la que el usuario quiere sacar, reusando ClassifyCategoryCreate: ya hace
// exactamente esa pregunta ("¿esto que me describís ya existe?").
//
// Devuelve nil ante cualquier duda — error, timeout, propuesta en vez de match,
// o un match que resuelve al propio origen. nil significa "sin sugerencia", y
// el flujo cae al picker manual. Nunca bloquea.
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

	descriptions, _ := c.movements.TopDescriptionsBySubcategory(userID, sourceID, topDescriptionsForSuggestion)
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

// finishCategoryManageTargetFlow aplica lo que el confirm ya le mostró al
// usuario. Es pura ejecución: el gate de confirmación quedó atrás, dentro del
// flujo.
//
// El orden importa. Primero se mueven los movimientos, después se borra la
// categoría. Al revés, un fallo intermedio dejaría movimientos apuntando a una
// fila borrada. En este orden, un fallo del borrado deja la categoría vacía —
// un estado consistente que el usuario puede reintentar.
func (c *controller) finishCategoryManageTargetFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) || !conversation.Flag(data, conversation.KeyConfirmed) {
		c.resolveMetric(ctx, data.UserID(), outcomeCategoryManageCancelled)
		c.sendText(ctx, b, chatID, msgFlowCancelled)
		return
	}

	userID := data.UserID()
	sourceID, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeySourceSubcategoryID]), 10, 64)
	if err != nil {
		c.sendText(ctx, b, chatID, msgSomethingBroke)
		return
	}

	targetRaw := conversation.StringOrEmpty(data[conversation.KeyTargetSubcategoryID])
	if targetRaw != "" {
		targetID, err := strconv.ParseUint(targetRaw, 10, 64)
		if err != nil {
			c.sendText(ctx, b, chatID, msgSomethingBroke)
			return
		}
		if err := c.movements.ReassignSubcategory(userID, sourceID, targetID); err != nil {
			slog.ErrorContext(ctx, "category manage: reassign", "err", err)
			c.sendText(ctx, b, chatID, msgCouldNotSave("el cambio"))
			return
		}
	}

	// Delete devuelve ErrSubcategoryNotFound cuando no borró nada (fila ajena,
	// global o inexistente). Hay que cortar acá: decirle "listo, la saqué" a
	// alguien cuya categoría sigue estando sería mentirle.
	if err := c.subcategories.Delete(userID, sourceID); err != nil {
		slog.ErrorContext(ctx, "category manage: delete", "err", err)
		c.sendText(ctx, b, chatID, msgCouldNotDelete("tu categoría"))
		return
	}
	// El Cache es read-through: sin Reload la categoría borrada seguiría
	// apareciendo hasta el próximo reinicio del server.
	if err := c.subcategories.Reload(); err != nil {
		slog.ErrorContext(ctx, "category manage: cache reload", "err", err)
	}

	c.resolveMetric(ctx, userID, outcomeCategoryManageApplied)
	if targetRaw == "" {
		c.sendText(ctx, b, chatID, msgCategoryManageDeleted(sourceLabel(data)))
		return
	}
	c.sendText(ctx, b, chatID, msgCategoryManageMerged(
		conversation.StringOrEmpty(data[conversation.KeyMovementCount]), sourceLabel(data), targetLabel(data)))
}
