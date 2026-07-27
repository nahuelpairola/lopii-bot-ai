package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

// finishAccountManageFlow applies the confirmed operation. Every branch
// already passed its confirm gate inside the flow — this is pure execution.
func (c *controller) finishAccountManageFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if flag(data, keyCancelled) {
		c.resolveMetric(ctx, data.UserID(), outcomeAccountManageCancelled)
		c.sendText(ctx, b, chatID, msgFlowCancelled)
		return
	}

	switch stringOrEmpty(data[keyOperation]) {
	case opCreateNew:
		c.resolveMetric(ctx, data.UserID(), outcomeAccountCreateRouted)
		c.startAccountCreate(ctx, b, chatID, data.UserID(), stringOrEmpty(data[keyMessage]))
	case opRename:
		c.finishAccountRename(ctx, b, chatID, data)
	case opAdjust:
		c.finishAccountAdjust(ctx, b, chatID, data) // Task 7
	case opDefault:
		c.finishAccountDefault(ctx, b, chatID, data) // Task 8
	default:
		c.sendText(ctx, b, chatID, msgSomethingBroke)
	}
}

func (c *controller) finishAccountRename(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	id, err := strconv.ParseUint(stringOrEmpty(data[keyAccountID]), 10, 64)
	if err != nil {
		c.sendText(ctx, b, chatID, msgSomethingBroke)
		return
	}
	newName := stringOrEmpty(data[keyNewName])
	if err := c.accounts.Rename(id, newName); err != nil {
		if errors.Is(err, account.ErrAccountAlreadyExists) {
			c.sendText(ctx, b, chatID, account.MsgAccountAlreadyExists(newName, stringOrEmpty(data[keyAccountCurrency])))
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
	accountID, err := strconv.ParseUint(stringOrEmpty(data[keyAccountID]), 10, 64)
	if err != nil {
		c.sendText(ctx, b, chatID, msgSomethingBroke)
		return
	}
	newTotal, err := parseARAmount(stringOrEmpty(data[keyNewTotal]))
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

	sub, err := c.subcategories.FindByCategoryAndSubcategory(data.UserID(), "Sistema", "Ajuste de saldo")
	if err != nil {
		c.sendText(ctx, b, chatID, msgCouldNotLoad)
		return
	}

	mType := movement.Income
	amount := delta.Abs()
	if delta.IsNegative() {
		mType = movement.Expense
		amount = delta.Abs().Neg()
	}
	m := movement.Movement{
		UserID:        data.UserID(),
		AccountID:     &accountID,
		SubcategoryID: uint64(sub.ID),
		Date:          time.Now(),
		Type:          mType,
		Amount:        amount,
		Currency:      currency.Currency(stringOrEmpty(data[keyAccountCurrency])),
	}
	if err := c.movements.InsertBatch([]movement.Movement{m}); err != nil {
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
		stringOrEmpty(data[keyAccountName]), newTotal.String(), stringOrEmpty(data[keyAccountCurrency])))
}

func (c *controller) finishAccountDefault(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	accountID, err := strconv.ParseUint(stringOrEmpty(data[keyAccountID]), 10, 64)
	if err != nil {
		c.sendText(ctx, b, chatID, msgSomethingBroke)
		return
	}
	cur := currency.Currency(stringOrEmpty(data[keyAccountCurrency]))
	name := stringOrEmpty(data[keyAccountName])

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
		keyMoveFromID:      strconv.FormatUint(uint64(prev.ID), 10),
		keyMoveFromName:    prev.Name,
		keyMoveFromBalance: balance.String(),
		keyMoveToID:        strconv.FormatUint(accountID, 10),
		keyMoveToName:      name,
	}
	prompt, err := c.engine.StartWithData(data.UserID(), accountMoveOfferFlowName, seed)
	if err != nil {
		return
	}
	c.sendPrompt(ctx, b, chatID, prompt)
}

func (c *controller) finishAccountMoveOffer(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if stringOrEmpty(data[keyMoveChoice]) != moveChoiceMove {
		c.sendText(ctx, b, chatID, "Listo, dejé todo como estaba.")
		return
	}
	fromID, err1 := strconv.ParseUint(stringOrEmpty(data[keyMoveFromID]), 10, 64)
	toID, err2 := strconv.ParseUint(stringOrEmpty(data[keyMoveToID]), 10, 64)
	if err1 != nil || err2 != nil {
		c.sendText(ctx, b, chatID, msgSomethingBroke)
		return
	}
	if err := c.movements.ReassignAccount(fromID, toID); err != nil {
		c.sendText(ctx, b, chatID, msgCouldNotSave("el cambio"))
		return
	}
	c.sendText(ctx, b, chatID, fmt.Sprintf("Listo: los movimientos de %s ahora están en %s. %s quedó en 0.",
		stringOrEmpty(data[keyMoveFromName]), stringOrEmpty(data[keyMoveToName]), stringOrEmpty(data[keyMoveFromName])))
}
