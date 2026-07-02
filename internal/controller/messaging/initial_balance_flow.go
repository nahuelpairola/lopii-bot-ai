package messaging

import (
	"context"
	"time"

	"github.com/go-telegram/bot"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

const (
	initialBalanceFlowName = "initial_balance_setup"

	stepAskARSBalance   = "ask_ars_balance"
	stepAskUSDBalance   = "ask_usd_balance"
	stepConfirmBalances = "confirm_balances"

	dataKeyARSBalance = "ars_balance"
	dataKeyUSDBalance = "usd_balance"
)

// NewInitialBalanceFlow pide el saldo actual de las wallets ARS y USD
// (creadas en /start) y lo confirma antes de terminar. Se arranca
// automáticamente al final de /start (ver start.go).
func NewInitialBalanceFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepAskARSBalance: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				return account.MsgAskInitialBalance("Wallet", "ARS")
			},
			DataKey:  dataKeyARSBalance,
			Validate: validateBalanceAmount,
			NextStep: stepAskUSDBalance,
		},
		stepAskUSDBalance: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				return account.MsgAskInitialBalance("Wallet", "USD")
			},
			DataKey:  dataKeyUSDBalance,
			Validate: validateBalanceAmount,
			NextStep: stepConfirmBalances,
		},
		stepConfirmBalances: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				ars, _ := data[dataKeyARSBalance].(string)
				usd, _ := data[dataKeyUSDBalance].(string)
				return msgConfirmInitialBalances(ars, usd)
			},
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: "confirm", Finish: true},
				{Label: "✏️ Corregir", Value: "retry", NextStep: stepAskARSBalance},
			},
			InvalidChoiceMessage: msgGenericFlowError,
		},
	}

	flow, err := conversation.NewFlow(initialBalanceFlowName, stepAskARSBalance, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

func validateBalanceAmount(text string, _ conversation.Data) string {
	amount, err := decimal.NewFromString(text)
	if err != nil || amount.IsNegative() {
		return account.MsgInvalidAmount
	}
	return ""
}

// finishInitialBalanceFlow resuelve el completion de "initial_balance_setup":
// inserta el saldo cargado en cada wallet como un movement de apertura, y le
// avisa al usuario si salió bien o mal.
func (c *controller) finishInitialBalanceFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if err := c.insertInitialBalanceMovements(data); err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgGenericFlowError})
		return
	}
	b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgAccountSetupFinished})
}

// insertInitialBalanceMovements hace el trabajo real de
// finishInitialBalanceFlow (sin depender de *bot.Bot, para poder testearlo
// aislado): inserta el saldo cargado en cada wallet como un movement de
// apertura (type=transfer, subcategoría "Sistema | Saldo inicial"), en una
// sola transacción para no dejar un saldo cargado y el otro no.
func (c *controller) insertInitialBalanceMovements(data conversation.Data) error {
	userID := data.UserID()

	arsText, _ := data[dataKeyARSBalance].(string)
	usdText, _ := data[dataKeyUSDBalance].(string)

	arsAmount, err := decimal.NewFromString(arsText)
	if err != nil {
		return err
	}
	usdAmount, err := decimal.NewFromString(usdText)
	if err != nil {
		return err
	}

	sub, err := c.subcategories.FindByCategoryAndSubcategory("Sistema", "Saldo inicial")
	if err != nil {
		return err
	}
	subcategoryID := uint64(sub.ID)

	arsAccount, err := c.accounts.FindDefaultByCurrency(userID, currency.ARS)
	if err != nil {
		return err
	}
	usdAccount, err := c.accounts.FindDefaultByCurrency(userID, currency.USD)
	if err != nil {
		return err
	}
	arsAccountID := uint64(arsAccount.ID)
	usdAccountID := uint64(usdAccount.ID)

	today := time.Now()
	movements := []movement.Movement{
		{
			UserID:        userID,
			AccountID:     &arsAccountID,
			SubcategoryID: subcategoryID,
			Date:          today,
			Type:          movement.Transfer,
			Amount:        arsAmount,
			Currency:      currency.ARS,
		},
		{
			UserID:        userID,
			AccountID:     &usdAccountID,
			SubcategoryID: subcategoryID,
			Date:          today,
			Type:          movement.Transfer,
			Amount:        usdAmount,
			Currency:      currency.USD,
		},
	}

	return c.movements.InsertBatch(movements)
}
