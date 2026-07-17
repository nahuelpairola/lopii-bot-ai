package messaging

import (
	"strings"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/subcategory"
)

const (
	subcategorySetupFlowName = "subcategory_setup"

	stepChooseMode             = "subcategory_setup_choose_mode"
	stepPickExistingCategory   = "subcategory_setup_pick_category"
	stepNewCategoryName        = "subcategory_setup_new_category_name"
	stepNewCategoryIcon        = "subcategory_setup_new_category_icon"
	stepSubcategoryName        = "subcategory_setup_subcategory_name"
	stepSubcategoryDescription = "subcategory_setup_description"
	stepConfirmSubcategory     = "subcategory_setup_confirm"

	optionNewCategory = "new_category"
)

// NewSubcategorySetupFlow builds the 7-step flow for creating a custom
// subcategory (in an existing category or a brand-new one), including a
// short description step that feeds orchestrator.TaxonomyEntry.Description
// for future CREATE classification. Started by startSubcategorySetup
// (free_text.go) whenever Call 1 classifies a message as CREATE_CATEGORY.
// Every step is cancelable via TextStep.EscapeOptions/ChoiceOption's
// cancelOption (the same mechanism account_create_flow.go already uses)
// and back-able except the entry point stepChooseMode.
func NewSubcategorySetupFlow(subcategories subcategoryRepository) *conversation.Flow {
	backOption := func(to string) conversation.ChoiceOption {
		return conversation.ChoiceOption{Label: "⬅️ Atrás", Value: optionBack, NextStep: to}
	}

	steps := map[string]conversation.Step{
		stepChooseMode: conversation.ChoiceStep{
			PromptText: func(conversation.Data) string { return subcategory.MsgChooseCategoryIntro },
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				cats, _ := subcategories.DistinctCategoriesForUser(data.UserID())
				opts := make([]conversation.ChoiceOption, 0, len(cats)+2)
				if len(cats) > 0 {
					opts = append(opts, conversation.ChoiceOption{Label: "📂 Elegir categoría existente", Value: "existing", NextStep: stepPickExistingCategory})
				}
				opts = append(opts,
					conversation.ChoiceOption{Label: subcategory.BtnNewCategory, Value: optionNewCategory, NextStep: stepNewCategoryName},
					cancelOption,
				)
				return opts
			},
			DeclaredNextSteps: []string{stepPickExistingCategory, stepNewCategoryName},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == optionCancel {
					return onAccountCreateEscape(optionCancel, data)
				}
				next := copyData(data)
				if value == optionNewCategory {
					setFlag(next, keyCategoryIsNew)
				}
				return next
			},
			InvalidChoiceMessage: msgGenericFlowError,
		},

		stepPickExistingCategory: conversation.ChoiceStep{
			PromptText: func(conversation.Data) string { return subcategory.MsgChooseCategoryIntro },
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				cats, _ := subcategories.DistinctCategoriesForUser(data.UserID())
				opts := make([]conversation.ChoiceOption, 0, len(cats)+2)
				for _, cat := range cats {
					opts = append(opts, conversation.ChoiceOption{
						Label:    subcategories.IconForCategory(data.UserID(), cat) + " " + cat,
						Value:    cat,
						NextStep: stepSubcategoryName,
					})
				}
				opts = append(opts,
					conversation.ChoiceOption{Label: subcategory.BtnNewCategory, Value: optionNewCategory, NextStep: stepNewCategoryName},
					backOption(stepChooseMode),
					cancelOption,
				)
				return opts
			},
			DeclaredNextSteps: []string{stepSubcategoryName, stepNewCategoryName, stepChooseMode},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == optionCancel || value == optionBack {
					return onAccountCreateEscape(value, data)
				}
				next := copyData(data)
				if value == optionNewCategory {
					setFlag(next, keyCategoryIsNew)
					return next
				}
				next["category"] = value
				next[keyCategoryIsNew] = "false"
				return next
			},
			InvalidChoiceMessage: msgGenericFlowError,
		},

		stepNewCategoryName: conversation.TextStep{
			PromptText: func(conversation.Data) string { return subcategory.MsgAskNewCategoryName() },
			DataKey:    keyCategory,
			Validate: func(text string, _ conversation.Data) string {
				if strings.TrimSpace(text) == "" || subcategory.IsReserved(text) {
					return subcategory.MsgInvalidCategoryName
				}
				return ""
			},
			NextStep:      stepNewCategoryIcon,
			EscapeOptions: []conversation.ChoiceOption{backOption(stepChooseMode), cancelOption},
			EscapeOptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				v := stringOrEmpty(data["category"])
				if v == "" {
					return nil
				}
				return []conversation.ChoiceOption{{Label: "✅ Usar " + v, Value: optionConfirmSeed, NextStep: stepNewCategoryIcon}}
			},
			OnEscape: onAccountCreateEscape,
		},

		// stepNewCategoryIcon is only ever reached via stepNewCategoryName's
		// NextStep — the existing-category path in stepPickExistingCategory
		// targets stepSubcategoryName directly, so this step is "skipped"
		// by construction (two different NextStep targets) rather than via
		// a runtime Skip/SkipIf check.
		stepNewCategoryIcon: conversation.TextStep{
			PromptText: func(conversation.Data) string { return "¿Qué emoji querés usar para esta categoría?" },
			DataKey:    keyCategoryIcon,
			Validate: func(text string, _ conversation.Data) string {
				if !subcategory.ValidIcon(strings.TrimSpace(text)) {
					return "Mandame un solo emoji para representar la categoría."
				}
				return ""
			},
			NextStep:      stepSubcategoryName,
			EscapeOptions: []conversation.ChoiceOption{backOption(stepNewCategoryName), cancelOption},
			EscapeOptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				v := stringOrEmpty(data[keyCategoryIcon])
				if v == "" {
					return nil
				}
				return []conversation.ChoiceOption{{Label: "✅ Usar " + v, Value: optionConfirmSeed, NextStep: stepSubcategoryName}}
			},
			OnEscape: onAccountCreateEscape,
		},

		stepSubcategoryName: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				return subcategory.MsgAskSubcategoryName(stringOrEmpty(data["category"]))
			},
			DataKey: keySubcategory,
			Validate: func(text string, data conversation.Data) string {
				text = strings.TrimSpace(text)
				if text == "" {
					return subcategory.MsgInvalidSubcategoryName
				}
				if _, err := subcategories.FindByCategoryAndSubcategory(data.UserID(), stringOrEmpty(data["category"]), text); err == nil {
					return subcategory.MsgSubcategoryAlreadyExists(stringOrEmpty(data["category"]), text)
				}
				return ""
			},
			NextStep:      stepSubcategoryDescription,
			EscapeOptions: []conversation.ChoiceOption{cancelOption},
			EscapeOptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				v := stringOrEmpty(data["subcategory"])
				if v == "" {
					return nil
				}
				return []conversation.ChoiceOption{{Label: "✅ Usar " + v, Value: optionConfirmSeed, NextStep: stepSubcategoryDescription}}
			},
			OnEscape: onAccountCreateEscape,
		},

		// stepSubcategoryDescription asks the one thing the pre-existing
		// scaffolding (msgAskNewCategoryName et al.) never covered: a short
		// description of *when* this subcategory applies. This is not
		// decorative — orchestrator.TaxonomyEntry.Description is fed to Call
		// 2 CREATE as the classification hint (see the seeded taxonomy's own
		// disambiguation notes, e.g. "NO incluye compras específicas como
		// carnicería"); a user-created subcategory with no description is
		// indistinguishable from PENDING_REVIEW to the LLM the next time a
		// movement should land here.
		stepSubcategoryDescription: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				return msgAskSubcategoryDescription(stringOrEmpty(data["subcategory"]))
			},
			DataKey: keySubcategoryDescription,
			Validate: func(text string, _ conversation.Data) string {
				if strings.TrimSpace(text) == "" {
					return "Contame en pocas palabras cuándo se usa esta subcategoría."
				}
				return ""
			},
			NextStep:      stepConfirmSubcategory,
			EscapeOptions: []conversation.ChoiceOption{backOption(stepSubcategoryName), cancelOption},
			EscapeOptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				if stringOrEmpty(data[keySubcategoryDescription]) == "" {
					return nil
				}
				return []conversation.ChoiceOption{{Label: "✅ Usar la propuesta", Value: optionConfirmSeed, NextStep: stepConfirmSubcategory}}
			},
			OnEscape: onAccountCreateEscape,
		},

		stepConfirmSubcategory: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				icon := stringOrEmpty(data[keyCategoryIcon])
				if icon == "" {
					icon = subcategories.IconForCategory(data.UserID(), stringOrEmpty(data["category"]))
				}
				return icon + " " + stringOrEmpty(data["category"]) + " › " + stringOrEmpty(data["subcategory"]) +
					"\n📝 " + stringOrEmpty(data[keySubcategoryDescription]) + "\n\n¿Confirmás?"
			},
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: optionConfirm, Finish: true},
				backOption(stepSubcategoryDescription),
				cancelOption,
			},
			OnChoice:             onAccountCreateEscape,
			InvalidChoiceMessage: msgGenericFlowError,
		},
	}

	flow, err := conversation.NewFlow(subcategorySetupFlowName, stepChooseMode, steps)
	if err != nil {
		panic(err)
	}
	return flow
}
