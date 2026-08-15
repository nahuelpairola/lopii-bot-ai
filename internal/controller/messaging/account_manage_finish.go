package messaging

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
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

// finishAccountManageFlow applies the confirmed operation. Every branch
// already passed its confirm gate inside the flow — this is pure execution.
func (c *controller) finishAccountManageFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		c.resolveMetric(ctx, data.UserID(), outcomeAccountManageCancelled)
		c.sendText(ctx, b, chatID, msgFlowCancelled)
		return
	}

	switch conversation.StringOrEmpty(data[conversation.KeyOperation]) {
	case flow.OpCreateNew:
		c.resolveMetric(ctx, data.UserID(), outcomeAccountCreateRouted)
		c.startAccountCreate(ctx, b, chatID, data.UserID(), conversation.StringOrEmpty(data[conversation.KeyMessage]))
	case flow.OpRename:
		c.finishAccountRename(ctx, b, chatID, data)
	case flow.OpAdjust:
		c.finishAccountAdjust(ctx, b, chatID, data) // Task 7
	case flow.OpDefault:
		c.finishAccountDefault(ctx, b, chatID, data) // Task 8
	default:
		c.sendText(ctx, b, chatID, msgSomethingBroke)
	}
}

func (c *controller) finishAccountRename(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	id, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyAccountID]), 10, 64)
	if err != nil {
		c.sendText(ctx, b, chatID, msgSomethingBroke)
		return
	}
	newName := conversation.StringOrEmpty(data[conversation.KeyNewName])
	if err := c.accounts.Rename(id, newName); err != nil {
		if errors.Is(err, account.ErrAccountAlreadyExists) {
			c.sendText(ctx, b, chatID, account.MsgAccountAlreadyExists(newName, conversation.StringOrEmpty(data[conversation.KeyAccountCurrency])))
			return
		}
		c.sendText(ctx, b, chatID, msgCouldNotSave("el cambio"))
		return
	}
	c.resolveMetric(ctx, data.UserID(), outcomeAccountRenamed)
	c.sendText(ctx, b, chatID, "Listo, ahora se llama "+newName+".")
}

// finishAccountAdjust inserts THE adjustment movement: delta between the
// declared new total and SUM(amount). The app owns the sign here exactly
// like the guard does: income → +Abs, expense → -Abs. Never the LLM (the
// LLM never even saw the number — it came from a validated TextStep).
func (c *controller) finishAccountAdjust(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	accountID, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyAccountID]), 10, 64)
	if err != nil {
		c.sendText(ctx, b, chatID, msgSomethingBroke)
		return
	}
	newTotal, err := movement.ParseARAmount(conversation.StringOrEmpty(data[conversation.KeyNewTotal]))
	if err != nil {
		c.sendText(ctx, b, chatID, msgSomethingBroke)
		return
	}
	current, err := c.movements.SumAmountForAccount(accountID)
	if err != nil {
		c.sendText(ctx, b, chatID, msgCouldNotLoad)
		return
	}

	delta := newTotal.Sub(current)
	if delta.IsZero() {
		c.resolveMetric(ctx, data.UserID(), outcomeAccountAdjusted)
		c.sendText(ctx, b, chatID, msgAccountManageNoChange)
		return
	}

	sub, err := c.subcategories.FindByCategoryAndSubcategory(data.UserID(), subcategory.CategorySystem, "Ajuste de saldo")
	if err != nil {
		c.sendText(ctx, b, chatID, msgCouldNotLoad)
		return
	}

	// La cuenta se relee de la DB para que el guard compare la moneda del
	// movimiento contra la verdad de la base, y no contra lo que dice la Data del
	// flujo. Si las dos discrepan, el ajuste se rechaza en vez de escribir un
	// movimiento en una moneda distinta a la de su cuenta — que es el error que
	// corrompe un balance en silencio, porque el saldo es SUM(amount) y no mira
	// la moneda de cada fila.
	acc, err := c.accounts.GetAccount(accountID)
	if err != nil {
		c.sendText(ctx, b, chatID, msgCouldNotLoad)
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
			"user_id", data.UserID(), "account_id", accountID, "reason", guardReason(err))
		c.sendText(ctx, b, chatID, createErrorCopy(err))
		return
	}

	if err := c.movements.InsertBatch(movs); err != nil {
		// Mismo motivo que en account_create_finish: función void, el error no
		// sube a withTrace. Sin este log, un ajuste de saldo fallido es
		// invisible en producción.
		slog.ErrorContext(ctx, "balance adjustment insert failed",
			"user_id", data.UserID(), "account_id", accountID, "err", err)
		c.sendText(ctx, b, chatID, msgCouldNotSave("el ajuste"))
		return
	}
	c.resolveMetric(ctx, data.UserID(), outcomeAccountAdjusted)
	c.sendText(ctx, b, chatID, fmt.Sprintf("%s: %s %s.",
		conversation.StringOrEmpty(data[conversation.KeyAccountName]), newTotal.String(), conversation.StringOrEmpty(data[conversation.KeyAccountCurrency])))
}

func (c *controller) finishAccountDefault(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	accountID, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyAccountID]), 10, 64)
	if err != nil {
		c.sendText(ctx, b, chatID, msgSomethingBroke)
		return
	}
	cur := currency.Currency(conversation.StringOrEmpty(data[conversation.KeyAccountCurrency]))
	name := conversation.StringOrEmpty(data[conversation.KeyAccountName])

	// capture the previous default BEFORE unsetting it
	prev, prevErr := c.accounts.FindDefaultByCurrency(data.UserID(), cur)

	if err := c.accounts.UnsetDefault(data.UserID(), cur); err != nil {
		c.sendText(ctx, b, chatID, msgCouldNotSave("el cambio"))
		return
	}
	if err := c.accounts.SetDefault(accountID); err != nil {
		c.sendText(ctx, b, chatID, msgCouldNotSave("el cambio"))
		return
	}
	c.resolveMetric(ctx, data.UserID(), outcomeAccountDefaultSet)
	c.sendText(ctx, b, chatID, fmt.Sprintf("⭐ %s es tu cuenta en %s por defecto.", name, cur.String()))

	// same-currency guaranteed: prev is the old default OF THIS currency
	if prevErr != nil || prev == nil || uint64(prev.ID) == accountID {
		return
	}
	balance, err := c.movements.SumAmountForAccount(uint64(prev.ID))
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
	prompt, err := c.engine.StartWithData(data.UserID(), flow.AccountMoveOfferFlowName, seed)
	if err != nil {
		return
	}
	c.sendPrompt(ctx, b, chatID, prompt)
}

func (c *controller) finishAccountMoveOffer(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if conversation.StringOrEmpty(data[conversation.KeyMoveChoice]) != flow.MoveChoiceMove {
		c.sendText(ctx, b, chatID, "Listo, dejé todo como estaba.")
		return
	}
	fromID, err1 := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyMoveFromID]), 10, 64)
	toID, err2 := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyMoveToID]), 10, 64)
	if err1 != nil || err2 != nil {
		c.sendText(ctx, b, chatID, msgSomethingBroke)
		return
	}
	if err := c.movements.ReassignAccount(fromID, toID); err != nil {
		c.sendText(ctx, b, chatID, msgCouldNotSave("el cambio"))
		return
	}
	c.sendText(ctx, b, chatID, fmt.Sprintf("Listo: los movimientos de %s ahora están en %s. %s quedó en 0.",
		conversation.StringOrEmpty(data[conversation.KeyMoveFromName]), conversation.StringOrEmpty(data[conversation.KeyMoveToName]), conversation.StringOrEmpty(data[conversation.KeyMoveFromName])))
}
