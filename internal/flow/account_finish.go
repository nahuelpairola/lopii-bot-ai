package flow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

func FinishAccountCreate(ctx context.Context, r runner, chat messenger.Chat, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		r.SendText(ctx, chat, MsgAccountCreateCancelled)
		return
	}

	name := conversation.StringOrEmpty(data[conversation.KeyAccountName])
	cur := conversation.StringOrEmpty(data[conversation.KeyAccountCurrency])
	balance := conversation.StringOrEmpty(data[conversation.KeyAccountBalance])

	newAccount := &account.Account{
		UserID:   data.UserID(),
		Name:     name,
		Currency: currency.Currency(cur),
	}
	if err := r.InsertAccount(newAccount); err != nil {
		if errors.Is(err, account.ErrAccountAlreadyExists) {
			r.SendText(ctx, chat, account.MsgAccountAlreadyExists(name, cur))
			return
		}
		r.SendText(ctx, chat, MsgCouldNotSave("tu cuenta"))
		return
	}

	if err := InsertAccountOpeningMovement(r, newAccount, balance); err != nil {
		slog.ErrorContext(ctx, "account opening movement failed",
			"user_id", data.UserID(), "account_id", newAccount.ID, "err", err)
		r.SendText(ctx, chat, MsgCouldNotSave("tu cuenta"))
		return
	}

	r.SendText(ctx, chat, MsgAccountCreateSuccess(name, cur, balance))
}

func InsertAccountOpeningMovement(r runner, acc *account.Account, balanceText string) error {
	sub, err := r.FindSubcategory(acc.UserID, subcategory.CategorySystem, subcategory.SubOpeningBalance)
	if err != nil {
		return err
	}
	amount, err := movement.ParseARAmount(balanceText)
	if err != nil {
		return err
	}

	accountID := uint64(acc.ID)
	return r.InsertMovementsBatch([]movement.Movement{{
		UserID:        acc.UserID,
		AccountID:     &accountID,
		SubcategoryID: uint64(sub.ID),
		Date:          movement.TodayCivil(),
		Type:          movement.Transfer,
		Amount:        amount,
		Currency:      acc.Currency,
	}})
}

func FinishAccountManage(ctx context.Context, r runner, chat messenger.Chat, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		r.ResolveMetric(ctx, data.UserID(), OutcomeAccountManageCancelled)
		r.SendText(ctx, chat, MsgFlowCancelled)
		return
	}

	switch conversation.StringOrEmpty(data[conversation.KeyOperation]) {
	case OpCreateNew:
		r.ResolveMetric(ctx, data.UserID(), OutcomeAccountCreateRouted)
		_ = r.StartAccountCreate(ctx, chat, data.UserID(), conversation.StringOrEmpty(data[conversation.KeyMessage]))
	case OpRename:
		FinishAccountRename(ctx, r, chat, data)
	case OpAdjust:
		FinishAccountAdjust(ctx, r, chat, data)
	case OpDefault:
		FinishAccountDefault(ctx, r, chat, data)
	default:
		r.SendText(ctx, chat, MsgSomethingBroke)
	}
}

func FinishAccountRename(ctx context.Context, r runner, chat messenger.Chat, data conversation.Data) {
	id, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyAccountID]), 10, 64)
	if err != nil {
		r.SendText(ctx, chat, MsgSomethingBroke)
		return
	}
	newName := conversation.StringOrEmpty(data[conversation.KeyNewName])
	if err := r.RenameAccount(id, newName); err != nil {
		if errors.Is(err, account.ErrAccountAlreadyExists) {
			r.SendText(ctx, chat, account.MsgAccountAlreadyExists(newName, conversation.StringOrEmpty(data[conversation.KeyAccountCurrency])))
			return
		}
		r.SendText(ctx, chat, MsgCouldNotSave("el cambio"))
		return
	}
	r.ResolveMetric(ctx, data.UserID(), OutcomeAccountRenamed)
	r.SendText(ctx, chat, "Listo, ahora se llama "+newName+".")
}

func FinishAccountAdjust(ctx context.Context, r runner, chat messenger.Chat, data conversation.Data) {
	accountID, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyAccountID]), 10, 64)
	if err != nil {
		r.SendText(ctx, chat, MsgSomethingBroke)
		return
	}
	newTotal, err := movement.ParseARAmount(conversation.StringOrEmpty(data[conversation.KeyNewTotal]))
	if err != nil {
		r.SendText(ctx, chat, MsgSomethingBroke)
		return
	}
	current, err := r.SumAmountForAccount(accountID)
	if err != nil {
		r.SendText(ctx, chat, MsgCouldNotLoad)
		return
	}

	delta := newTotal.Sub(current)
	if delta.IsZero() {
		r.ResolveMetric(ctx, data.UserID(), OutcomeAccountAdjusted)
		r.SendText(ctx, chat, MsgAccountManageNoChange)
		return
	}

	sub, err := r.FindSubcategory(data.UserID(), subcategory.CategorySystem, "Ajuste de saldo")
	if err != nil {
		r.SendText(ctx, chat, MsgCouldNotLoad)
		return
	}

	acc, err := r.GetAccount(accountID)
	if err != nil {
		r.SendText(ctx, chat, MsgCouldNotLoad)
		return
	}

	mType := movement.Income
	if delta.IsNegative() {
		mType = movement.Expense
	}
	movs, err := movement.Normalize(
		[]movement.Movement{{
			UserID:        data.UserID(),
			AccountID:     &accountID,
			SubcategoryID: uint64(sub.ID),
			Date:          movement.TodayCivil(),
			Type:          mType,
			Amount:        delta.Abs(),
			Currency:      currency.Currency(conversation.StringOrEmpty(data[conversation.KeyAccountCurrency])),
		}},
		map[uint64]account.Account{accountID: *acc},
		nil,
	)
	if err != nil {
		slog.ErrorContext(ctx, "balance adjustment rejected by guard",
			"user_id", data.UserID(), "account_id", accountID, "reason", GuardReason(err))
		r.SendText(ctx, chat, CreateErrorCopy(err))
		return
	}

	if err := r.InsertMovementsBatch(movs); err != nil {
		slog.ErrorContext(ctx, "balance adjustment insert failed",
			"user_id", data.UserID(), "account_id", accountID, "err", err)
		r.SendText(ctx, chat, MsgCouldNotSave("el ajuste"))
		return
	}
	r.ResolveMetric(ctx, data.UserID(), OutcomeAccountAdjusted)
	r.SendText(ctx, chat, fmt.Sprintf("%s: %s %s.",
		conversation.StringOrEmpty(data[conversation.KeyAccountName]), newTotal.String(), conversation.StringOrEmpty(data[conversation.KeyAccountCurrency])))
}

func FinishAccountDefault(ctx context.Context, r runner, chat messenger.Chat, data conversation.Data) {
	accountID, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyAccountID]), 10, 64)
	if err != nil {
		r.SendText(ctx, chat, MsgSomethingBroke)
		return
	}
	cur := currency.Currency(conversation.StringOrEmpty(data[conversation.KeyAccountCurrency]))
	name := conversation.StringOrEmpty(data[conversation.KeyAccountName])

	prev, prevErr := r.FindDefaultAccountByCurrency(data.UserID(), cur)

	if err := r.UnsetDefaultAccount(data.UserID(), cur); err != nil {
		r.SendText(ctx, chat, MsgCouldNotSave("el cambio"))
		return
	}
	if err := r.SetDefaultAccount(accountID); err != nil {
		r.SendText(ctx, chat, MsgCouldNotSave("el cambio"))
		return
	}
	r.ResolveMetric(ctx, data.UserID(), OutcomeAccountDefaultSet)
	r.SendText(ctx, chat, fmt.Sprintf("⭐ %s es tu cuenta en %s por defecto.", name, cur.String()))

	if prevErr != nil || prev == nil || uint64(prev.ID) == accountID {
		return
	}
	balance, err := r.SumAmountForAccount(uint64(prev.ID))
	if err != nil {
		return
	}
	if balance.IsZero() {
		return
	}
	seed := conversation.Data{
		conversation.KeyMoveFromID:      strconv.FormatUint(uint64(prev.ID), 10),
		conversation.KeyMoveFromName:    prev.Name,
		conversation.KeyMoveFromBalance: balance.String(),
		conversation.KeyMoveToID:        strconv.FormatUint(accountID, 10),
		conversation.KeyMoveToName:      name,
	}
	_ = r.StartFlow(ctx, chat, data.UserID(), AccountMoveOfferFlowName, seed, "account_manage: start move-offer flow")
}

func FinishAccountMoveOffer(ctx context.Context, r runner, chat messenger.Chat, data conversation.Data) {
	if conversation.StringOrEmpty(data[conversation.KeyMoveChoice]) != MoveChoiceMove {
		r.SendText(ctx, chat, "Listo, dejé todo como estaba.")
		return
	}
	fromID, err1 := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyMoveFromID]), 10, 64)
	toID, err2 := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyMoveToID]), 10, 64)
	if err1 != nil || err2 != nil {
		r.SendText(ctx, chat, MsgSomethingBroke)
		return
	}
	if err := r.ReassignAccountMovements(fromID, toID); err != nil {
		r.SendText(ctx, chat, MsgCouldNotSave("el cambio"))
		return
	}
	r.SendText(ctx, chat, fmt.Sprintf("Listo: los movimientos de %s ahora están en %s. %s quedó en 0.",
		conversation.StringOrEmpty(data[conversation.KeyMoveFromName]), conversation.StringOrEmpty(data[conversation.KeyMoveToName]), conversation.StringOrEmpty(data[conversation.KeyMoveFromName])))
}
