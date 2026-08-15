package flow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

// FinishAccountCreate is the Telegram-facing finish of the ACCOUNT_CREATE
// confirm: cancel → cancel copy; duplicate name → friendly copy; success →
// opening movement + success copy.
func FinishAccountCreate(ctx context.Context, r runner, b *bot.Bot, chatID int64, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		r.SendText(ctx, b, chatID, MsgAccountCreateCancelled)
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
			r.SendText(ctx, b, chatID, account.MsgAccountAlreadyExists(name, cur))
			return
		}
		r.SendText(ctx, b, chatID, MsgCouldNotSave("tu cuenta"))
		return
	}

	if err := InsertAccountOpeningMovement(r, newAccount, balance); err != nil {
		// Se loguea porque acá se corta: esta función no devuelve error, así que
		// sin esto un saldo de apertura que falla no deja rastro en ningún lado
		// (ni en slog ni en request_traces) y la cuenta queda creada sin él.
		slog.ErrorContext(ctx, "account opening movement failed",
			"user_id", data.UserID(), "account_id", newAccount.ID, "err", err)
		r.SendText(ctx, b, chatID, MsgCouldNotSave("tu cuenta"))
		return
	}

	r.SendText(ctx, b, chatID, MsgAccountCreateSuccess(name, cur, balance))
}

// InsertAccountOpeningMovement inserts the opening transfer movement for
// a freshly created account, same subcategory
// (Sistema | Saldo inicial) and shape as insertInitialBalanceMovements —
// inserted even when balance is "0", for the same reason: the balance is
// always computed from movements, never stored (see movement.SumAmountForAccount).
//
// Toma la cuenta entera, y no (id, moneda) por separado, para que el movimiento
// no pueda quedar en una moneda distinta a la de su cuenta: los dos datos salen
// de la misma fila, así que el desajuste es irrepresentable en vez de chequeado.
// Mezclar monedas es el error que corrompe un balance en silencio — el saldo es
// SUM(amount) y no mira la moneda de cada fila.
//
// NO pasa por movement.Normalize, a propósito: el guard exige que toda
// transferencia sea un grupo de 2 patas con transaction_id, y una apertura no
// tiene contraparte (está tipada Transfer solo para quedar fuera de los
// agregados de cash-flow). Rechazaría toda apertura, con cualquier monto.
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

// FinishAccountManage applies the confirmed operation. Every branch
// already passed its confirm gate inside the flow — this is pure execution.
func FinishAccountManage(ctx context.Context, r runner, b *bot.Bot, chatID int64, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		r.ResolveMetric(ctx, data.UserID(), OutcomeAccountManageCancelled)
		r.SendText(ctx, b, chatID, MsgFlowCancelled)
		return
	}

	switch conversation.StringOrEmpty(data[conversation.KeyOperation]) {
	case OpCreateNew:
		r.ResolveMetric(ctx, data.UserID(), OutcomeAccountCreateRouted)
		_ = r.StartAccountCreate(ctx, b, chatID, data.UserID(), conversation.StringOrEmpty(data[conversation.KeyMessage]))
	case OpRename:
		FinishAccountRename(ctx, r, b, chatID, data)
	case OpAdjust:
		FinishAccountAdjust(ctx, r, b, chatID, data) // Task 7
	case OpDefault:
		FinishAccountDefault(ctx, r, b, chatID, data) // Task 8
	default:
		r.SendText(ctx, b, chatID, MsgSomethingBroke)
	}
}

func FinishAccountRename(ctx context.Context, r runner, b *bot.Bot, chatID int64, data conversation.Data) {
	id, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyAccountID]), 10, 64)
	if err != nil {
		r.SendText(ctx, b, chatID, MsgSomethingBroke)
		return
	}
	newName := conversation.StringOrEmpty(data[conversation.KeyNewName])
	if err := r.RenameAccount(id, newName); err != nil {
		if errors.Is(err, account.ErrAccountAlreadyExists) {
			r.SendText(ctx, b, chatID, account.MsgAccountAlreadyExists(newName, conversation.StringOrEmpty(data[conversation.KeyAccountCurrency])))
			return
		}
		r.SendText(ctx, b, chatID, MsgCouldNotSave("el cambio"))
		return
	}
	r.ResolveMetric(ctx, data.UserID(), OutcomeAccountRenamed)
	r.SendText(ctx, b, chatID, "Listo, ahora se llama "+newName+".")
}

// FinishAccountAdjust inserts THE adjustment movement: delta between the
// declared new total and SUM(amount). The app owns the sign here exactly
// like the guard does: income → +Abs, expense → -Abs. Never the LLM (the
// LLM never even saw the number — it came from a validated TextStep).
func FinishAccountAdjust(ctx context.Context, r runner, b *bot.Bot, chatID int64, data conversation.Data) {
	accountID, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyAccountID]), 10, 64)
	if err != nil {
		r.SendText(ctx, b, chatID, MsgSomethingBroke)
		return
	}
	newTotal, err := movement.ParseARAmount(conversation.StringOrEmpty(data[conversation.KeyNewTotal]))
	if err != nil {
		r.SendText(ctx, b, chatID, MsgSomethingBroke)
		return
	}
	current, err := r.SumAmountForAccount(accountID)
	if err != nil {
		r.SendText(ctx, b, chatID, MsgCouldNotLoad)
		return
	}

	delta := newTotal.Sub(current)
	if delta.IsZero() {
		r.ResolveMetric(ctx, data.UserID(), OutcomeAccountAdjusted)
		r.SendText(ctx, b, chatID, MsgAccountManageNoChange)
		return
	}

	sub, err := r.FindSubcategory(data.UserID(), subcategory.CategorySystem, "Ajuste de saldo")
	if err != nil {
		r.SendText(ctx, b, chatID, MsgCouldNotLoad)
		return
	}

	// La cuenta se relee de la DB para que el guard compare la moneda del
	// movimiento contra la verdad de la base, y no contra lo que dice la Data del
	// flujo. Si las dos discrepan, el ajuste se rechaza en vez de escribir un
	// movimiento en una moneda distinta a la de su cuenta — que es el error que
	// corrompe un balance en silencio, porque el saldo es SUM(amount) y no mira
	// la moneda de cada fila.
	acc, err := r.GetAccount(accountID)
	if err != nil {
		r.SendText(ctx, b, chatID, MsgCouldNotLoad)
		return
	}

	// El tipo lo decide el signo del delta; el signo del amount lo re-deriva el
	// guard a partir del tipo. Antes esa derivación estaba copiada acá a mano,
	// duplicando la regla que movement.Normalize ya es dueño de aplicar.
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
		nil, // sin fallback a la default: la cuenta del ajuste siempre es explícita
	)
	if err != nil {
		slog.ErrorContext(ctx, "balance adjustment rejected by guard",
			"user_id", data.UserID(), "account_id", accountID, "reason", GuardReason(err))
		r.SendText(ctx, b, chatID, CreateErrorCopy(err))
		return
	}

	if err := r.InsertMovementsBatch(movs); err != nil {
		// Mismo motivo que en el finish de cuenta: función void, el error no
		// sube a withTrace. Sin este log, un ajuste de saldo fallido es
		// invisible en producción.
		slog.ErrorContext(ctx, "balance adjustment insert failed",
			"user_id", data.UserID(), "account_id", accountID, "err", err)
		r.SendText(ctx, b, chatID, MsgCouldNotSave("el ajuste"))
		return
	}
	r.ResolveMetric(ctx, data.UserID(), OutcomeAccountAdjusted)
	r.SendText(ctx, b, chatID, fmt.Sprintf("%s: %s %s.",
		conversation.StringOrEmpty(data[conversation.KeyAccountName]), newTotal.String(), conversation.StringOrEmpty(data[conversation.KeyAccountCurrency])))
}

func FinishAccountDefault(ctx context.Context, r runner, b *bot.Bot, chatID int64, data conversation.Data) {
	accountID, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyAccountID]), 10, 64)
	if err != nil {
		r.SendText(ctx, b, chatID, MsgSomethingBroke)
		return
	}
	cur := currency.Currency(conversation.StringOrEmpty(data[conversation.KeyAccountCurrency]))
	name := conversation.StringOrEmpty(data[conversation.KeyAccountName])

	// capture the previous default BEFORE unsetting it
	prev, prevErr := r.FindDefaultAccountByCurrency(data.UserID(), cur)

	if err := r.UnsetDefaultAccount(data.UserID(), cur); err != nil {
		r.SendText(ctx, b, chatID, MsgCouldNotSave("el cambio"))
		return
	}
	if err := r.SetDefaultAccount(accountID); err != nil {
		r.SendText(ctx, b, chatID, MsgCouldNotSave("el cambio"))
		return
	}
	r.ResolveMetric(ctx, data.UserID(), OutcomeAccountDefaultSet)
	r.SendText(ctx, b, chatID, fmt.Sprintf("⭐ %s es tu cuenta en %s por defecto.", name, cur.String()))

	// same-currency guaranteed: prev is the old default OF THIS currency
	if prevErr != nil || prev == nil || uint64(prev.ID) == accountID {
		return
	}
	balance, err := r.SumAmountForAccount(uint64(prev.ID))
	if err != nil {
		return
	}
	// don't offer to move an empty account — nothing meaningful to consolidate
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
	_ = r.StartFlow(ctx, b, chatID, data.UserID(), AccountMoveOfferFlowName, seed, "account_manage: start move-offer flow")
}

func FinishAccountMoveOffer(ctx context.Context, r runner, b *bot.Bot, chatID int64, data conversation.Data) {
	if conversation.StringOrEmpty(data[conversation.KeyMoveChoice]) != MoveChoiceMove {
		r.SendText(ctx, b, chatID, "Listo, dejé todo como estaba.")
		return
	}
	fromID, err1 := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyMoveFromID]), 10, 64)
	toID, err2 := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyMoveToID]), 10, 64)
	if err1 != nil || err2 != nil {
		r.SendText(ctx, b, chatID, MsgSomethingBroke)
		return
	}
	if err := r.ReassignAccountMovements(fromID, toID); err != nil {
		r.SendText(ctx, b, chatID, MsgCouldNotSave("el cambio"))
		return
	}
	r.SendText(ctx, b, chatID, fmt.Sprintf("Listo: los movimientos de %s ahora están en %s. %s quedó en 0.",
		conversation.StringOrEmpty(data[conversation.KeyMoveFromName]), conversation.StringOrEmpty(data[conversation.KeyMoveToName]), conversation.StringOrEmpty(data[conversation.KeyMoveFromName])))
}
