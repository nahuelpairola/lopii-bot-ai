package messaging

import (
	"context"
	"errors"
	"time"

	"github.com/go-telegram/bot"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

// finishAccountCreateFlow is the Telegram-facing wrapper around the real
// work, same split as finishInitialBalanceFlow/insertInitialBalanceMovements:
// it creates the account.Account (never default — see server.go's onboarding,
// which already guarantees default ARS/USD wallets exist) and one opening
// transfer movement, mirroring insertInitialBalanceMovements.
func (c *controller) finishAccountCreateFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if stringOrEmpty(data["cancelled"]) == "true" {
		c.sendText(ctx, b, chatID, msgAccountCreateCancelled)
		return
	}

	name := stringOrEmpty(data["account_name"])
	cur := stringOrEmpty(data["account_currency"])
	balance := stringOrEmpty(data["account_balance"])

	newAccount := &account.Account{
		UserID:   data.UserID(),
		Name:     name,
		Currency: currency.Currency(cur),
	}
	if err := c.accounts.Insert(newAccount); err != nil {
		if errors.Is(err, account.ErrAccountAlreadyExists) {
			c.sendText(ctx, b, chatID, account.MsgAccountAlreadyExists(name, cur))
			return
		}
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}

	if err := c.insertAccountOpeningMovement(data.UserID(), uint64(newAccount.ID), cur, balance); err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}

	c.sendText(ctx, b, chatID, msgAccountCreateSuccess(name, cur, balance))
}

// insertAccountOpeningMovement inserts the opening transfer movement for
// a freshly created account, same subcategory
// (Sistema | Saldo inicial) and shape as insertInitialBalanceMovements —
// inserted even when balance is "0", for the same reason: the balance is
// always computed from movements, never stored (see movement.SumAmountForAccount).
func (c *controller) insertAccountOpeningMovement(userID, accountID uint64, cur, balanceText string) error {
	sub, err := c.subcategories.FindByCategoryAndSubcategory("Sistema", "Saldo inicial")
	if err != nil {
		return err
	}
	amount, err := decimal.NewFromString(balanceText)
	if err != nil {
		return err
	}

	m := movement.Movement{
		UserID:        userID,
		AccountID:     &accountID,
		SubcategoryID: uint64(sub.ID),
		Date:          time.Now(),
		Type:          movement.Transfer,
		Amount:        amount,
		Currency:      currency.Currency(cur),
	}
	return c.movements.InsertBatch([]movement.Movement{m})
}
