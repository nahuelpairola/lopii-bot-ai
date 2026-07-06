package messaging

import (
	"context"
	"time"

	"github.com/go-telegram/bot"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

const (
	initialBalanceFlowName = "initial_balance_setup"

	stepConfirmBalances = "confirm_balances"
)

// askBalanceStepName y balanceDataKey derivan el nombre de step / data key
// a partir de la moneda, en vez de tener un const fijo por moneda: agregar
// una moneda a currency.SupportedCurrencies alcanza para que este flow la
// contemple, sin tocar este archivo.
func askBalanceStepName(cu currency.Currency) string {
	return "ask_balance_" + cu.String()
}

func balanceDataKey(cu currency.Currency) string {
	return "balance_" + cu.String()
}

// NewInitialBalanceFlow pide el saldo actual de cada wallet default (una
// por moneda en currency.SupportedCurrencies, creadas en /start) y lo
// confirma antes de terminar. Se arranca automáticamente al final de
// /start (ver start.go).
func NewInitialBalanceFlow() *conversation.Flow {
	currencies := currency.SupportedCurrencies

	steps := map[string]conversation.Step{
		stepConfirmBalances: conversation.ChoiceStep{
			PromptText: msgConfirmInitialBalances,
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: "confirm", Finish: true},
				{Label: "✏️ Corregir", Value: "retry", NextStep: askBalanceStepName(currencies[0])},
			},
			InvalidChoiceMessage: msgGenericFlowError,
		},
	}

	for i, cu := range currencies {
		next := stepConfirmBalances
		if i+1 < len(currencies) {
			next = askBalanceStepName(currencies[i+1])
		}
		steps[askBalanceStepName(cu)] = conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				return account.MsgAskInitialBalance(constants.DefaultWalletName, cu.String())
			},
			DataKey:  balanceDataKey(cu),
			Validate: validateBalanceAmount,
			NextStep: next,
		}
	}

	flow, err := conversation.NewFlow(initialBalanceFlowName, askBalanceStepName(currencies[0]), steps)
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

	sub, err := c.subcategories.FindByCategoryAndSubcategory(userID, "Sistema", "Saldo inicial")
	if err != nil {
		return err
	}
	subcategoryID := uint64(sub.ID)

	today := time.Now()
	movements := make([]movement.Movement, 0, len(currency.SupportedCurrencies))
	for _, cu := range currency.SupportedCurrencies {
		amountText, _ := data[balanceDataKey(cu)].(string)
		amount, err := decimal.NewFromString(amountText)
		if err != nil {
			return err
		}

		acc, err := c.accounts.FindDefaultByCurrency(userID, cu)
		if err != nil {
			return err
		}
		accountID := uint64(acc.ID)

		movements = append(movements, movement.Movement{
			UserID:        userID,
			AccountID:     &accountID,
			SubcategoryID: subcategoryID,
			Date:          today,
			Type:          movement.Transfer,
			Amount:        amount,
			Currency:      cu,
		})
	}

	return c.movements.InsertBatch(movements)
}
