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

func FinishMovementCreate(ctx context.Context, r runner, chat messenger.Chat, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		r.ResolveMetric(ctx, data.UserID(), OutcomeCreateCancelled)
		_ = messenger.SendText(ctx, chat, MsgCreateCancelled)
		return
	}

	inserted, err := ResolveAndInsertMovements(r, data)
	if err != nil {
		var short *InsufficientFunds
		if errors.As(err, &short) {
			gateSeed := conversation.CopyData(data)
			gateSeed[conversation.KeyGatePrompt] = MsgInsufficientFunds(short.Shortfalls)
			if serr := r.StartFlow(ctx, chat, data.UserID(), MovementNegativeConfirmFlowName, gateSeed, "create: start negative-confirm flow"); serr != nil {
				slog.ErrorContext(ctx, "negative-confirm flow failed to start", "user_id", data.UserID(), "error", serr)
			}
			return
		}
		slog.ErrorContext(ctx, "movement insert failed", "user_id", data.UserID(), "reason", GuardReason(err))
		r.ResolveMetric(ctx, data.UserID(), FailureOutcomeFor(data))
		_ = messenger.SendText(ctx, chat, CreateErrorCopy(err))
		return
	}
	r.ResolveMetric(ctx, data.UserID(), WriteOutcomeFor(data), CollectMovementIDs(inserted)...)
	_ = messenger.SendText(ctx, chat, MsgConfirmMovements(inserted))
	if name := conversation.StringOrEmpty(data[conversation.KeyFirstAccountName]); name != "" {
		_ = messenger.SendText(ctx, chat, MsgFirstAccountDefault(name, conversation.DecodeStringSlice(data, conversation.KeyFirstAccountCurrencies)))
		_ = messenger.SendText(ctx, chat, MsgInviteMoreAccounts)
		_ = r.MarkTipSent(data.UserID(), NudgeCorrectTip)
	}
}

func FinishMovementNegativeConfirm(ctx context.Context, r runner, chat messenger.Chat, data conversation.Data) {
	switch conversation.StringOrEmpty(data[gateChoiceKey]) {
	case "register":
		conversation.SetFlag(data, conversation.KeySkipBalanceCheck)
		inserted, err := ResolveAndInsertMovements(r, data)
		if err != nil {
			r.SendText(ctx, chat, CreateErrorCopy(err))
			return
		}
		r.ResolveMetric(ctx, data.UserID(), WriteOutcomeFor(data), CollectMovementIDs(inserted)...)
		r.SendText(ctx, chat, MsgConfirmMovements(inserted))
	case "missing":
		r.SendText(ctx, chat, MsgLogMissingFirst)
	default:
		r.SendText(ctx, chat, MsgNotUnderstood)
	}
}

func FinishMovementDelete(ctx context.Context, r runner, chat messenger.Chat, data conversation.Data) {
	if !conversation.Flag(data, conversation.KeyConfirmed) {
		r.ResolveMetric(ctx, data.UserID(), OutcomeDeleteCancelled)
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

	if err := r.SoftDeleteByIDs(ids); err != nil {
		_ = messenger.SendText(ctx, chat, MsgCouldNotDelete("tu movimiento"))
		return
	}

	r.ResolveMetric(ctx, data.UserID(), OutcomeDeleteConfirmed, ids...)
	_ = messenger.SendText(ctx, chat, MsgDeleteApplied)
}

func FinishMovementUpdateConfirm(ctx context.Context, r runner, chat messenger.Chat, data conversation.Data) {
	if !conversation.Flag(data, conversation.KeyConfirmed) {
		r.ResolveMetric(ctx, data.UserID(), OutcomeUpdateCancelled)
		_ = messenger.SendText(ctx, chat, MsgUpdateCancelled)
		return
	}

	if conversation.Flag(data, conversation.KeyDeleteInstead) {
		oldIDs, err := ParseUintSlice(conversation.DecodeStringSlice(data, conversation.KeyOldMovementIDs))
		if err == nil {
			err = r.SoftDeleteByIDs(oldIDs)
		}
		if err != nil {
			slog.ErrorContext(ctx, "update delete failed", "user_id", data.UserID(), "old_ids", oldIDs, "err", err)
			if errors.Is(err, movement.ErrMovementNotFound) {
				r.ResolveMetric(ctx, data.UserID(), OutcomeUpdateConfirmed, oldIDs...)
				r.SendText(ctx, chat, MsgUpdateDeleted)
				return
			}
			r.ResolveMetric(ctx, data.UserID(), OutcomeWriteFailed)
			r.SendText(ctx, chat, MsgCouldNotSave("el cambio"))
			return
		}
		r.ResolveMetric(ctx, data.UserID(), OutcomeUpdateConfirmed, oldIDs...)
		_ = messenger.SendText(ctx, chat, MsgUpdateDeleted)
		return
	}

	inserted, err := ResolveAndInsertMovements(r, data)
	if err != nil {
		slog.ErrorContext(ctx, "update insert failed", "user_id", data.UserID(), "err", err)
		r.ResolveMetric(ctx, data.UserID(), OutcomeWriteFailed)
		_ = messenger.SendText(ctx, chat, CreateErrorCopy(err))
		return
	}
	r.ResolveMetric(ctx, data.UserID(), OutcomeUpdateConfirmed, CollectMovementIDs(inserted)...)
	_ = messenger.SendText(ctx, chat, MsgUpdateApplied)
}
