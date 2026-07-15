package messaging

import (
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/conversation"
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
	next := copyData(data)
	next["cancelled"] = "true"
	return next
}

func accountManageBalance(balances balanceSummer, data conversation.Data) decimal.Decimal {
	id, err := strconv.ParseUint(stringOrEmpty(data["account_id"]), 10, 64)
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
				labels := decodeStringSlice(data, "candidate_labels")
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
				next := copyData(data)
				switch value {
				case optionCancel:
					next["cancelled"] = "true"
				case optionManageCreate:
					next["operation"] = "create_new"
				default:
					i, err := strconv.Atoi(strings.TrimPrefix(value, "pick_"))
					ids := decodeStringSlice(data, "candidate_ids")
					names := decodeStringSlice(data, "candidate_names")
					curs := decodeStringSlice(data, "candidate_currencies")
					if err == nil && i >= 0 && i < len(ids) {
						next["account_id"] = ids[i]
						next["account_name"] = names[i]
						next["account_currency"] = curs[i]
					}
				}
				return next
			},
			InvalidChoiceMessage: msgGenericFlowError,
			SkipIf: func(data conversation.Data) (string, bool) {
				if stringOrEmpty(data["account_id"]) != "" {
					return stepAccountManageMenu, true
				}
				return "", false
			},
		},
		stepAccountManageMenu: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				return msgAccountManageMenu(
					stringOrEmpty(data["account_name"]),
					stringOrEmpty(data["account_currency"]),
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
			InvalidChoiceMessage: msgGenericFlowError,
		},
		stepAccountManageAskName: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				return msgAskAccountNewName(stringOrEmpty(data["account_name"]))
			},
			DataKey: "new_name",
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
				return msgConfirmAccountRename(stringOrEmpty(data["account_name"]), stringOrEmpty(data["new_name"]))
			},
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: "confirm", Finish: true},
				{Label: "⬅️ Atrás", Value: optionBack, NextStep: stepAccountManageAskName},
				cancelOption,
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := onAccountManageCancel(value, data)
				if value == "confirm" {
					next = copyData(next)
					next["operation"] = "rename"
				}
				return next
			},
			InvalidChoiceMessage: msgGenericFlowError,
		},
		stepAccountManageAskTotal: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				return msgAskAccountNewTotal(stringOrEmpty(data["account_name"]))
			},
			DataKey:  "new_total",
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
				newTotal, _ := decimal.NewFromString(stringOrEmpty(data["new_total"]))
				return msgConfirmAccountAdjust(
					stringOrEmpty(data["account_name"]),
					stringOrEmpty(data["account_currency"]),
					current, newTotal,
				)
			},
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: "confirm", Finish: true},
				{Label: "⬅️ Atrás", Value: optionBack, NextStep: stepAccountManageAskTotal},
				cancelOption,
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := onAccountManageCancel(value, data)
				if value == "confirm" {
					next = copyData(next)
					next["operation"] = "adjust"
				}
				return next
			},
			InvalidChoiceMessage: msgGenericFlowError,
		},
		stepAccountManageConfirmDefault: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				return msgConfirmAccountDefault(stringOrEmpty(data["account_name"]), stringOrEmpty(data["account_currency"]))
			},
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: "confirm", Finish: true},
				{Label: "⬅️ Atrás", Value: optionBack, NextStep: stepAccountManageMenu},
				cancelOption,
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := onAccountManageCancel(value, data)
				if value == "confirm" {
					next = copyData(next)
					next["operation"] = "default"
				}
				return next
			},
			InvalidChoiceMessage: msgGenericFlowError,
		},
	}

	flow, err := conversation.NewFlow(accountManageFlowName, stepAccountManagePick, steps)
	if err != nil {
		panic(err)
	}
	return flow
}
