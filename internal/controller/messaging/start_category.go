package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)

// startSubcategoryWizard starts the classic 7-step wizard fresh — the
// fallback whenever the LLM path can't produce a trustworthy match/proposal.
func (c *controller) startSubcategoryWizard(ctx context.Context, b *bot.Bot, chatID int64, userID uint64) error {
	return c.startFlow(ctx, b, chatID, userID, subcategorySetupFlowName, nil, "start subcategory_setup flow")
}

// startSubcategorySetup resolves a CREATE_CATEGORY message with the LLM
// first: an existing-entry match offers reuse (the "regalos ya existía"
// case), a full proposal collapses the 7-step wizard into one confirmation.
// Any doubt → the classic wizard, never a dead end.
func (c *controller) startSubcategorySetup(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) error {
	slog.InfoContext(ctx, "flow started", "flow", subcategorySetupFlowName, "user_id", userID)
	subs, err := c.subcategories.FindAllForUser(userID)
	if err != nil {
		return c.startSubcategoryWizard(ctx, b, chatID, userID)
	}
	taxonomy := make([]orchestrator.TaxonomyEntry, 0, len(subs))
	for _, s := range subs {
		if subcategory.IsReserved(s.Category) {
			continue
		}
		taxonomy = append(taxonomy, orchestrator.TaxonomyEntry{Category: s.Category, Subcategory: s.Subcategory, Description: s.Description})
	}

	res, err := c.orchestrator.ClassifyCategoryCreate(ctx, text, taxonomy)
	if err != nil {
		return c.startSubcategoryWizard(ctx, b, chatID, userID)
	}

	if res.Match != nil {
		existing, err := c.subcategories.FindByCategoryAndSubcategory(userID, res.Match.Category, res.Match.Subcategory)
		if err != nil { // hallucinated match → can't offer it
			return c.startSubcategoryWizard(ctx, b, chatID, userID)
		}
		return c.startCategoryMatchOffer(ctx, b, chatID, userID, existing)
	}

	if res.Proposal == nil { // neither match nor proposal usable → never a dead end
		return c.startSubcategoryWizard(ctx, b, chatID, userID)
	}
	p := res.Proposal
	p.Category, p.Subcategory = strings.TrimSpace(p.Category), strings.TrimSpace(p.Subcategory)
	if p.Category == "" || p.Subcategory == "" || subcategory.IsReserved(p.Category) || subcategory.IsReserved(p.Subcategory) {
		return c.startSubcategoryWizard(ctx, b, chatID, userID)
	}
	if existing, err := c.subcategories.FindByCategoryAndSubcategory(userID, p.Category, p.Subcategory); err == nil {
		return c.startCategoryMatchOffer(ctx, b, chatID, userID, existing) // exact duplicate → offer, don't re-create
	}

	isNew := "true"
	if cats, err := c.subcategories.DistinctCategoriesForUser(userID); err == nil {
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
		keyCategory:               p.Category,
		keyCategoryIsNew:          isNew,
		keyCategoryIcon:           icon,
		keySubcategory:            p.Subcategory,
		keySubcategoryDescription: strings.TrimSpace(p.Description),
	}
	prompt, err := c.engine.StartWithData(userID, categoryProposalConfirmFlowName, seed)
	if err != nil {
		return c.startSubcategoryWizard(ctx, b, chatID, userID)
	}
	c.sendPrompt(ctx, b, chatID, prompt)
	return nil
}

// startCategoryManage arranca el flujo de sacar una categoría propia. Antes de
// nada verifica que el usuario tenga alguna: sin eso el picker mostraría solo
// "Cancelar", que es un callejón sin salida disfrazado de flujo.
func (c *controller) startCategoryManage(ctx context.Context, b *bot.Bot, chatID int64, userID uint64) error {
	slog.InfoContext(ctx, "flow started", "flow", categoryManagePickFlowName, "user_id", userID)

	owned, err := c.subcategories.FindOwnedByUser(userID)
	if err != nil {
		c.sendText(ctx, b, chatID, msgCouldNotLoad)
		return fmt.Errorf("category manage: find owned: %w", err)
	}
	if len(owned) == 0 {
		// Se resuelve la métrica: el bot entendió y respondió bien. Sin esto el
		// evento queda pendiente y el sweeper lo marca "abandoned", que en las
		// métricas de asertividad se lee como una falla del bot.
		c.resolveMetric(ctx, userID, outcomeCategoryManageNoOwn)
		c.sendText(ctx, b, chatID, msgCategoryManageNoOwn)
		return nil
	}

	return c.startFlow(ctx, b, chatID, userID, categoryManagePickFlowName, nil, "start category_manage_pick flow")
}

// startCategoryMatchOffer seeds and starts category_match_offer from an
// existing taxonomy row.
func (c *controller) startCategoryMatchOffer(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, s *subcategory.Subcategory) error {
	seed := conversation.Data{
		keyCategory:               s.Category,
		keySubcategory:            s.Subcategory,
		keySubcategoryDescription: s.Description,
		keyCategoryIcon:           s.Icon,
	}
	prompt, err := c.engine.StartWithData(userID, categoryMatchOfferFlowName, seed)
	if err != nil {
		return c.startSubcategoryWizard(ctx, b, chatID, userID)
	}
	c.sendPrompt(ctx, b, chatID, prompt)
	return nil
}
