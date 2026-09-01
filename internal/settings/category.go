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

func startSubcategoryWizard(ctx context.Context, s Services, chat messenger.Chat, userID uint64) error {
	return s.StartFlow(ctx, chat, userID, flow.SubcategorySetupFlowName, nil, "start subcategory_setup flow")
}

func StartSubcategorySetup(ctx context.Context, s Services, chat messenger.Chat, userID uint64, text string) error {
	slog.InfoContext(ctx, "flow started", "flow", flow.SubcategorySetupFlowName, "user_id", userID)
	subs, err := s.SubcategoriesFindAllForUser(userID)
	if err != nil {
		return startSubcategoryWizard(ctx, s, chat, userID)
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
		if handled, oerr := s.HandleGroqError(ctx, chat, userID, text, err); handled {
			return oerr
		}
		return startSubcategoryWizard(ctx, s, chat, userID)
	}

	if res.Match != nil {
		existing, err := s.FindSubcategory(userID, res.Match.Category, res.Match.Subcategory)
		if err != nil {
			return startSubcategoryWizard(ctx, s, chat, userID)
		}
		return startCategoryMatchOffer(ctx, s, chat, userID, existing)
	}

	if res.Proposal == nil {
		return startSubcategoryWizard(ctx, s, chat, userID)
	}
	p := res.Proposal
	p.Category, p.Subcategory = strings.TrimSpace(p.Category), strings.TrimSpace(p.Subcategory)
	if p.Category == "" || p.Subcategory == "" || subcategory.IsReserved(p.Category) || subcategory.IsReserved(p.Subcategory) {
		return startSubcategoryWizard(ctx, s, chat, userID)
	}
	if existing, err := s.FindSubcategory(userID, p.Category, p.Subcategory); err == nil {
		return startCategoryMatchOffer(ctx, s, chat, userID, existing)
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
		icon = ""
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
		return startSubcategoryWizard(ctx, s, chat, userID)
	}
	s.SendPrompt(ctx, chat, prompt)
	return nil
}

func StartCategoryManage(ctx context.Context, s Services, chat messenger.Chat, userID uint64) error {
	slog.InfoContext(ctx, "flow started", "flow", flow.CategoryManagePickFlowName, "user_id", userID)

	owned, err := s.FindOwnedSubcategories(userID)
	if err != nil {
		s.SendText(ctx, chat, flow.MsgCouldNotLoad)
		return fmt.Errorf("category manage: find owned: %w", err)
	}
	if len(owned) == 0 {
		s.ResolveMetric(ctx, userID, OutcomeCategoryManageNoOwn)
		s.SendText(ctx, chat, flow.MsgCategoryManageNoOwn)
		return nil
	}

	return s.StartFlow(ctx, chat, userID, flow.CategoryManagePickFlowName, nil, "start category_manage_pick flow")
}

func startCategoryMatchOffer(ctx context.Context, s Services, chat messenger.Chat, userID uint64, sub *subcategory.Subcategory) error {
	seed := conversation.Data{
		conversation.KeyCategory:               sub.Category,
		conversation.KeySubcategory:            sub.Subcategory,
		conversation.KeySubcategoryDescription: sub.Description,
		conversation.KeyCategoryIcon:           sub.Icon,
	}
	prompt, err := s.EngineStartWithData(userID, flow.CategoryMatchOfferFlowName, seed)
	if err != nil {
		return startSubcategoryWizard(ctx, s, chat, userID)
	}
	s.SendPrompt(ctx, chat, prompt)
	return nil
}

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
			continue
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
