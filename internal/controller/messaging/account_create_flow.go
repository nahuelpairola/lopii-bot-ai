package messaging

import (
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
)

const (
	accountCreateFlowName = "account_create"

	stepAccountCreateAskName     = "account_create_ask_name"
	stepAccountCreateAskCurrency = "account_create_ask_currency"
	stepAccountCreateAskBalance  = "account_create_ask_balance"
	stepAccountCreateConfirm     = "account_create_confirm"

	optionBack = "back"
)

// onAccountCreateEscape is the shared OnEscape/OnChoice handler for this
// flow's Cancelar and Atrás buttons: Cancelar flags cancelled=true (read
// by finishAccountCreateFlow to skip every DB write); Atrás just moves to
// the declared NextStep with data untouched.
func onAccountCreateEscape(value string, data conversation.Data) conversation.Data {
	if value != optionCancel {
		return data
	}
	next := copyData(data)
	next["cancelled"] = "true"
	return next
}

// NewAccountCreateFlow builds the 4-step flow for creating an additional,
// purpose-specific account (investment, retirement, savings, etc.) —
// creation only, cancelable/back-able at every step. Started by
// startAccountCreate (free_text.go) whenever Call 1 classifies a message
// as ACCOUNT_CREATE.
func NewAccountCreateFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepAccountCreateAskName: conversation.TextStep{
			PromptText: msgAskAccountCreateName,
			DataKey:    "account_name",
			Validate: func(text string, _ conversation.Data) string {
				if text == "" {
					return msgInvalidAccountCreateName
				}
				return ""
			},
			NextStep:      stepAccountCreateAskCurrency,
			EscapeOptions: []conversation.ChoiceOption{cancelOption},
			OnEscape:      onAccountCreateEscape,
		},
		stepAccountCreateAskCurrency: conversation.ChoiceStep{
			PromptText: msgAskAccountCreateCurrency,
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				opts := make([]conversation.ChoiceOption, 0, len(currency.SupportedCurrencies)+2)
				for _, cu := range currency.SupportedCurrencies {
					opts = append(opts, conversation.ChoiceOption{
						Label:    cu.String(),
						Value:    cu.String(),
						NextStep: stepAccountCreateAskBalance,
					})
				}
				opts = append(opts,
					conversation.ChoiceOption{Label: "⬅️ Atrás", Value: optionBack, NextStep: stepAccountCreateAskName},
					cancelOption,
				)
				return opts
			},
			DeclaredNextSteps: []string{stepAccountCreateAskBalance, stepAccountCreateAskName},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == optionCancel || value == optionBack {
					return onAccountCreateEscape(value, data)
				}
				next := copyData(data)
				next["account_currency"] = value
				return next
			},
			InvalidChoiceMessage: msgGenericFlowError,
		},
		stepAccountCreateAskBalance: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				return account.MsgAskInitialBalance(stringOrEmpty(data["account_name"]), stringOrEmpty(data["account_currency"]))
			},
			DataKey:  "account_balance",
			Validate: validateBalanceAmount,
			NextStep: stepAccountCreateConfirm,
			EscapeOptions: []conversation.ChoiceOption{
				{Label: "⬅️ Atrás", Value: optionBack, NextStep: stepAccountCreateAskCurrency},
				cancelOption,
			},
			OnEscape: onAccountCreateEscape,
		},
		stepAccountCreateConfirm: conversation.ChoiceStep{
			PromptText: msgConfirmAccountCreate,
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: "confirm", Finish: true},
				{Label: "⬅️ Atrás", Value: optionBack, NextStep: stepAccountCreateAskBalance},
				cancelOption,
			},
			OnChoice: onAccountCreateEscape,
			InvalidChoiceMessage: msgGenericFlowError,
		},
	}

	flow, err := conversation.NewFlow(accountCreateFlowName, stepAccountCreateAskName, steps)
	if err != nil {
		panic(err)
	}
	return flow
}
