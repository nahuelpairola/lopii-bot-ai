package flow

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
)

const NudgeCorrectTip = "correct_tip"

func (f *Finisher) FinishMovementCreate(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		f.resolveMetric(ctx, data.UserID(), OutcomeCreateCancelled)
		_ = messenger.SendText(ctx, chat, MsgCreateCancelled)
		return
	}

	inserted, err := f.ResolveAndInsertMovements(data)
	if err != nil {
		var short *InsufficientFunds
		if errors.As(err, &short) {
			gateSeed := conversation.CopyData(data)
			gateSeed[conversation.KeyGatePrompt] = MsgInsufficientFunds(short.Shortfalls)
			if serr := StartFlow(ctx, f.engine, chat, data.UserID(), MovementNegativeConfirmFlowName, gateSeed, "create: start negative-confirm flow"); serr != nil {
				slog.ErrorContext(ctx, "negative-confirm flow failed to start", "user_id", data.UserID(), "error", serr)
			}
			return
		}
		slog.ErrorContext(ctx, "movement insert failed", "user_id", data.UserID(), "reason", GuardReason(err))
		f.resolveMetric(ctx, data.UserID(), FailureOutcomeFor(data))
		_ = messenger.SendText(ctx, chat, CreateErrorCopy(err))
		return
	}
	f.resolveMetric(ctx, data.UserID(), WriteOutcomeFor(data), CollectMovementIDs(inserted)...)
	_ = messenger.SendText(ctx, chat, MsgConfirmMovements(inserted))
	if name := conversation.StringOrEmpty(data[conversation.KeyFirstAccountName]); name != "" {
		_ = messenger.SendText(ctx, chat, MsgFirstAccountDefault(name, conversation.DecodeStringSlice(data, conversation.KeyFirstAccountCurrencies)))
		_ = messenger.SendText(ctx, chat, MsgInviteMoreAccounts)
		_ = f.markTipSent(data.UserID(), NudgeCorrectTip)
	}
}

func (f *Finisher) FinishMovementNegativeConfirm(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	switch conversation.StringOrEmpty(data[gateChoiceKey]) {
	case "register":
		conversation.SetFlag(data, conversation.KeySkipBalanceCheck)
		inserted, err := f.ResolveAndInsertMovements(data)
		if err != nil {
			SendText(ctx, chat, CreateErrorCopy(err))
			return
		}
		f.resolveMetric(ctx, data.UserID(), WriteOutcomeFor(data), CollectMovementIDs(inserted)...)
		SendText(ctx, chat, MsgConfirmMovements(inserted))
	case "missing":
		SendText(ctx, chat, MsgLogMissingFirst)
	default:
		SendText(ctx, chat, MsgNotUnderstood)
	}
}

func (f *Finisher) FinishMovementDelete(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	if !conversation.Flag(data, conversation.KeyConfirmed) {
		f.resolveMetric(ctx, data.UserID(), OutcomeDeleteCancelled)
		_ = messenger.SendText(ctx, chat, MsgDeleteCancelled)
		return
	}

	idx, err := strconv.Atoi(conversation.StringOrEmpty(data[conversation.KeyResolvedIndex]))
	candidates := DecodeCandidateGroups(data)
	if err != nil || idx < 0 || idx >= len(candidates) {
		_ = messenger.SendText(ctx, chat, MsgSomethingBroke)
		return
	}

	ids, err := ParseUintSlice(candidates[idx].OldIDs)
	if err != nil {
		_ = messenger.SendText(ctx, chat, MsgSomethingBroke)
		return
	}

	if err := f.movements.SoftDeleteByIDs(ids); err != nil {
		_ = messenger.SendText(ctx, chat, MsgCouldNotDelete("tu movimiento"))
		return
	}

	f.resolveMetric(ctx, data.UserID(), OutcomeDeleteConfirmed, ids...)
	_ = messenger.SendText(ctx, chat, MsgDeleteApplied)
}

func (f *Finisher) FinishMovementUpdateConfirm(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	if !conversation.Flag(data, conversation.KeyConfirmed) {
		f.resolveMetric(ctx, data.UserID(), OutcomeUpdateCancelled)
		_ = messenger.SendText(ctx, chat, MsgUpdateCancelled)
		return
	}

	if conversation.Flag(data, conversation.KeyDeleteInstead) {
		oldIDs, err := ParseUintSlice(conversation.DecodeStringSlice(data, conversation.KeyOldMovementIDs))
		if err == nil {
			err = f.movements.SoftDeleteByIDs(oldIDs)
		}
		if err != nil {
			slog.ErrorContext(ctx, "update delete failed", "user_id", data.UserID(), "old_ids", oldIDs, "err", err)
			if errors.Is(err, movement.ErrMovementNotFound) {
				f.resolveMetric(ctx, data.UserID(), OutcomeUpdateConfirmed, oldIDs...)
				SendText(ctx, chat, MsgUpdateDeleted)
				return
			}
			f.resolveMetric(ctx, data.UserID(), OutcomeWriteFailed)
			SendText(ctx, chat, MsgCouldNotSave("el cambio"))
			return
		}
		f.resolveMetric(ctx, data.UserID(), OutcomeUpdateConfirmed, oldIDs...)
		_ = messenger.SendText(ctx, chat, MsgUpdateDeleted)
		return
	}

	inserted, err := f.ResolveAndInsertMovements(data)
	if err != nil {
		slog.ErrorContext(ctx, "update insert failed", "user_id", data.UserID(), "err", err)
		f.resolveMetric(ctx, data.UserID(), OutcomeWriteFailed)
		_ = messenger.SendText(ctx, chat, CreateErrorCopy(err))
		return
	}
	f.resolveMetric(ctx, data.UserID(), OutcomeUpdateConfirmed, CollectMovementIDs(inserted)...)
	_ = messenger.SendText(ctx, chat, MsgUpdateApplied)
}
