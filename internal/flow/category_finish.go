package flow

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/subcategory"
)

// FinishSubcategorySetup is the Telegram-facing finish of the subcategory
// wizard: cancel → cancel metric + copy; success → insert + cache reload.
func FinishSubcategorySetup(ctx context.Context, r runner, b *bot.Bot, chatID int64, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		r.ResolveMetric(ctx, data.UserID(), OutcomeCategoryCancelled)
		r.SendText(ctx, b, chatID, MsgCreateCancelled)
		return
	}

	if err := InsertNewSubcategory(r, data); err != nil {
		r.SendText(ctx, b, chatID, MsgCouldNotSave("tu categoría"))
		return
	}

	r.ResolveMetric(ctx, data.UserID(), OutcomeCategoryCreated)
	r.SendText(ctx, b, chatID, MsgSubcategorySetupFinished)
}

// InsertNewSubcategory does the real work (kept separate from the
// Telegram-facing finish so it's testable without *bot.Bot, same
// pattern as InsertAccountOpeningMovement). When category_is_new is "true",
// the new category's icon comes from the step the user just answered
// (category_icon); otherwise it's copied from the existing category's icon
// (found via the same Cache lookup used for the duplicate check) — never
// re-asked. Description comes straight from the subcategory-description
// step's answer — this is the field orchestrator.TaxonomyEntry.Description
// feeds to Call 2 CREATE as a classification hint, so a user-created
// subcategory is only as useful as this description is specific.
func InsertNewSubcategory(r runner, data conversation.Data) error {
	userID := data.UserID()
	category := conversation.StringOrEmpty(data[conversation.KeyCategory])
	sub := conversation.StringOrEmpty(data[conversation.KeySubcategory])
	description := conversation.StringOrEmpty(data[conversation.KeySubcategoryDescription])

	icon := conversation.StringOrEmpty(data[conversation.KeyCategoryIcon])
	if icon == "" {
		icon = r.SubcategoryIconForCategory(userID, category)
	}

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

// FinishCategoryMatchOffer handles the "ya existe algo parecido" gate result:
// reuse the existing entry, fall through to the classic wizard to create a
// distinct one, or cancel.
func FinishCategoryMatchOffer(ctx context.Context, r runner, b *bot.Bot, chatID int64, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		r.ResolveMetric(ctx, data.UserID(), OutcomeCategoryCancelled)
		r.SendText(ctx, b, chatID, MsgCreateCancelled)
		return
	}
	switch conversation.StringOrEmpty(data[conversation.KeyMatchChoice]) {
	case OptionUseExisting:
		r.ResolveMetric(ctx, data.UserID(), OutcomeCategoryMatchUsed)
		r.SendText(ctx, b, chatID, MsgCategoryMatchUse)
	case OptionCreateNew:
		// stays pending in intent_events; the wizard's own terminal resolves it
		_ = r.StartFlow(ctx, b, chatID, data.UserID(), SubcategorySetupFlowName, nil, "start subcategory_setup flow")
	default:
		r.SendText(ctx, b, chatID, MsgSomethingBroke)
	}
}

// FinishCategoryProposalConfirm handles the proposal confirmation: create it
// as-is, drop into the classic wizard seeded with the proposal to edit, or
// cancel.
func FinishCategoryProposalConfirm(ctx context.Context, r runner, b *bot.Bot, chatID int64, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		r.ResolveMetric(ctx, data.UserID(), OutcomeCategoryCancelled)
		r.SendText(ctx, b, chatID, MsgCreateCancelled)
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
		_ = r.StartFlow(ctx, b, chatID, data.UserID(), SubcategorySetupFlowName, seed, "start subcategory_setup flow")
		return
	}
	if err := InsertNewSubcategory(r, data); err != nil {
		r.SendText(ctx, b, chatID, MsgCouldNotSave("tu categoría"))
		return
	}
	r.ResolveMetric(ctx, data.UserID(), OutcomeCategoryCreated)
	r.SendText(ctx, b, chatID, MsgSubcategorySetupFinished)
}

// FinishCategoryManagePickFlow corre cuando el usuario eligió (o no) el origen.
// Si eligió, hace el puente al flujo 2.
func FinishCategoryManagePickFlow(ctx context.Context, r runner, b *bot.Bot, chatID int64, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		r.ResolveMetric(ctx, data.UserID(), OutcomeCategoryManageCancelled)
		r.SendText(ctx, b, chatID, MsgFlowCancelled)
		return
	}
	if err := ProceedToCategoryTarget(ctx, r, b, chatID, data); err != nil {
		slog.ErrorContext(ctx, "category manage: proceed to target", "err", err)
		r.SendText(ctx, b, chatID, MsgSomethingBroke)
	}
}

// ProceedToCategoryTarget es el puente entre los dos flujos: cuenta los
// movimientos del origen y, solo si hay alguno, pide una sugerencia de destino.
// Después arranca el flujo 2 con todo eso sembrado.
func ProceedToCategoryTarget(ctx context.Context, r runner, b *bot.Bot, chatID int64, data conversation.Data) error {
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

	return r.StartFlow(ctx, b, chatID, userID, CategoryManageTargetFlowName, seed, "start category_manage_target flow")
}

// FinishCategoryManageTargetFlow aplica lo que el confirm ya le mostró al
// usuario. Es pura ejecución: el gate de confirmación quedó atrás, dentro del
// flujo.
//
// El orden importa. Primero se mueven los movimientos, después se borra la
// categoría. Al revés, un fallo intermedio dejaría movimientos apuntando a una
// fila borrada. En este orden, un fallo del borrado deja la categoría vacía —
// un estado consistente que el usuario puede reintentar.
func FinishCategoryManageTargetFlow(ctx context.Context, r runner, b *bot.Bot, chatID int64, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) || !conversation.Flag(data, conversation.KeyConfirmed) {
		r.ResolveMetric(ctx, data.UserID(), OutcomeCategoryManageCancelled)
		r.SendText(ctx, b, chatID, MsgFlowCancelled)
		return
	}

	userID := data.UserID()
	sourceID, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeySourceSubcategoryID]), 10, 64)
	if err != nil {
		r.SendText(ctx, b, chatID, MsgSomethingBroke)
		return
	}

	targetRaw := conversation.StringOrEmpty(data[conversation.KeyTargetSubcategoryID])
	if targetRaw != "" {
		targetID, err := strconv.ParseUint(targetRaw, 10, 64)
		if err != nil {
			r.SendText(ctx, b, chatID, MsgSomethingBroke)
			return
		}
		if err := r.ReassignSubcategoryMovements(userID, sourceID, targetID); err != nil {
			slog.ErrorContext(ctx, "category manage: reassign", "err", err)
			r.SendText(ctx, b, chatID, MsgCouldNotSave("el cambio"))
			return
		}
	}

	// Delete devuelve ErrSubcategoryNotFound cuando no borró nada (fila ajena,
	// global o inexistente). Hay que cortar acá: decirle "listo, la saqué" a
	// alguien cuya categoría sigue estando sería mentirle.
	if err := r.DeleteSubcategory(userID, sourceID); err != nil {
		slog.ErrorContext(ctx, "category manage: delete", "err", err)
		r.SendText(ctx, b, chatID, MsgCouldNotDelete("tu categoría"))
		return
	}
	// El Cache es read-through: sin Reload la categoría borrada seguiría
	// apareciendo hasta el próximo reinicio del server.
	if err := r.ReloadSubcategories(); err != nil {
		slog.ErrorContext(ctx, "category manage: cache reload", "err", err)
	}

	r.ResolveMetric(ctx, userID, OutcomeCategoryManageApplied)
	if targetRaw == "" {
		r.SendText(ctx, b, chatID, MsgCategoryManageDeleted(SourceLabel(data)))
		return
	}
	r.SendText(ctx, b, chatID, MsgCategoryManageMerged(
		conversation.StringOrEmpty(data[conversation.KeyMovementCount]), SourceLabel(data), TargetLabel(data)))
}
