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

// NudgeCorrectTip es la key de user_nudges del tip "corrige diciéndolo". La
// marca el finish de CREATE para que el nudge post-mensaje no apile un tercer
// tip sobre este mismo turno. Vive acá porque la dueña es la que la marca; el
// borde la re-exporta (nudge.go) mientras la lista de nudges siga en messaging.
const NudgeCorrectTip = "correct_tip"

// FinishMovementCreate is the Telegram-facing finish of the CREATE confirm:
// cancel → cancel metric + copy; insufficient funds → re-route into the
// negative-confirm gate; guard rejection → reason log + failure metric + copy;
// success → success metric (with movement ids) + receipt + first-account
// default invite.
func FinishMovementCreate(ctx context.Context, r runner, chat messenger.Chat, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		r.ResolveMetric(ctx, data.UserID(), OutcomeCreateCancelled)
		_ = messenger.SendText(ctx, chat, MsgCreateCancelled)
		return
	}

	inserted, err := ResolveAndInsertMovements(r, data)
	if err != nil {
		// Saldo insuficiente no es una falla: es una pregunta. Los otros dos
		// caminos de CREATE (startMovementCreate y agentExecutor.record) ya la
		// hacían; éste no, y se comía el movimiento con un error genérico.
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
		// R1/R2 just fired — mark correct_tip sent (not delivered) so the
		// post-message nudge hook doesn't stack a 3rd tip on this same turn.
		_ = r.MarkTipSent(data.UserID(), NudgeCorrectTip)
	}
}

// FinishMovementNegativeConfirm applies the user's choice on the
// insufficient-funds gate.
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
	default: // rewrite / anything else: drop it, the user re-sends
		r.SendText(ctx, chat, MsgNotUnderstood)
	}
}

// FinishMovementDelete applies (or discards) the delete depending on which
// button the user pressed.
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

// FinishMovementUpdateConfirm applies (or discards) a confirmed UPDATE.
// The confirm ChoiceStep always finishes with KeyConfirmed set to one of
// "true"/"false" — never both, never neither.
func FinishMovementUpdateConfirm(ctx context.Context, r runner, chat messenger.Chat, data conversation.Data) {
	if !conversation.Flag(data, conversation.KeyConfirmed) {
		r.ResolveMetric(ctx, data.UserID(), OutcomeUpdateCancelled)
		_ = messenger.SendText(ctx, chat, MsgUpdateCancelled)
		return
	}

	// A correction that zeroes the movement (regalo/gratis total) deletes it
	// instead of storing an illegal amount-0 row — see correctionIsDeletion.
	if conversation.Flag(data, conversation.KeyDeleteInstead) {
		oldIDs, err := ParseUintSlice(conversation.DecodeStringSlice(data, conversation.KeyOldMovementIDs))
		if err == nil {
			err = r.SoftDeleteByIDs(oldIDs)
		}
		if err != nil {
			// El error acá se convierte en copy y se pierde. Sin esta línea un
			// update_failed no dice nada: medido el 2026-08-10, dos de dos salieron
			// de este gate (el usuario ya había confirmado) y no hubo con qué saber
			// por qué falló la escritura.
			slog.ErrorContext(ctx, "update delete failed", "user_id", data.UserID(), "old_ids", oldIDs, "err", err)
			// El movimiento ya no está: el usuario pidió que desapareciera y no
			// está. Decirle que falló sería mentirle, y lo mandaría a reintentar.
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
