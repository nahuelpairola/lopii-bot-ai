package flow

import (
	"strconv"
	"strings"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
)

const (
	stepCreateFirstAccount  = "create_first_account"
	stepFirstAccountBalance = "first_account_balance"
	stepResolveCategory     = "resolve_category"
	stepResolveSubcategory  = "resolve_subcategory"
	stepResolveAccount      = "resolve_account"
)

func NewMovementCreateFlow(subcategories subcategoryRepository, accounts accountRepository) *conversation.Flow {
	steps := map[string]conversation.Step{
		stepCreateFirstAccount: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				return MsgAskFirstAccountName(FirstAccountCurrency(data, HasDefaultFor(accounts, data)))
			},
			DataKey: conversation.KeyFirstAccountName,
			SkipIf: func(data conversation.Data) (string, bool) {
				if NeedsFirstAccount(data, HasDefaultFor(accounts, data)) {
					return "", false
				}
				return stepResolveCategory, true
			},
			Validate: func(text string, _ conversation.Data) string {
				if strings.TrimSpace(text) == "" {
					return MsgInvalidAccountCreateName
				}
				return ""
			},
			NextStep:      stepFirstAccountBalance,
			EscapeOptions: []conversation.ChoiceOption{CancelOption},
			OnEscape: func(value string, data conversation.Data) conversation.Data {
				if value != OptionCancel {
					return data
				}
				next := conversation.CopyData(data)
				conversation.SetFlag(next, conversation.KeyCancelled)
				return next
			},
		},
		stepFirstAccountBalance: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				return MsgAskFirstAccountBalance(
					conversation.StringOrEmpty(data[conversation.KeyFirstAccountName]),
					FirstAccountCurrency(data, HasDefaultFor(accounts, data)),
				)
			},
			DataKey: conversation.KeyFirstAccountBalance,
			SkipIf: func(data conversation.Data) (string, bool) {
				if conversation.StringOrEmpty(data[conversation.KeyFirstAccountName]) == "" {
					return stepResolveCategory, true
				}
				return "", false
			},
			Validate: func(text string, _ conversation.Data) string {
				if _, err := movement.ParseARAmount(text); err != nil {
					return account.MsgInvalidAmount
				}
				return ""
			},
			NextStep: stepResolveCategory,
			EscapeOptions: []conversation.ChoiceOption{
				{Label: "⬅️ Atrás", Value: OptionBack, NextStep: stepCreateFirstAccount},
				{Label: "⏭️ Después", Value: OptionBalanceLater, NextStep: stepResolveCategory},
				CancelOption,
			},
			OnEscape: func(value string, data conversation.Data) conversation.Data {
				next := conversation.CopyData(data)
				if value == OptionCancel {
					conversation.SetFlag(next, conversation.KeyCancelled)
				}
				return next
			},
		},
		stepResolveCategory: conversation.ChoiceStep{
			PromptText: MsgAskCategory,
			SkipIf: func(data conversation.Data) (string, bool) {
				if len(conversation.DecodeStringSlice(data, conversation.KeyPendingCategoryGaps)) == 0 {
					return stepResolveAccount, true
				}
				if conversation.StringOrEmpty(data[conversation.KeyMode]) == ModeUpdate &&
					rowCategoryExists(subcategories, data) {
					return stepResolveSubcategory, true
				}
				return "", false
			},
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				cats, _ := subcategories.DistinctCategoriesForUser(data.UserID())
				opts := make([]conversation.ChoiceOption, 0, len(cats))
				for _, cat := range cats {
					opts = append(opts, conversation.ChoiceOption{
						Label:    subcategories.IconForCategory(data.UserID(), cat) + " " + cat,
						Value:    cat,
						NextStep: stepResolveSubcategory,
					})
				}
				opts = append(opts, CancelOption)
				return opts
			},
			DeclaredNextSteps: []string{stepResolveSubcategory},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == OptionCancel {
					next := conversation.CopyData(data)
					conversation.SetFlag(next, conversation.KeyCancelled)
					return next
				}
				gaps := conversation.DecodeStringSlice(data, conversation.KeyPendingCategoryGaps)
				if len(gaps) == 0 {
					return data
				}
				next := conversation.CopyData(data)
				next[conversation.KeyGapActiveRow] = gaps[0]
				rows := movement.DecodeMovementRows(data)
				idx, _ := strconv.Atoi(gaps[0])
				rows[idx].Category = value
				next[conversation.KeyMovements] = movement.EncodeMovementRows(rows)
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},
		stepResolveSubcategory: conversation.ChoiceStep{
			PromptText: MsgAskSubcategory,
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				rowIdx := ActiveGapRow(data)
				rows := movement.DecodeMovementRows(data)
				category := rows[rowIdx].Category

				subs, _ := subcategories.FindAllForUser(data.UserID())
				var opts []conversation.ChoiceOption
				for _, s := range subs {
					if s.Category != category {
						continue
					}
					opts = append(opts, conversation.ChoiceOption{
						Label:    s.Subcategory,
						Value:    s.Subcategory,
						NextStep: stepResolveCategory,
					})
				}
				opts = append(opts, CancelOption)
				return opts
			},
			DeclaredNextSteps: []string{stepResolveCategory},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == OptionCancel {
					next := conversation.CopyData(data)
					conversation.SetFlag(next, conversation.KeyCancelled)
					return next
				}
				next := conversation.CopyData(data)
				gaps := conversation.DecodeStringSlice(data, conversation.KeyPendingCategoryGaps)
				rowIdx := ActiveGapRow(data)

				rows := movement.DecodeMovementRows(data)
				rows[rowIdx].Subcategory = value
				next[conversation.KeyMovements] = movement.EncodeMovementRows(rows)
				if len(gaps) > 0 {
					next[conversation.KeyPendingCategoryGaps] = conversation.EncodeStringSlice(gaps[1:])
				}
				next[conversation.KeyGapActiveRow] = ""
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},
		stepResolveAccount: conversation.ChoiceStep{
			PromptText: MsgAskAccount,
			SkipIf: func(data conversation.Data) (string, bool) {
				if len(conversation.DecodeStringSlice(data, conversation.KeyPendingAccountGaps)) == 0 {
					return "", true
				}
				return "", false
			},
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				gaps := conversation.DecodeStringSlice(data, conversation.KeyPendingAccountGaps)
				if len(gaps) == 0 {
					return nil
				}
				rowIdx, _ := strconv.Atoi(gaps[0])
				rows := movement.DecodeMovementRows(data)

				accs, _ := accounts.FindByUserID(data.UserID())
				var opts []conversation.ChoiceOption
				for _, a := range accs {
					if a.Currency.String() != rows[rowIdx].Currency {
						continue
					}
					opts = append(opts, conversation.ChoiceOption{
						Label:    a.Name,
						Value:    AccountChoiceExistingPrefix + strconv.FormatUint(uint64(a.ID), 10),
						NextStep: stepResolveAccount,
					})
				}
				opts = append(opts, conversation.ChoiceOption{
					Label:    "➕ Crear cuenta \"" + rows[rowIdx].AccountNameGuess + "\"",
					Value:    OptionAccountCreate,
					NextStep: stepResolveAccount,
				})
				opts = append(opts, CancelOption)
				return opts
			},
			DeclaredNextSteps: []string{stepResolveAccount},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == OptionCancel {
					next := conversation.CopyData(data)
					conversation.SetFlag(next, conversation.KeyCancelled)
					return next
				}
				next := conversation.CopyData(data)
				gaps := conversation.DecodeStringSlice(data, conversation.KeyPendingAccountGaps)
				if len(gaps) == 0 {
					return next
				}
				rowIdx, _ := strconv.Atoi(gaps[0])

				rows := movement.DecodeMovementRows(data)
				if value == OptionAccountCreate {
					rows[rowIdx].AccountID = AccountPendingCreate
				} else {
					rows[rowIdx].AccountID = strings.TrimPrefix(value, AccountChoiceExistingPrefix)
				}
				next[conversation.KeyMovements] = movement.EncodeMovementRows(rows)
				next[conversation.KeyPendingAccountGaps] = conversation.EncodeStringSlice(gaps[1:])
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},
	}

	flow, err := conversation.NewFlow(MovementCreateFlowName, stepCreateFirstAccount, steps)
	if err != nil {
		panic(err)
	}
	return flow
}
