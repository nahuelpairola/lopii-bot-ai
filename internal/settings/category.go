package settings

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)

// OutcomeCategoryManageNoOwn: el usuario pidió sacar una categoría pero no creó
// ninguna. El bot entendió y respondió bien; no es una falla. Exportado porque
// los tests del borde lo comparan.
const OutcomeCategoryManageNoOwn = "category_manage_no_own"

// startSubcategoryWizard starts the classic 7-step wizard fresh — the
// fallback whenever the LLM path can't produce a trustworthy match/proposal.
func startSubcategoryWizard(ctx context.Context, s Services, b *bot.Bot, chatID int64, userID uint64) error {
	return s.StartFlow(ctx, b, chatID, userID, flow.SubcategorySetupFlowName, nil, "start subcategory_setup flow")
}

// StartSubcategorySetup resolves a CREATE_CATEGORY message with the LLM
// first: an existing-entry match offers reuse (the "regalos ya existía"
// case), a full proposal collapses the 7-step wizard into one confirmation.
// Any doubt → the classic wizard, never a dead end.
func StartSubcategorySetup(ctx context.Context, s Services, b *bot.Bot, chatID int64, userID uint64, text string) error {
	slog.InfoContext(ctx, "flow started", "flow", flow.SubcategorySetupFlowName, "user_id", userID)
	subs, err := s.SubcategoriesFindAllForUser(userID)
	if err != nil {
		return startSubcategoryWizard(ctx, s, b, chatID, userID)
	}
	taxonomy := make([]orchestrator.TaxonomyEntry, 0, len(subs))
	for _, sub := range subs {
		if subcategory.IsReserved(sub.Category) {
			continue
		}
		taxonomy = append(taxonomy, orchestrator.TaxonomyEntry{Category: sub.Category, Subcategory: sub.Subcategory, Description: sub.Description})
	}

	res, err := s.ClassifyCategoryCreate(ctx, text, taxonomy)
	if err != nil {
		// El 429 se atiende ANTES del wizard. Sin esto, un problema de cupo se
		// disfraza de "no te entendí" y le cobra al usuario las 7 preguntas del
		// wizard por algo que se resuelve solo en segundos.
		if handled, oerr := s.HandleGroqError(ctx, b, chatID, userID, text, err); handled {
			return oerr
		}
		return startSubcategoryWizard(ctx, s, b, chatID, userID)
	}

	if res.Match != nil {
		existing, err := s.FindSubcategory(userID, res.Match.Category, res.Match.Subcategory)
		if err != nil { // hallucinated match → can't offer it
			return startSubcategoryWizard(ctx, s, b, chatID, userID)
		}
		return startCategoryMatchOffer(ctx, s, b, chatID, userID, existing)
	}

	if res.Proposal == nil { // neither match nor proposal usable → never a dead end
		return startSubcategoryWizard(ctx, s, b, chatID, userID)
	}
	p := res.Proposal
	p.Category, p.Subcategory = strings.TrimSpace(p.Category), strings.TrimSpace(p.Subcategory)
	if p.Category == "" || p.Subcategory == "" || subcategory.IsReserved(p.Category) || subcategory.IsReserved(p.Subcategory) {
		return startSubcategoryWizard(ctx, s, b, chatID, userID)
	}
	if existing, err := s.FindSubcategory(userID, p.Category, p.Subcategory); err == nil {
		return startCategoryMatchOffer(ctx, s, b, chatID, userID, existing) // exact duplicate → offer, don't re-create
	}

	isNew := "true"
	if cats, err := s.DistinctCategoriesForUser(userID); err == nil {
		for _, cat := range cats {
			if cat == p.Category {
				isNew = "false"
				break
			}
		}
	}
	icon := strings.TrimSpace(p.Icon)
	if !subcategory.ValidIcon(icon) {
		icon = "" // insertNewSubcategory falls back to IconForCategory / 📂
	}
	seed := conversation.Data{
		conversation.KeyCategory:               p.Category,
		conversation.KeyCategoryIsNew:          isNew,
		conversation.KeyCategoryIcon:           icon,
		conversation.KeySubcategory:            p.Subcategory,
		conversation.KeySubcategoryDescription: strings.TrimSpace(p.Description),
	}
	prompt, err := s.EngineStartWithData(userID, flow.CategoryProposalConfirmFlowName, seed)
	if err != nil {
		return startSubcategoryWizard(ctx, s, b, chatID, userID)
	}
	s.SendPrompt(ctx, b, chatID, prompt)
	return nil
}

// StartCategoryManage arranca el flujo de sacar una categoría propia. Antes de
// nada verifica que el usuario tenga alguna: sin eso el picker mostraría solo
// "Cancelar", que es un callejón sin salida disfrazado de flujo.
func StartCategoryManage(ctx context.Context, s Services, b *bot.Bot, chatID int64, userID uint64) error {
	slog.InfoContext(ctx, "flow started", "flow", flow.CategoryManagePickFlowName, "user_id", userID)

	owned, err := s.FindOwnedSubcategories(userID)
	if err != nil {
		s.SendText(ctx, b, chatID, flow.MsgCouldNotLoad)
		return fmt.Errorf("category manage: find owned: %w", err)
	}
	if len(owned) == 0 {
		// Se resuelve la métrica: el bot entendió y respondió bien. Sin esto el
		// evento queda pendiente y el sweeper lo marca "abandoned", que en las
		// métricas de asertividad se lee como una falla del bot.
		s.ResolveMetric(ctx, userID, OutcomeCategoryManageNoOwn)
		s.SendText(ctx, b, chatID, flow.MsgCategoryManageNoOwn)
		return nil
	}

	return s.StartFlow(ctx, b, chatID, userID, flow.CategoryManagePickFlowName, nil, "start category_manage_pick flow")
}

// startCategoryMatchOffer seeds and starts category_match_offer from an
// existing taxonomy row.
func startCategoryMatchOffer(ctx context.Context, s Services, b *bot.Bot, chatID int64, userID uint64, sub *subcategory.Subcategory) error {
	seed := conversation.Data{
		conversation.KeyCategory:               sub.Category,
		conversation.KeySubcategory:            sub.Subcategory,
		conversation.KeySubcategoryDescription: sub.Description,
		conversation.KeyCategoryIcon:           sub.Icon,
	}
	prompt, err := s.EngineStartWithData(userID, flow.CategoryMatchOfferFlowName, seed)
	if err != nil {
		return startSubcategoryWizard(ctx, s, b, chatID, userID)
	}
	s.SendPrompt(ctx, b, chatID, prompt)
	return nil
}

// SuggestMergeTarget le pregunta al LLM a qué subcategoría existente se parece
// la que el usuario quiere sacar, reusando ClassifyCategoryCreate: ya hace
// exactamente esa pregunta ("¿esto que me describís ya existe?").
//
// Devuelve nil ante cualquier duda — error, timeout, propuesta en vez de match,
// o un match que resuelve al propio origen. nil significa "sin sugerencia", y
// el flujo cae al picker manual. Nunca bloquea.
//
// flow la alcanza via runner (SuggestMergeTarget) porque flow no conoce al
// orchestrator: es la única parte de CATEGORY_MANAGE que toca el LLM.
func SuggestMergeTarget(ctx context.Context, s Services, userID, sourceID uint64, data conversation.Data) *subcategory.Subcategory {
	subs, err := s.SubcategoriesFindAllForUser(userID)
	if err != nil {
		return nil
	}

	taxonomy := make([]orchestrator.TaxonomyEntry, 0, len(subs))
	var sourceDescription string
	for _, sub := range subs {
		if subcategory.IsReserved(sub.Category) {
			continue
		}
		if uint64(sub.ID) == sourceID {
			sourceDescription = sub.Description
			continue // sin esta exclusión el LLM se matchearía a sí mismo
		}
		taxonomy = append(taxonomy, orchestrator.TaxonomyEntry{
			Category: sub.Category, Subcategory: sub.Subcategory, Description: sub.Description,
		})
	}

	descriptions, _ := s.TopDescriptionsBySubcategory(userID, sourceID, flow.TopDescriptionsForSuggestion)
	text := mergeSuggestionText(
		conversation.StringOrEmpty(data[conversation.KeySourceCategory]),
		conversation.StringOrEmpty(data[conversation.KeySourceSubcategory]),
		sourceDescription,
		descriptions,
	)

	res, err := s.ClassifyCategoryCreate(ctx, text, taxonomy)
	if err != nil || res.Match == nil {
		return nil
	}
	found, err := s.FindSubcategory(userID, res.Match.Category, res.Match.Subcategory)
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
