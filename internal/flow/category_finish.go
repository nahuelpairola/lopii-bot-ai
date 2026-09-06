package flow

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/subcategory"
)

func FinishSubcategorySetup(ctx context.Context, r runner, chat messenger.Chat, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		r.ResolveMetric(ctx, data.UserID(), OutcomeCategoryCancelled)
		r.SendText(ctx, chat, MsgCreateCancelled)
		return
	}

	if err := InsertNewSubcategory(r, data); err != nil {
		r.SendText(ctx, chat, MsgCouldNotSave("tu categoría"))
		return
	}

	r.ResolveMetric(ctx, data.UserID(), OutcomeCategoryCreated)
	r.SendText(ctx, chat, MsgSubcategorySetupFinished)
}

func InsertNewSubcategory(r runner, data conversation.Data) error {
	userID := data.UserID()
	category := conversation.StringOrEmpty(data[conversation.KeyCategory])
	sub := conversation.StringOrEmpty(data[conversation.KeySubcategory])
	description := conversation.StringOrEmpty(data[conversation.KeySubcategoryDescription])

	icon := conversation.StringOrEmpty(data[conversation.KeyCategoryIcon])
	if icon == "" {
		icon = r.SubcategoryIconForCategory(userID, category)
	}

	return insertSubcategory(r, userID, category, sub, description, icon)
}

func insertSubcategory(r runner, userID uint64, category, sub, description, icon string) error {
	s := &subcategory.Subcategory{
		UserID:      &userID,
		Category:    category,
		Subcategory: sub,
		Description: description,
		IsGlobal:    false,
		Icon:        icon,
	}
	if err := r.InsertSubcategory(s); err != nil {
		return err
	}
	return r.ReloadSubcategories()
}

func FinishCategoryMatchOffer(ctx context.Context, r runner, chat messenger.Chat, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		r.ResolveMetric(ctx, data.UserID(), OutcomeCategoryCancelled)
		r.SendText(ctx, chat, MsgCreateCancelled)
		return
	}
	switch conversation.StringOrEmpty(data[conversation.KeyMatchChoice]) {
	case OptionUseExisting:
		r.ResolveMetric(ctx, data.UserID(), OutcomeCategoryMatchUsed)
		r.SendText(ctx, chat, MsgCategoryMatchUse)
	case OptionCreateNew:
		_ = r.StartFlow(ctx, chat, data.UserID(), SubcategorySetupFlowName, nil, "start subcategory_setup flow")
	default:
		r.SendText(ctx, chat, MsgSomethingBroke)
	}
}

func FinishCategoryProposalConfirm(ctx context.Context, r runner, chat messenger.Chat, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		r.ResolveMetric(ctx, data.UserID(), OutcomeCategoryCancelled)
		r.SendText(ctx, chat, MsgCreateCancelled)
		return
	}
	if conversation.Flag(data, conversation.KeyEditProposal) {
		seed := conversation.Data{
			conversation.KeyCategory:               conversation.StringOrEmpty(data[conversation.KeyCategory]),
			conversation.KeyCategoryIsNew:          conversation.StringOrEmpty(data[conversation.KeyCategoryIsNew]),
			conversation.KeyCategoryIcon:           conversation.StringOrEmpty(data[conversation.KeyCategoryIcon]),
			conversation.KeySubcategory:            conversation.StringOrEmpty(data[conversation.KeySubcategory]),
			conversation.KeySubcategoryDescription: conversation.StringOrEmpty(data[conversation.KeySubcategoryDescription]),
		}
		_ = r.StartFlow(ctx, chat, data.UserID(), SubcategorySetupFlowName, seed, "start subcategory_setup flow")
		return
	}
	if err := InsertNewSubcategory(r, data); err != nil {
		r.SendText(ctx, chat, MsgCouldNotSave("tu categoría"))
		return
	}
	r.ResolveMetric(ctx, data.UserID(), OutcomeCategoryCreated)
	r.SendText(ctx, chat, MsgSubcategorySetupFinished)
}

func FinishCategoryManagePickFlow(ctx context.Context, r runner, chat messenger.Chat, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		r.ResolveMetric(ctx, data.UserID(), OutcomeCategoryManageCancelled)
		r.SendText(ctx, chat, MsgFlowCancelled)
		return
	}
	if err := ProceedToCategoryTarget(ctx, r, chat, data); err != nil {
		slog.ErrorContext(ctx, "category manage: proceed to target", "err", err)
		r.SendText(ctx, chat, MsgSomethingBroke)
	}
}

func ProceedToCategoryTarget(ctx context.Context, r runner, chat messenger.Chat, data conversation.Data) error {
	userID := data.UserID()
	sourceID, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeySourceSubcategoryID]), 10, 64)
	if err != nil {
		return fmt.Errorf("category manage: source id inválido: %w", err)
	}

	count, err := r.CountMovementsBySubcategory(userID, sourceID)
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
		if sug := r.SuggestMergeTarget(ctx, userID, sourceID, data); sug != nil {
			seed[conversation.KeySuggestedSubcategoryID] = strconv.FormatUint(uint64(sug.ID), 10)
			seed[conversation.KeySuggestedCategory] = sug.Category
			seed[conversation.KeySuggestedSubcategory] = sug.Subcategory
		}
	}

	return r.StartFlow(ctx, chat, userID, CategoryManageTargetFlowName, seed, "start category_manage_target flow")
}

func FinishCategoryManageTargetFlow(ctx context.Context, r runner, chat messenger.Chat, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) || !conversation.Flag(data, conversation.KeyConfirmed) {
		r.ResolveMetric(ctx, data.UserID(), OutcomeCategoryManageCancelled)
		r.SendText(ctx, chat, MsgFlowCancelled)
		return
	}

	userID := data.UserID()
	sourceID, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeySourceSubcategoryID]), 10, 64)
	if err != nil {
		r.SendText(ctx, chat, MsgSomethingBroke)
		return
	}

	targetRaw := conversation.StringOrEmpty(data[conversation.KeyTargetSubcategoryID])
	if targetRaw != "" {
		targetID, err := strconv.ParseUint(targetRaw, 10, 64)
		if err != nil {
			r.SendText(ctx, chat, MsgSomethingBroke)
			return
		}
		if err := r.ReassignSubcategoryMovements(userID, sourceID, targetID); err != nil {
			slog.ErrorContext(ctx, "category manage: reassign", "err", err)
			r.SendText(ctx, chat, MsgCouldNotSave("el cambio"))
			return
		}
	}

	if err := r.DeleteSubcategory(userID, sourceID); err != nil {
		slog.ErrorContext(ctx, "category manage: delete", "err", err)
		r.SendText(ctx, chat, MsgCouldNotDelete("tu categoría"))
		return
	}
	if err := r.ReloadSubcategories(); err != nil {
		slog.ErrorContext(ctx, "category manage: cache reload", "err", err)
	}

	r.ResolveMetric(ctx, userID, OutcomeCategoryManageApplied)
	if targetRaw == "" {
		r.SendText(ctx, chat, MsgCategoryManageDeleted(SourceLabel(data)))
		return
	}
	r.SendText(ctx, chat, MsgCategoryManageMerged(
		conversation.StringOrEmpty(data[conversation.KeyMovementCount]), SourceLabel(data), TargetLabel(data)))
}
