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

	optionBack        = "back"
	optionConfirmSeed = "confirm_seed"
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
	setFlag(next, keyCancelled)
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
			DataKey:    keyAccountName,
			Validate: func(text string, _ conversation.Data) string {
				if text == "" {
					return msgInvalidAccountCreateName
				}
				return ""
			},
			NextStep:      stepAccountCreateAskCurrency,
			EscapeOptions: []conversation.ChoiceOption{cancelOption},
			EscapeOptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				name := stringOrEmpty(data[keyAccountName])
				if name == "" {
					return nil
				}
				return []conversation.ChoiceOption{
					{Label: "✅ Usar " + name, Value: optionConfirmSeed, NextStep: stepAccountCreateAskCurrency},
				}
			},
			OnEscape: onAccountCreateEscape,
		},
		stepAccountCreateAskCurrency: conversation.ChoiceStep{
			PromptText: msgAskAccountCreateCurrency,
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				opts := make([]conversation.ChoiceOption, 0, len(currency.SupportedCurrencies)+2)
				for _, cu := range currency.SupportedCurrencies {
					opts = append(opts, conversation.ChoiceOption{
						// El botón dice "💱 pesos"; el Value sigue siendo el
						// código, que es lo que se guarda. Label y Value son dos
						// cosas distintas justamente acá.
						Label:    "💱 " + cu.Label(),
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
				next[keyAccountCurrency] = value
				return next
			},
			InvalidChoiceMessage: msgInvalidChoice,
		},
		stepAccountCreateAskBalance: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				return account.MsgAskInitialBalance(stringOrEmpty(data[keyAccountName]), stringOrEmpty(data[keyAccountCurrency]))
			},
			DataKey:  keyAccountBalance,
			Validate: validateBalanceAmount,
			NextStep: stepAccountCreateConfirm,
			EscapeOptions: []conversation.ChoiceOption{
				{Label: "⬅️ Atrás", Value: optionBack, NextStep: stepAccountCreateAskCurrency},
				cancelOption,
			},
			EscapeOptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				bal := stringOrEmpty(data[keyAccountBalance])
				if bal == "" {
					return nil
				}
				label := "✅ Usar " + bal
				if cur := stringOrEmpty(data[keyAccountCurrency]); cur != "" {
					label += " " + cur
				}
				return []conversation.ChoiceOption{
					{Label: label, Value: optionConfirmSeed, NextStep: stepAccountCreateConfirm},
				}
			},
			OnEscape: onAccountCreateEscape,
		},
		stepAccountCreateConfirm: conversation.ChoiceStep{
			PromptText: msgConfirmAccountCreate,
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: optionConfirm, Finish: true},
				{Label: "⬅️ Atrás", Value: optionBack, NextStep: stepAccountCreateAskBalance},
				cancelOption,
			},
			OnChoice:             onAccountCreateEscape,
			InvalidChoiceMessage: msgInvalidChoice,
		},
	}

	flow, err := conversation.NewFlow(accountCreateFlowName, stepAccountCreateAskName, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

// validateBalanceAmount validates that text is a valid decimal amount
// (non-negative). Used in account creation and initial balance flows.
func validateBalanceAmount(text string, _ conversation.Data) string {
	amount, err := parseARAmount(text)
	if err != nil || amount.IsNegative() {
		return account.MsgInvalidAmount
	}
	return ""
}
