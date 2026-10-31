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

func (f *Finisher) FinishSubcategorySetup(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		f.resolveMetric(ctx, data.UserID(), OutcomeCategoryCancelled)
		SendText(ctx, chat, MsgCreateCancelled)
		return
	}

	if err := f.InsertNewSubcategory(data); err != nil {
		SendText(ctx, chat, MsgCouldNotSave("tu categoría"))
		return
	}

	f.resolveMetric(ctx, data.UserID(), OutcomeCategoryCreated)
	SendText(ctx, chat, MsgSubcategorySetupFinished)
}

func (f *Finisher) InsertNewSubcategory(data conversation.Data) error {
	userID := data.UserID()
	category := conversation.StringOrEmpty(data[conversation.KeyCategory])
	sub := conversation.StringOrEmpty(data[conversation.KeySubcategory])
	description := conversation.StringOrEmpty(data[conversation.KeySubcategoryDescription])

	icon := conversation.StringOrEmpty(data[conversation.KeyCategoryIcon])
	if icon == "" {
		icon = f.subcategories.IconForCategory(userID, category)
	}

	return f.insertSubcategory(userID, category, sub, description, icon)
}

func (f *Finisher) insertSubcategory(userID uint64, category, sub, description, icon string) error {
	s := &subcategory.Subcategory{
		UserID:      &userID,
		Category:    category,
		Subcategory: sub,
		Description: description,
		IsGlobal:    false,
		Icon:        icon,
	}
	if err := f.subcategories.Insert(s); err != nil {
		return err
	}
	return f.subcategories.Reload()
}

func (f *Finisher) FinishCategoryMatchOffer(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		f.resolveMetric(ctx, data.UserID(), OutcomeCategoryCancelled)
		SendText(ctx, chat, MsgCreateCancelled)
		return
	}
	switch conversation.StringOrEmpty(data[conversation.KeyMatchChoice]) {
	case OptionUseExisting:
		f.resolveMetric(ctx, data.UserID(), OutcomeCategoryMatchUsed)
		SendText(ctx, chat, MsgCategoryMatchUse)
	case OptionCreateNew:
		_ = StartFlow(ctx, f.engine, chat, data.UserID(), SubcategorySetupFlowName, nil, "start subcategory_setup flow")
	default:
		SendText(ctx, chat, MsgSomethingBroke)
	}
}

func (f *Finisher) FinishCategoryProposalConfirm(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		f.resolveMetric(ctx, data.UserID(), OutcomeCategoryCancelled)
		SendText(ctx, chat, MsgCreateCancelled)
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
		_ = StartFlow(ctx, f.engine, chat, data.UserID(), SubcategorySetupFlowName, seed, "start subcategory_setup flow")
		return
	}
	if err := f.InsertNewSubcategory(data); err != nil {
		SendText(ctx, chat, MsgCouldNotSave("tu categoría"))
		return
	}
	f.resolveMetric(ctx, data.UserID(), OutcomeCategoryCreated)
	SendText(ctx, chat, MsgSubcategorySetupFinished)
}

func (f *Finisher) FinishCategoryManagePickFlow(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		f.resolveMetric(ctx, data.UserID(), OutcomeCategoryManageCancelled)
		SendText(ctx, chat, MsgFlowCancelled)
		return
	}
	if err := f.ProceedToCategoryTarget(ctx, chat, data); err != nil {
		slog.ErrorContext(ctx, "category manage: proceed to target", "err", err)
		SendText(ctx, chat, MsgSomethingBroke)
	}
}

func (f *Finisher) ProceedToCategoryTarget(ctx context.Context, chat messenger.Chat, data conversation.Data) error {
	userID := data.UserID()
	sourceID, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeySourceSubcategoryID]), 10, 64)
	if err != nil {
		return fmt.Errorf("category manage: source id inválido: %w", err)
	}

	count, err := f.movements.CountBySubcategory(userID, sourceID)
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
		if sug := f.suggestMergeTarget(ctx, userID, sourceID, data); sug != nil {
			seed[conversation.KeySuggestedSubcategoryID] = strconv.FormatUint(uint64(sug.ID), 10)
			seed[conversation.KeySuggestedCategory] = sug.Category
			seed[conversation.KeySuggestedSubcategory] = sug.Subcategory
		}
	}

	return StartFlow(ctx, f.engine, chat, userID, CategoryManageTargetFlowName, seed, "start category_manage_target flow")
}

func (f *Finisher) FinishCategoryManageTargetFlow(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) || !conversation.Flag(data, conversation.KeyConfirmed) {
		f.resolveMetric(ctx, data.UserID(), OutcomeCategoryManageCancelled)
		SendText(ctx, chat, MsgFlowCancelled)
		return
	}

	userID := data.UserID()
	sourceID, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeySourceSubcategoryID]), 10, 64)
	if err != nil {
		SendText(ctx, chat, MsgSomethingBroke)
		return
	}

	targetRaw := conversation.StringOrEmpty(data[conversation.KeyTargetSubcategoryID])
	if targetRaw != "" {
		targetID, err := strconv.ParseUint(targetRaw, 10, 64)
		if err != nil {
			SendText(ctx, chat, MsgSomethingBroke)
			return
		}
		if err := f.movements.ReassignSubcategory(userID, sourceID, targetID); err != nil {
			slog.ErrorContext(ctx, "category manage: reassign", "err", err)
			SendText(ctx, chat, MsgCouldNotSave("el cambio"))
			return
		}
	}

	if err := f.subcategories.Delete(userID, sourceID); err != nil {
		slog.ErrorContext(ctx, "category manage: delete", "err", err)
		SendText(ctx, chat, MsgCouldNotDelete("tu categoría"))
		return
	}
	if err := f.subcategories.Reload(); err != nil {
		slog.ErrorContext(ctx, "category manage: cache reload", "err", err)
	}

	f.resolveMetric(ctx, userID, OutcomeCategoryManageApplied)
	if targetRaw == "" {
		SendText(ctx, chat, MsgCategoryManageDeleted(SourceLabel(data)))
		return
	}
	SendText(ctx, chat, MsgCategoryManageMerged(
		conversation.StringOrEmpty(data[conversation.KeyMovementCount]), SourceLabel(data), TargetLabel(data)))
}
