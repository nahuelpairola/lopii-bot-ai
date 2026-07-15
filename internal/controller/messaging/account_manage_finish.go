package messaging

import (
	"context"
	"errors"
	"strconv"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
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

func (c *controller) finishAccountAdjust(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	c.sendText(ctx, b, chatID, msgGenericFlowError) // replaced in Task 7
}

func (c *controller) finishAccountDefault(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	c.sendText(ctx, b, chatID, msgGenericFlowError) // replaced in Task 8
}
