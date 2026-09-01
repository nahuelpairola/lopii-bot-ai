package flow

import (
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

const (
	StepAccountCreateAskName     = "account_create_ask_name"
	StepAccountCreateAskCurrency = "account_create_ask_currency"
	StepAccountCreateAskBalance  = "account_create_ask_balance"
	StepAccountCreateConfirm     = "account_create_confirm"
)

func OnAccountCreateEscape(value string, data conversation.Data) conversation.Data {
	if value != OptionCancel {
		return data
	}
	next := conversation.CopyData(data)
	conversation.SetFlag(next, conversation.KeyCancelled)
	return next
}

func NewAccountCreateFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		StepAccountCreateAskName: conversation.TextStep{
			PromptText: MsgAskAccountCreateName,
			DataKey:    conversation.KeyAccountName,
			Validate: func(text string, _ conversation.Data) string {
				if text == "" {
					return MsgInvalidAccountCreateName
				}
				return ""
			},
			NextStep:      StepAccountCreateAskCurrency,
			EscapeOptions: []conversation.ChoiceOption{CancelOption},
			EscapeOptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				name := conversation.StringOrEmpty(data[conversation.KeyAccountName])
				if name == "" {
					return nil
				}
				return []conversation.ChoiceOption{
					{Label: "✅ Usar " + name, Value: OptionConfirmSeed, NextStep: StepAccountCreateAskCurrency},
				}
			},
			OnEscape: OnAccountCreateEscape,
		},
		StepAccountCreateAskCurrency: conversation.ChoiceStep{
			PromptText: MsgAskAccountCreateCurrency,
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				opts := make([]conversation.ChoiceOption, 0, len(currency.SupportedCurrencies)+2)
				for _, cu := range currency.SupportedCurrencies {
					opts = append(opts, conversation.ChoiceOption{
						Label:    "💱 " + cu.Label(),
						Value:    cu.String(),
						NextStep: StepAccountCreateAskBalance,
					})
				}
				opts = append(opts,
					conversation.ChoiceOption{Label: "⬅️ Atrás", Value: OptionBack, NextStep: StepAccountCreateAskName},
					CancelOption,
				)
				return opts
			},
			DeclaredNextSteps: []string{StepAccountCreateAskBalance, StepAccountCreateAskName},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == OptionCancel || value == OptionBack {
					return OnAccountCreateEscape(value, data)
				}
				next := conversation.CopyData(data)
				next[conversation.KeyAccountCurrency] = value
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},
		StepAccountCreateAskBalance: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				return account.MsgAskInitialBalance(conversation.StringOrEmpty(data[conversation.KeyAccountName]), conversation.StringOrEmpty(data[conversation.KeyAccountCurrency]))
			},
			DataKey:  conversation.KeyAccountBalance,
			Validate: ValidateBalanceAmount,
			NextStep: StepAccountCreateConfirm,
			EscapeOptions: []conversation.ChoiceOption{
				{Label: "⬅️ Atrás", Value: OptionBack, NextStep: StepAccountCreateAskCurrency},
				CancelOption,
			},
			EscapeOptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				bal := conversation.StringOrEmpty(data[conversation.KeyAccountBalance])
				if bal == "" {
					return nil
				}
				label := "✅ Usar " + bal
				if cur := conversation.StringOrEmpty(data[conversation.KeyAccountCurrency]); cur != "" {
					label += " " + cur
				}
				return []conversation.ChoiceOption{
					{Label: label, Value: OptionConfirmSeed, NextStep: StepAccountCreateConfirm},
				}
			},
			OnEscape: OnAccountCreateEscape,
		},
		StepAccountCreateConfirm: conversation.ChoiceStep{
			PromptText: MsgConfirmAccountCreate,
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: OptionConfirm, Finish: true},
				{Label: "⬅️ Atrás", Value: OptionBack, NextStep: StepAccountCreateAskBalance},
				CancelOption,
			},
			OnChoice:             OnAccountCreateEscape,
			InvalidChoiceMessage: MsgInvalidChoice,
		},
	}

	flow, err := conversation.NewFlow(AccountCreateFlowName, StepAccountCreateAskName, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

func ValidateBalanceAmount(text string, _ conversation.Data) string {
	amount, err := movement.ParseARAmount(text)
	if err != nil || amount.IsNegative() {
		return account.MsgInvalidAmount
	}
	return ""
}
