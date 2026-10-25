package settings

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)

const OutcomeCategoryManageNoOwn = "category_manage_no_own"

func (w *Wizards) startSubcategoryWizard(ctx context.Context, chat messenger.Chat, userID uint64) error {
	return flow.StartFlow(ctx, w.engine, chat, userID, flow.SubcategorySetupFlowName, nil, "start subcategory_setup flow")
}

func (w *Wizards) StartSubcategorySetup(ctx context.Context, chat messenger.Chat, userID uint64, text string) error {
	slog.InfoContext(ctx, "flow started", "flow", flow.SubcategorySetupFlowName, "user_id", userID)
	subs, err := w.subcategories.FindAllForUser(userID)
	if err != nil {
		return w.startSubcategoryWizard(ctx, chat, userID)
	}
	taxonomy := make([]orchestrator.TaxonomyEntry, 0, len(subs))
	for _, sub := range subs {
		if subcategory.IsReserved(sub.Category) {
			continue
		}
		taxonomy = append(taxonomy, orchestrator.TaxonomyEntry{Category: sub.Category, Subcategory: sub.Subcategory, Description: sub.Description})
	}

	res, err := w.model.ClassifyCategoryCreate(ctx, text, taxonomy)
	if err != nil {
		if handled, oerr := w.handleGroqError(ctx, chat, userID, text, err); handled {
			return oerr
		}
		return w.startSubcategoryWizard(ctx, chat, userID)
	}

	if res.Match != nil {
		existing, err := w.subcategories.FindByCategoryAndSubcategory(userID, res.Match.Category, res.Match.Subcategory)
		if err != nil {
			return w.startSubcategoryWizard(ctx, chat, userID)
		}
		return w.startCategoryMatchOffer(ctx, chat, userID, existing)
	}

	if res.Proposal == nil {
		return w.startSubcategoryWizard(ctx, chat, userID)
	}
	p := res.Proposal
	p.Category, p.Subcategory = strings.TrimSpace(p.Category), strings.TrimSpace(p.Subcategory)
	if p.Category == "" || p.Subcategory == "" || subcategory.IsReserved(p.Category) || subcategory.IsReserved(p.Subcategory) {
		return w.startSubcategoryWizard(ctx, chat, userID)
	}
	if existing, err := w.subcategories.FindByCategoryAndSubcategory(userID, p.Category, p.Subcategory); err == nil {
		return w.startCategoryMatchOffer(ctx, chat, userID, existing)
	}

	isNew := "true"
	if cats, err := w.subcategories.DistinctCategoriesForUser(userID); err == nil {
		for _, cat := range cats {
			if cat == p.Category {
				isNew = "false"
				break
			}
		}
	}
	icon := strings.TrimSpace(p.Icon)
	if !subcategory.ValidIcon(icon) {
		icon = ""
	}
	seed := conversation.Data{
		conversation.KeyCategory:               p.Category,
		conversation.KeyCategoryIsNew:          isNew,
		conversation.KeyCategoryIcon:           icon,
		conversation.KeySubcategory:            p.Subcategory,
		conversation.KeySubcategoryDescription: strings.TrimSpace(p.Description),
	}
	prompt, err := w.engine.StartWithData(userID, flow.CategoryProposalConfirmFlowName, seed)
	if err != nil {
		return w.startSubcategoryWizard(ctx, chat, userID)
	}
	flow.SendPrompt(ctx, chat, prompt)
	return nil
}

func (w *Wizards) StartCategoryManage(ctx context.Context, chat messenger.Chat, userID uint64) error {
	slog.InfoContext(ctx, "flow started", "flow", flow.CategoryManagePickFlowName, "user_id", userID)

	owned, err := w.subcategories.FindOwnedByUser(userID)
	if err != nil {
		flow.SendText(ctx, chat, flow.MsgCouldNotLoad)
		return fmt.Errorf("category manage: find owned: %w", err)
	}
	if len(owned) == 0 {
		w.resolveMetric(ctx, userID, OutcomeCategoryManageNoOwn)
		flow.SendText(ctx, chat, flow.MsgCategoryManageNoOwn)
		return nil
	}

	return flow.StartFlow(ctx, w.engine, chat, userID, flow.CategoryManagePickFlowName, nil, "start category_manage_pick flow")
}

func (w *Wizards) startCategoryMatchOffer(ctx context.Context, chat messenger.Chat, userID uint64, sub *subcategory.Subcategory) error {
	seed := conversation.Data{
		conversation.KeyCategory:               sub.Category,
		conversation.KeySubcategory:            sub.Subcategory,
		conversation.KeySubcategoryDescription: sub.Description,
		conversation.KeyCategoryIcon:           sub.Icon,
	}
	prompt, err := w.engine.StartWithData(userID, flow.CategoryMatchOfferFlowName, seed)
	if err != nil {
		return w.startSubcategoryWizard(ctx, chat, userID)
	}
	flow.SendPrompt(ctx, chat, prompt)
	return nil
}

func (w *Wizards) SuggestMergeTarget(ctx context.Context, userID, sourceID uint64, data conversation.Data) *subcategory.Subcategory {
	subs, err := w.subcategories.FindAllForUser(userID)
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
			continue
		}
		taxonomy = append(taxonomy, orchestrator.TaxonomyEntry{
			Category: sub.Category, Subcategory: sub.Subcategory, Description: sub.Description,
		})
	}

	descriptions, _ := w.movements.TopDescriptionsBySubcategory(userID, sourceID, flow.TopDescriptionsForSuggestion)
	text := mergeSuggestionText(
		conversation.StringOrEmpty(data[conversation.KeySourceCategory]),
		conversation.StringOrEmpty(data[conversation.KeySourceSubcategory]),
		sourceDescription,
		descriptions,
	)

	res, err := w.model.ClassifyCategoryCreate(ctx, text, taxonomy)
	if err != nil || res.Match == nil {
		return nil
	}
	found, err := w.subcategories.FindByCategoryAndSubcategory(userID, res.Match.Category, res.Match.Subcategory)
	if err != nil || found == nil || uint64(found.ID) == sourceID {
		return nil
	}
	return found
}

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
