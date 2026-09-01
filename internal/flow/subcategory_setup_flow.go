package flow

import (
	"strings"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/subcategory"
)

const (
	StepChooseMode             = "subcategory_setup_choose_mode"
	StepPickExistingCategory   = "subcategory_setup_pick_category"
	StepNewCategoryName        = "subcategory_setup_new_category_name"
	StepNewCategoryIcon        = "subcategory_setup_new_category_icon"
	StepSubcategoryName        = "subcategory_setup_subcategory_name"
	StepSubcategoryDescription = "subcategory_setup_description"
	StepConfirmSubcategory     = "subcategory_setup_confirm"

	OptionNewCategory = "new_category"
)

func NewSubcategorySetupFlow(subcategories subcategoryRepository) *conversation.Flow {
	backOption := func(to string) conversation.ChoiceOption {
		return conversation.ChoiceOption{Label: "⬅️ Atrás", Value: OptionBack, NextStep: to}
	}

	steps := map[string]conversation.Step{
		StepChooseMode: conversation.ChoiceStep{
			PromptText: func(conversation.Data) string { return subcategory.MsgChooseCategoryIntro },
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				cats, _ := subcategories.DistinctCategoriesForUser(data.UserID())
				opts := make([]conversation.ChoiceOption, 0, len(cats)+2)
				if len(cats) > 0 {
					opts = append(opts, conversation.ChoiceOption{Label: "📂 Elegir categoría existente", Value: "existing", NextStep: StepPickExistingCategory})
				}
				opts = append(opts,
					conversation.ChoiceOption{Label: subcategory.BtnNewCategory, Value: OptionNewCategory, NextStep: StepNewCategoryName},
					CancelOption,
				)
				return opts
			},
			DeclaredNextSteps: []string{StepPickExistingCategory, StepNewCategoryName},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == OptionCancel {
					return OnAccountCreateEscape(OptionCancel, data)
				}
				next := conversation.CopyData(data)
				if value == OptionNewCategory {
					conversation.SetFlag(next, conversation.KeyCategoryIsNew)
				}
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},

		StepPickExistingCategory: conversation.ChoiceStep{
			PromptText: func(conversation.Data) string { return subcategory.MsgChooseCategoryIntro },
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				return CategoryOptions(subcategories, data, StepSubcategoryName,
					conversation.ChoiceOption{Label: subcategory.BtnNewCategory, Value: OptionNewCategory, NextStep: StepNewCategoryName},
					backOption(StepChooseMode),
					CancelOption,
				)
			},
			DeclaredNextSteps: []string{StepSubcategoryName, StepNewCategoryName, StepChooseMode},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == OptionCancel || value == OptionBack {
					return OnAccountCreateEscape(value, data)
				}
				next := conversation.CopyData(data)
				if value == OptionNewCategory {
					conversation.SetFlag(next, conversation.KeyCategoryIsNew)
					return next
				}
				next["category"] = value
				next[conversation.KeyCategoryIsNew] = "false"
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},

		StepNewCategoryName: conversation.TextStep{
			PromptText: func(conversation.Data) string { return subcategory.MsgAskNewCategoryName() },
			DataKey:    conversation.KeyCategory,
			Validate: func(text string, _ conversation.Data) string {
				if strings.TrimSpace(text) == "" || subcategory.IsReserved(text) {
					return subcategory.MsgInvalidCategoryName
				}
				return ""
			},
			NextStep:      StepNewCategoryIcon,
			EscapeOptions: []conversation.ChoiceOption{backOption(StepChooseMode), CancelOption},
			EscapeOptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				v := conversation.StringOrEmpty(data["category"])
				if v == "" {
					return nil
				}
				return []conversation.ChoiceOption{{Label: "✅ Usar " + v, Value: OptionConfirmSeed, NextStep: StepNewCategoryIcon}}
			},
			OnEscape: OnAccountCreateEscape,
		},

		StepNewCategoryIcon: conversation.TextStep{
			PromptText: func(conversation.Data) string { return "¿Qué emoji querés usar para esta categoría?" },
			DataKey:    conversation.KeyCategoryIcon,
			Validate: func(text string, _ conversation.Data) string {
				if !subcategory.ValidIcon(strings.TrimSpace(text)) {
					return "Mandame un solo emoji para representar la categoría."
				}
				return ""
			},
			NextStep:      StepSubcategoryName,
			EscapeOptions: []conversation.ChoiceOption{backOption(StepNewCategoryName), CancelOption},
			EscapeOptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				v := conversation.StringOrEmpty(data[conversation.KeyCategoryIcon])
				if v == "" {
					return nil
				}
				return []conversation.ChoiceOption{{Label: "✅ Usar " + v, Value: OptionConfirmSeed, NextStep: StepSubcategoryName}}
			},
			OnEscape: OnAccountCreateEscape,
		},

		StepSubcategoryName: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				return subcategory.MsgAskSubcategoryName(conversation.StringOrEmpty(data["category"]))
			},
			DataKey: conversation.KeySubcategory,
			Validate: func(text string, data conversation.Data) string {
				text = strings.TrimSpace(text)
				if text == "" {
					return subcategory.MsgInvalidSubcategoryName
				}
				if _, err := subcategories.FindByCategoryAndSubcategory(data.UserID(), conversation.StringOrEmpty(data["category"]), text); err == nil {
					return subcategory.MsgSubcategoryAlreadyExists(conversation.StringOrEmpty(data["category"]), text)
				}
				return ""
			},
			NextStep:      StepSubcategoryDescription,
			EscapeOptions: []conversation.ChoiceOption{CancelOption},
			EscapeOptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				v := conversation.StringOrEmpty(data["subcategory"])
				if v == "" {
					return nil
				}
				return []conversation.ChoiceOption{{Label: "✅ Usar " + v, Value: OptionConfirmSeed, NextStep: StepSubcategoryDescription}}
			},
			OnEscape: OnAccountCreateEscape,
		},

		StepSubcategoryDescription: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				return MsgAskSubcategoryDescription(conversation.StringOrEmpty(data["subcategory"]))
			},
			DataKey: conversation.KeySubcategoryDescription,
			Validate: func(text string, _ conversation.Data) string {
				if strings.TrimSpace(text) == "" {
					return "Contame en pocas palabras cuándo se usa esta subcategoría."
				}
				return ""
			},
			NextStep:      StepConfirmSubcategory,
			EscapeOptions: []conversation.ChoiceOption{backOption(StepSubcategoryName), CancelOption},
			EscapeOptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				if conversation.StringOrEmpty(data[conversation.KeySubcategoryDescription]) == "" {
					return nil
				}
				return []conversation.ChoiceOption{{Label: "✅ Usar la propuesta", Value: OptionConfirmSeed, NextStep: StepConfirmSubcategory}}
			},
			OnEscape: OnAccountCreateEscape,
		},

		StepConfirmSubcategory: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				icon := conversation.StringOrEmpty(data[conversation.KeyCategoryIcon])
				if icon == "" {
					icon = subcategories.IconForCategory(data.UserID(), conversation.StringOrEmpty(data["category"]))
				}
				return icon + " " + conversation.StringOrEmpty(data["category"]) + " › " + conversation.StringOrEmpty(data["subcategory"]) +
					"\n📝 " + conversation.StringOrEmpty(data[conversation.KeySubcategoryDescription]) + "\n\n¿Confirmás?"
			},
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: OptionConfirm, Finish: true},
				backOption(StepSubcategoryDescription),
				CancelOption,
			},
			OnChoice:             OnAccountCreateEscape,
			InvalidChoiceMessage: MsgInvalidChoice,
		},
	}

	flow, err := conversation.NewFlow(SubcategorySetupFlowName, StepChooseMode, steps)
	if err != nil {
		panic(err)
	}
	return flow
}
