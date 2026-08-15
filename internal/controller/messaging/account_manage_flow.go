package messaging

import (
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
)

const (
	accountManageFlowName = "account_manage"

	stepAccountManagePick           = "account_manage_pick"
	stepAccountManageMenu           = "account_manage_menu"
	stepAccountManageAskName        = "account_manage_ask_name"
	stepAccountManageConfirmRename  = "account_manage_confirm_rename"
	stepAccountManageAskTotal       = "account_manage_ask_total"
	stepAccountManageConfirmAdjust  = "account_manage_confirm_adjust"
	stepAccountManageConfirmDefault = "account_manage_confirm_default"

	optionManageRename  = "op_rename"
	optionManageAdjust  = "op_adjust"
	optionManageDefault = "op_default"
	optionManageCreate  = "op_create_new"

	// operation values stored under conversation.KeyOperation and switched on in
	// account_manage_finish.go — distinct from the optionManage* button values.
	opRename    = "rename"
	opAdjust    = "adjust"
	opDefault   = "default"
	opCreateNew = "create_new"
)

// balanceSummer is the narrow read the flow needs to show saldo actual.
type balanceSummer interface {
	SumAmountForAccount(accountID uint64) (decimal.Decimal, error)
}

// onAccountManageCancel marks the flow cancelled (finishAccountManageFlow
// skips every write) — same contract as onAccountCreateEscape.
func onAccountManageCancel(value string, data conversation.Data) conversation.Data {
	if value != optionCancel {
		return data
	}
	next := conversation.CopyData(data)
	conversation.SetFlag(next, conversation.KeyCancelled)
	return next
}

func accountManageBalance(balances balanceSummer, data conversation.Data) decimal.Decimal {
	id, err := strconv.ParseUint(conversation.StringOrEmpty(data[conversation.KeyAccountID]), 10, 64)
	if err != nil {
		return decimal.Zero
	}
	sum, err := balances.SumAmountForAccount(id)
	if err != nil {
		return decimal.Zero
	}
	return sum
}

// NewAccountManageFlow builds the account-management flow: an optional
// candidate picker (skipped when Call 2 already matched the account), a
// deterministic operation menu, and one confirm gate per operation — every
// operation confirms, Cancelar aborts anywhere without touching anything.
func NewAccountManageFlow(balances balanceSummer) *conversation.Flow {
	steps := map[string]conversation.Step{
		stepAccountManagePick: conversation.ChoiceStep{
			PromptText: msgAccountManagePick,
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				labels := conversation.DecodeStringSlice(data, conversation.KeyCandidateLabels)
				opts := make([]conversation.ChoiceOption, 0, len(labels)+2)
				for i, l := range labels {
					opts = append(opts, conversation.ChoiceOption{
						Label: l, Value: "pick_" + strconv.Itoa(i), NextStep: stepAccountManageMenu,
					})
				}
				opts = append(opts,
					conversation.ChoiceOption{Label: "➕ Crear una cuenta nueva", Value: optionManageCreate, Finish: true},
					cancelOption,
				)
				return opts
			},
			DeclaredNextSteps: []string{stepAccountManageMenu},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := conversation.CopyData(data)
				switch value {
				case optionCancel:
					conversation.SetFlag(next, conversation.KeyCancelled)
				case optionManageCreate:
					next[conversation.KeyOperation] = opCreateNew
				default:
					i, err := strconv.Atoi(strings.TrimPrefix(value, "pick_"))
					ids := conversation.DecodeStringSlice(data, conversation.KeyCandidateIDs)
					names := conversation.DecodeStringSlice(data, conversation.KeyCandidateNames)
					curs := conversation.DecodeStringSlice(data, conversation.KeyCandidateCurrencies)
					if err == nil && i >= 0 && i < len(ids) {
						next[conversation.KeyAccountID] = ids[i]
						next[conversation.KeyAccountName] = names[i]
						next[conversation.KeyAccountCurrency] = curs[i]
					}
				}
				return next
			},
			InvalidChoiceMessage: msgInvalidChoice,
			SkipIf: func(data conversation.Data) (string, bool) {
				if conversation.StringOrEmpty(data[conversation.KeyAccountID]) != "" {
					return stepAccountManageMenu, true
				}
				return "", false
			},
		},
		stepAccountManageMenu: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				return msgAccountManageMenu(
					conversation.StringOrEmpty(data[conversation.KeyAccountName]),
					conversation.StringOrEmpty(data[conversation.KeyAccountCurrency]),
					accountManageBalance(balances, data),
				)
			},
			Options: []conversation.ChoiceOption{
				{Label: "✏️ Cambiar nombre", Value: optionManageRename, NextStep: stepAccountManageAskName},
				{Label: "💰 Ajustar saldo", Value: optionManageAdjust, NextStep: stepAccountManageAskTotal},
				{Label: "⭐ Hacer default", Value: optionManageDefault, NextStep: stepAccountManageConfirmDefault},
				cancelOption,
			},
			OnChoice:             onAccountManageCancel,
			InvalidChoiceMessage: msgInvalidChoice,
		},
		stepAccountManageAskName: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				return msgAskAccountNewName(conversation.StringOrEmpty(data[conversation.KeyAccountName]))
			},
			DataKey: conversation.KeyNewName,
			Validate: func(text string, _ conversation.Data) string {
				if text == "" {
					return msgInvalidAccountCreateName
				}
				return ""
			},
			NextStep: stepAccountManageConfirmRename,
			EscapeOptions: []conversation.ChoiceOption{
				{Label: "⬅️ Atrás", Value: optionBack, NextStep: stepAccountManageMenu},
				cancelOption,
			},
			OnEscape: onAccountManageCancel,
		},
		stepAccountManageConfirmRename: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				return msgConfirmAccountRename(conversation.StringOrEmpty(data[conversation.KeyAccountName]), conversation.StringOrEmpty(data[conversation.KeyNewName]))
			},
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: optionConfirm, Finish: true},
				{Label: "⬅️ Atrás", Value: optionBack, NextStep: stepAccountManageAskName},
				cancelOption,
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := onAccountManageCancel(value, data)
				if value == optionConfirm {
					next = conversation.CopyData(next)
					next[conversation.KeyOperation] = opRename
				}
				return next
			},
			InvalidChoiceMessage: msgInvalidChoice,
		},
		stepAccountManageAskTotal: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				return msgAskAccountNewTotal(conversation.StringOrEmpty(data[conversation.KeyAccountName]))
			},
			DataKey:  conversation.KeyNewTotal,
			Validate: validateBalanceAmount,
			NextStep: stepAccountManageConfirmAdjust,
			EscapeOptions: []conversation.ChoiceOption{
				{Label: "⬅️ Atrás", Value: optionBack, NextStep: stepAccountManageMenu},
				cancelOption,
			},
			OnEscape: onAccountManageCancel,
		},
		stepAccountManageConfirmAdjust: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				current := accountManageBalance(balances, data)
				// ponytail: error ignorado a propósito — stepAccountManageAskTotal ya
				// validó este valor con validateBalanceAmount, así que acá no puede fallar.
				newTotal, _ := movement.ParseARAmount(conversation.StringOrEmpty(data[conversation.KeyNewTotal]))
				return msgConfirmAccountAdjust(
					conversation.StringOrEmpty(data[conversation.KeyAccountName]),
					conversation.StringOrEmpty(data[conversation.KeyAccountCurrency]),
					current, newTotal,
				)
			},
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: optionConfirm, Finish: true},
				{Label: "⬅️ Atrás", Value: optionBack, NextStep: stepAccountManageAskTotal},
				cancelOption,
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := onAccountManageCancel(value, data)
				if value == optionConfirm {
					next = conversation.CopyData(next)
					next[conversation.KeyOperation] = opAdjust
				}
				return next
			},
			InvalidChoiceMessage: msgInvalidChoice,
		},
		stepAccountManageConfirmDefault: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				return msgConfirmAccountDefault(conversation.StringOrEmpty(data[conversation.KeyAccountName]), conversation.StringOrEmpty(data[conversation.KeyAccountCurrency]))
			},
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: optionConfirm, Finish: true},
				{Label: "⬅️ Atrás", Value: optionBack, NextStep: stepAccountManageMenu},
				cancelOption,
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := onAccountManageCancel(value, data)
				if value == optionConfirm {
					next = conversation.CopyData(next)
					next[conversation.KeyOperation] = opDefault
				}
				return next
			},
			InvalidChoiceMessage: msgInvalidChoice,
		},
	}

	flow, err := conversation.NewFlow(accountManageFlowName, stepAccountManagePick, steps)
	if err != nil {
		panic(err)
	}
	return flow
}
