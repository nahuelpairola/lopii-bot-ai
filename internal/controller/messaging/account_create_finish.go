package messaging

import (
	"context"
	"errors"
	"log/slog"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

// finishAccountCreateFlow is the Telegram-facing wrapper around the real
// work, same split as finishInitialBalanceFlow/insertInitialBalanceMovements:
// it creates the account.Account (never default — see server.go's onboarding,
// which already guarantees default ARS/USD wallets exist) and one opening
// transfer movement, mirroring insertInitialBalanceMovements.
func (c *controller) finishAccountCreateFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		c.sendText(ctx, b, chatID, msgAccountCreateCancelled)
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
	if err := c.accounts.Insert(newAccount); err != nil {
		if errors.Is(err, account.ErrAccountAlreadyExists) {
			c.sendText(ctx, b, chatID, account.MsgAccountAlreadyExists(name, cur))
			return
		}
		c.sendText(ctx, b, chatID, msgCouldNotSave("tu cuenta"))
		return
	}

	if err := c.insertAccountOpeningMovement(newAccount, balance); err != nil {
		// Se loguea porque acá se corta: esta función no devuelve error, así que
		// sin esto un saldo de apertura que falla no deja rastro en ningún lado
		// (ni en slog ni en request_traces) y la cuenta queda creada sin él.
		slog.ErrorContext(ctx, "account opening movement failed",
			"user_id", data.UserID(), "account_id", newAccount.ID, "err", err)
		c.sendText(ctx, b, chatID, msgCouldNotSave("tu cuenta"))
		return
	}

	c.sendText(ctx, b, chatID, msgAccountCreateSuccess(name, cur, balance))
}

// insertAccountOpeningMovement inserts the opening transfer movement for
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
func (c *controller) insertAccountOpeningMovement(acc *account.Account, balanceText string) error {
	sub, err := c.subcategories.FindByCategoryAndSubcategory(acc.UserID, subcategory.CategorySystem, subcategory.SubOpeningBalance)
	if err != nil {
		return err
	}
	amount, err := movement.ParseARAmount(balanceText)
	if err != nil {
		return err
	}

	accountID := uint64(acc.ID)
	return c.movements.InsertBatch([]movement.Movement{{
		UserID:        acc.UserID,
		AccountID:     &accountID,
		SubcategoryID: uint64(sub.ID),
		Date:          movement.TodayCivil(),
		Type:          movement.Transfer,
		Amount:        amount,
		Currency:      acc.Currency,
	}})
}
