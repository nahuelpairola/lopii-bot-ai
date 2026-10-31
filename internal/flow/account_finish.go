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

func (f *Finisher) FinishAccountCreate(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		SendText(ctx, chat, MsgAccountCreateCancelled)
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
	if err := f.accounts.Insert(newAccount); err != nil {
		if errors.Is(err, account.ErrAccountAlreadyExists) {
			SendText(ctx, chat, account.MsgAccountAlreadyExists(name, cur))
			return
		}
		SendText(ctx, chat, MsgCouldNotSave("tu cuenta"))
		return
	}

	if err := f.InsertAccountOpeningMovement(newAccount, balance); err != nil {
		slog.ErrorContext(ctx, "account opening movement failed",
			"user_id", data.UserID(), "account_id", newAccount.ID, "err", err)
		SendText(ctx, chat, MsgCouldNotSave("tu cuenta"))
		return
	}

	SendText(ctx, chat, MsgAccountCreateSuccess(name, cur, balance))
}

func (f *Finisher) InsertAccountOpeningMovement(acc *account.Account, balanceText string) error {
	sub, err := f.subcategories.FindByCategoryAndSubcategory(acc.UserID, subcategory.CategorySystem, subcategory.SubOpeningBalance)
	if err != nil {
		return err
	}
	amount, err := movement.ParseARAmount(balanceText)
	if err != nil {
		return err
	}

	accountID := uint64(acc.ID)
	return f.movements.InsertBatch([]movement.Movement{{
		UserID:        acc.UserID,
		AccountID:     &accountID,
		SubcategoryID: uint64(sub.ID),
		Date:          movement.TodayCivil(),
		Type:          movement.Transfer,
		Amount:        amount,
		Currency:      acc.Currency,
	}})
}

func (f *Finisher) FinishAccountManage(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		f.resolveMetric(ctx, data.UserID(), OutcomeAccountManageCancelled)
		SendText(ctx, chat, MsgFlowCancelled)
		return
	}

	switch conversation.StringOrEmpty(data[conversation.KeyOperation]) {
	case OpCreateNew:
		f.resolveMetric(ctx, data.UserID(), OutcomeAccountCreateRouted)
		_ = f.startAccountCreate(ctx, chat, data.UserID(), conversation.StringOrEmpty(data[conversation.KeyMessage]))
	case OpRename:
		f.FinishAccountRename(ctx, chat, data)
	case OpAdjust:
		f.FinishAccountAdjust(ctx, chat, data)
	case OpDefault:
		f.FinishAccountDefault(ctx, chat, data)
	default:
		SendText(ctx, chat, MsgSomethingBroke)
	}
}

func (f *Finisher) FinishAccountRename(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	id, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyAccountID]), 10, 64)
	if err != nil {
		SendText(ctx, chat, MsgSomethingBroke)
		return
	}
	newName := conversation.StringOrEmpty(data[conversation.KeyNewName])
	if err := f.accounts.Rename(id, newName); err != nil {
		if errors.Is(err, account.ErrAccountAlreadyExists) {
			SendText(ctx, chat, account.MsgAccountAlreadyExists(newName, conversation.StringOrEmpty(data[conversation.KeyAccountCurrency])))
			return
		}
		SendText(ctx, chat, MsgCouldNotSave("el cambio"))
		return
	}
	f.resolveMetric(ctx, data.UserID(), OutcomeAccountRenamed)
	SendText(ctx, chat, "Listo, ahora se llama "+newName+".")
}

func (f *Finisher) FinishAccountAdjust(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	accountID, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyAccountID]), 10, 64)
	if err != nil {
		SendText(ctx, chat, MsgSomethingBroke)
		return
	}
	newTotal, err := movement.ParseARAmount(conversation.StringOrEmpty(data[conversation.KeyNewTotal]))
	if err != nil {
		SendText(ctx, chat, MsgSomethingBroke)
		return
	}
	current, err := f.movements.SumAmountForAccount(accountID)
	if err != nil {
		SendText(ctx, chat, MsgCouldNotLoad)
		return
	}

	delta := newTotal.Sub(current)
	if delta.IsZero() {
		f.resolveMetric(ctx, data.UserID(), OutcomeAccountAdjusted)
		SendText(ctx, chat, MsgAccountManageNoChange)
		return
	}

	sub, err := f.subcategories.FindByCategoryAndSubcategory(data.UserID(), subcategory.CategorySystem, "Ajuste de saldo")
	if err != nil {
		SendText(ctx, chat, MsgCouldNotLoad)
		return
	}

	acc, err := f.accounts.GetAccount(accountID)
	if err != nil {
		SendText(ctx, chat, MsgCouldNotLoad)
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
		SendText(ctx, chat, CreateErrorCopy(err))
		return
	}

	if err := f.movements.InsertBatch(movs); err != nil {
		slog.ErrorContext(ctx, "balance adjustment insert failed",
			"user_id", data.UserID(), "account_id", accountID, "err", err)
		SendText(ctx, chat, MsgCouldNotSave("el ajuste"))
		return
	}
	f.resolveMetric(ctx, data.UserID(), OutcomeAccountAdjusted)
	SendText(ctx, chat, fmt.Sprintf("%s: %s %s.",
		conversation.StringOrEmpty(data[conversation.KeyAccountName]), newTotal.String(), conversation.StringOrEmpty(data[conversation.KeyAccountCurrency])))
}

func (f *Finisher) FinishAccountDefault(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	accountID, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyAccountID]), 10, 64)
	if err != nil {
		SendText(ctx, chat, MsgSomethingBroke)
		return
	}
	cur := currency.Currency(conversation.StringOrEmpty(data[conversation.KeyAccountCurrency]))
	name := conversation.StringOrEmpty(data[conversation.KeyAccountName])

	prev, prevErr := f.accounts.FindDefaultByCurrency(data.UserID(), cur)

	if err := f.accounts.UnsetDefault(data.UserID(), cur); err != nil {
		SendText(ctx, chat, MsgCouldNotSave("el cambio"))
		return
	}
	if err := f.accounts.SetDefault(accountID); err != nil {
		SendText(ctx, chat, MsgCouldNotSave("el cambio"))
		return
	}
	f.resolveMetric(ctx, data.UserID(), OutcomeAccountDefaultSet)
	SendText(ctx, chat, fmt.Sprintf("⭐ %s es tu cuenta en %s por defecto.", name, cur.String()))

	if prevErr != nil || prev == nil || uint64(prev.ID) == accountID {
		return
	}
	balance, err := f.movements.SumAmountForAccount(uint64(prev.ID))
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
	_ = StartFlow(ctx, f.engine, chat, data.UserID(), AccountMoveOfferFlowName, seed, "account_manage: start move-offer flow")
}

func (f *Finisher) FinishAccountMoveOffer(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	if conversation.StringOrEmpty(data[conversation.KeyMoveChoice]) != MoveChoiceMove {
		SendText(ctx, chat, "Listo, dejé todo como estaba.")
		return
	}
	fromID, err1 := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyMoveFromID]), 10, 64)
	toID, err2 := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyMoveToID]), 10, 64)
	if err1 != nil || err2 != nil {
		SendText(ctx, chat, MsgSomethingBroke)
		return
	}
	if err := f.movements.ReassignAccount(fromID, toID); err != nil {
		SendText(ctx, chat, MsgCouldNotSave("el cambio"))
		return
	}
	SendText(ctx, chat, fmt.Sprintf("Listo: los movimientos de %s ahora están en %s. %s quedó en 0.",
		conversation.StringOrEmpty(data[conversation.KeyMoveFromName]), conversation.StringOrEmpty(data[conversation.KeyMoveToName]), conversation.StringOrEmpty(data[conversation.KeyMoveFromName])))
}
