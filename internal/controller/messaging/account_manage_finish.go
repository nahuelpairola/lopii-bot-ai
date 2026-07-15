package messaging

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

// finishAccountManageFlow applies the confirmed operation. Every branch
// already passed its confirm gate inside the flow — this is pure execution.
func (c *controller) finishAccountManageFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if stringOrEmpty(data["cancelled"]) == "true" {
		c.resolveMetric(data.UserID(), outcomeAccountManageCancelled)
		c.sendText(ctx, b, chatID, msgAccountManageCancelled)
		return
	}

	switch stringOrEmpty(data["operation"]) {
	case "create_new":
		c.resolveMetric(data.UserID(), outcomeAccountCreateRouted)
		c.startAccountCreate(ctx, b, chatID, data.UserID(), stringOrEmpty(data["message"]))
	case "rename":
		c.finishAccountRename(ctx, b, chatID, data)
	case "adjust":
		c.finishAccountAdjust(ctx, b, chatID, data) // Task 7
	case "default":
		c.finishAccountDefault(ctx, b, chatID, data) // Task 8
	default:
		c.sendText(ctx, b, chatID, msgGenericFlowError)
	}
}

func (c *controller) finishAccountRename(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	id, err := strconv.ParseUint(stringOrEmpty(data["account_id"]), 10, 64)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}
	newName := stringOrEmpty(data["new_name"])
	if err := c.accounts.Rename(id, newName); err != nil {
		if errors.Is(err, account.ErrAccountAlreadyExists) {
			c.sendText(ctx, b, chatID, account.MsgAccountAlreadyExists(newName, stringOrEmpty(data["account_currency"])))
			return
		}
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}
	c.resolveMetric(data.UserID(), outcomeAccountRenamed)
	c.sendText(ctx, b, chatID, "Listo, ahora se llama "+newName+".")
}

// finishAccountAdjust inserts THE adjustment movement: delta between the
// declared new total and SUM(amount). The app owns the sign here exactly
// like the guard does: income → +Abs, expense → -Abs. Never the LLM (the
// LLM never even saw the number — it came from a validated TextStep).
func (c *controller) finishAccountAdjust(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	accountID, err := strconv.ParseUint(stringOrEmpty(data["account_id"]), 10, 64)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}
	newTotal, err := decimal.NewFromString(stringOrEmpty(data["new_total"]))
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}
	current, err := c.movements.SumAmountForAccount(accountID)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}

	delta := newTotal.Sub(current)
	if delta.IsZero() {
		c.resolveMetric(data.UserID(), outcomeAccountAdjusted)
		c.sendText(ctx, b, chatID, msgAccountManageNoChange)
		return
	}

	sub, err := c.subcategories.FindByCategoryAndSubcategory(data.UserID(), "Sistema", "Ajuste de saldo")
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
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
		Currency:      currency.Currency(stringOrEmpty(data["account_currency"])),
	}
	if err := c.movements.InsertBatch([]movement.Movement{m}); err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}
	c.resolveMetric(data.UserID(), outcomeAccountAdjusted)
	c.sendText(ctx, b, chatID, fmt.Sprintf("%s: %s %s.",
		stringOrEmpty(data["account_name"]), newTotal.String(), stringOrEmpty(data["account_currency"])))
}

func (c *controller) finishAccountDefault(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	c.sendText(ctx, b, chatID, msgGenericFlowError) // replaced in Task 8
}
