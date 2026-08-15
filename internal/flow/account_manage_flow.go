package flow

import (
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
)

const (
	StepAccountManagePick           = "account_manage_pick"
	StepAccountManageMenu           = "account_manage_menu"
	StepAccountManageAskName        = "account_manage_ask_name"
	StepAccountManageConfirmRename  = "account_manage_confirm_rename"
	StepAccountManageAskTotal       = "account_manage_ask_total"
	StepAccountManageConfirmAdjust  = "account_manage_confirm_adjust"
	StepAccountManageConfirmDefault = "account_manage_confirm_default"

	OptionManageRename  = "op_rename"
	OptionManageAdjust  = "op_adjust"
	OptionManageDefault = "op_default"
	OptionManageCreate  = "op_create_new"

	// operation values stored under conversation.KeyOperation and switched on in
	// account_manage_finish.go — distinct from the optionManage* button values.
	OpRename    = "rename"
	OpAdjust    = "adjust"
	OpDefault   = "default"
	OpCreateNew = "create_new"
)

// OnAccountManageCancel marks the flow cancelled (finishAccountManageFlow
// skips every write) — same contract as OnAccountCreateEscape.
func OnAccountManageCancel(value string, data conversation.Data) conversation.Data {
	if value != OptionCancel {
		return data
	}
	next := conversation.CopyData(data)
	conversation.SetFlag(next, conversation.KeyCancelled)
	return next
}

func AccountManageBalance(balances balanceSummer, data conversation.Data) decimal.Decimal {
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
		StepAccountManagePick: conversation.ChoiceStep{
			PromptText: MsgAccountManagePick,
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				labels := conversation.DecodeStringSlice(data, conversation.KeyCandidateLabels)
				opts := make([]conversation.ChoiceOption, 0, len(labels)+2)
				for i, l := range labels {
					opts = append(opts, conversation.ChoiceOption{
						Label: l, Value: "pick_" + strconv.Itoa(i), NextStep: StepAccountManageMenu,
					})
				}
				opts = append(opts,
					conversation.ChoiceOption{Label: "➕ Crear una cuenta nueva", Value: OptionManageCreate, Finish: true},
					CancelOption,
				)
				return opts
			},
			DeclaredNextSteps: []string{StepAccountManageMenu},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := conversation.CopyData(data)
				switch value {
				case OptionCancel:
					conversation.SetFlag(next, conversation.KeyCancelled)
				case OptionManageCreate:
					next[conversation.KeyOperation] = OpCreateNew
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
			InvalidChoiceMessage: MsgInvalidChoice,
			SkipIf: func(data conversation.Data) (string, bool) {
				if conversation.StringOrEmpty(data[conversation.KeyAccountID]) != "" {
					return StepAccountManageMenu, true
				}
				return "", false
			},
		},
		StepAccountManageMenu: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				return MsgAccountManageMenu(
					conversation.StringOrEmpty(data[conversation.KeyAccountName]),
					conversation.StringOrEmpty(data[conversation.KeyAccountCurrency]),
					AccountManageBalance(balances, data),
				)
			},
			Options: []conversation.ChoiceOption{
				{Label: "✏️ Cambiar nombre", Value: OptionManageRename, NextStep: StepAccountManageAskName},
				{Label: "💰 Ajustar saldo", Value: OptionManageAdjust, NextStep: StepAccountManageAskTotal},
				{Label: "⭐ Hacer default", Value: OptionManageDefault, NextStep: StepAccountManageConfirmDefault},
				CancelOption,
			},
			OnChoice:             OnAccountManageCancel,
			InvalidChoiceMessage: MsgInvalidChoice,
		},
		StepAccountManageAskName: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				return MsgAskAccountNewName(conversation.StringOrEmpty(data[conversation.KeyAccountName]))
			},
			DataKey: conversation.KeyNewName,
			Validate: func(text string, _ conversation.Data) string {
				if text == "" {
					return MsgInvalidAccountCreateName
				}
				return ""
			},
			NextStep: StepAccountManageConfirmRename,
			EscapeOptions: []conversation.ChoiceOption{
				{Label: "⬅️ Atrás", Value: OptionBack, NextStep: StepAccountManageMenu},
				CancelOption,
			},
			OnEscape: OnAccountManageCancel,
		},
		StepAccountManageConfirmRename: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				return MsgConfirmAccountRename(conversation.StringOrEmpty(data[conversation.KeyAccountName]), conversation.StringOrEmpty(data[conversation.KeyNewName]))
			},
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: OptionConfirm, Finish: true},
				{Label: "⬅️ Atrás", Value: OptionBack, NextStep: StepAccountManageAskName},
				CancelOption,
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := OnAccountManageCancel(value, data)
				if value == OptionConfirm {
					next = conversation.CopyData(next)
					next[conversation.KeyOperation] = OpRename
				}
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},
		StepAccountManageAskTotal: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				return MsgAskAccountNewTotal(conversation.StringOrEmpty(data[conversation.KeyAccountName]))
			},
			DataKey:  conversation.KeyNewTotal,
			Validate: ValidateBalanceAmount,
			NextStep: StepAccountManageConfirmAdjust,
			EscapeOptions: []conversation.ChoiceOption{
				{Label: "⬅️ Atrás", Value: OptionBack, NextStep: StepAccountManageMenu},
				CancelOption,
			},
			OnEscape: OnAccountManageCancel,
		},
		StepAccountManageConfirmAdjust: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				current := AccountManageBalance(balances, data)
				// ponytail: error ignorado a propósito — StepAccountManageAskTotal ya
				// validó este valor con ValidateBalanceAmount, así que acá no puede fallar.
				newTotal, _ := movement.ParseARAmount(conversation.StringOrEmpty(data[conversation.KeyNewTotal]))
				return MsgConfirmAccountAdjust(
					conversation.StringOrEmpty(data[conversation.KeyAccountName]),
					conversation.StringOrEmpty(data[conversation.KeyAccountCurrency]),
					current, newTotal,
				)
			},
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: OptionConfirm, Finish: true},
				{Label: "⬅️ Atrás", Value: OptionBack, NextStep: StepAccountManageAskTotal},
				CancelOption,
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := OnAccountManageCancel(value, data)
				if value == OptionConfirm {
					next = conversation.CopyData(next)
					next[conversation.KeyOperation] = OpAdjust
				}
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},
		StepAccountManageConfirmDefault: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				return MsgConfirmAccountDefault(conversation.StringOrEmpty(data[conversation.KeyAccountName]), conversation.StringOrEmpty(data[conversation.KeyAccountCurrency]))
			},
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: OptionConfirm, Finish: true},
				{Label: "⬅️ Atrás", Value: OptionBack, NextStep: StepAccountManageMenu},
				CancelOption,
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := OnAccountManageCancel(value, data)
				if value == OptionConfirm {
					next = conversation.CopyData(next)
					next[conversation.KeyOperation] = OpDefault
				}
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},
	}

	flow, err := conversation.NewFlow(AccountManageFlowName, StepAccountManagePick, steps)
	if err != nil {
		panic(err)
	}
	return flow
}
