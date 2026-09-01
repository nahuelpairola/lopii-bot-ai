package flow

import (
	"strconv"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/subcategory"
)

const (
	StepPickSource            = "category_manage_pick_source"
	StepSuggestTarget         = "category_manage_suggest_target"
	StepPickTargetCategory    = "category_manage_pick_target_category"
	StepPickTargetSubcategory = "category_manage_pick_target_subcategory"
	StepConfirmCategoryManage = "category_manage_confirm"

	OptionAcceptSuggestion = "accept_suggestion"
	OptionChooseOther      = "choose_other"

	TargetOriginSuggested = "suggested"
	TargetOriginManual    = "manual"

	DefaultCategoryIcon = "📂"

	TopDescriptionsForSuggestion = 5
)

func OnCategoryManageCancel(value string, data conversation.Data) conversation.Data {
	if value != OptionCancel {
		return data
	}
	next := conversation.CopyData(data)
	conversation.SetFlag(next, conversation.KeyCancelled)
	return next
}

func ClearTarget(data conversation.Data) conversation.Data {
	next := ClearTargetSubcategory(data)
	delete(next, conversation.KeyTargetCategory)
	return next
}

func ClearTargetSubcategory(data conversation.Data) conversation.Data {
	next := conversation.CopyData(data)
	delete(next, conversation.KeyTargetSubcategoryID)
	delete(next, conversation.KeyTargetSubcategory)
	delete(next, conversation.KeyTargetOrigin)
	return next
}

func SubcategoryIcon(s subcategory.Subcategory) string {
	if s.Icon == "" {
		return DefaultCategoryIcon
	}
	return s.Icon
}

func SourceLabel(data conversation.Data) string {
	return conversation.StringOrEmpty(data[conversation.KeySourceCategory]) + " › " + conversation.StringOrEmpty(data[conversation.KeySourceSubcategory])
}

func SuggestionLabel(data conversation.Data) string {
	return conversation.StringOrEmpty(data[conversation.KeySuggestedCategory]) + " › " + conversation.StringOrEmpty(data[conversation.KeySuggestedSubcategory])
}

func TargetLabel(data conversation.Data) string {
	return conversation.StringOrEmpty(data[conversation.KeyTargetCategory]) + " › " + conversation.StringOrEmpty(data[conversation.KeyTargetSubcategory])
}

func NewCategoryManagePickFlow(subs ownedSubcategoryLister) *conversation.Flow {
	steps := map[string]conversation.Step{
		StepPickSource: conversation.ChoiceStep{
			PromptText: func(conversation.Data) string { return MsgCategoryManagePickSource },
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				owned, _ := subs.FindOwnedByUser(data.UserID())
				opts := make([]conversation.ChoiceOption, 0, len(owned)+1)
				for _, s := range owned {
					opts = append(opts, conversation.ChoiceOption{
						Label:  CategoryLabel(SubcategoryIcon(s), s.Category, s.Subcategory),
						Value:  strconv.FormatUint(uint64(s.ID), 10),
						Finish: true,
					})
				}
				return append(opts, CancelOption)
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == OptionCancel {
					return OnCategoryManageCancel(OptionCancel, data)
				}
				owned, _ := subs.FindOwnedByUser(data.UserID())
				for _, s := range owned {
					if strconv.FormatUint(uint64(s.ID), 10) == value {
						next := conversation.CopyData(data)
						next[conversation.KeySourceSubcategoryID] = value
						next[conversation.KeySourceCategory] = s.Category
						next[conversation.KeySourceSubcategory] = s.Subcategory
						return next
					}
				}
				return data
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},
	}

	flow, err := conversation.NewFlow(CategoryManagePickFlowName, StepPickSource, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

func NewCategoryManageTargetFlow(subs targetSubcategoryLister) *conversation.Flow {
	hasSuggestion := func(data conversation.Data) bool {
		return conversation.StringOrEmpty(data[conversation.KeySuggestedSubcategory]) != ""
	}
	isEmpty := func(data conversation.Data) bool {
		return conversation.StringOrEmpty(data[conversation.KeyMovementCount]) == "0"
	}

	steps := map[string]conversation.Step{
		StepSuggestTarget: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				return MsgCategoryManageSuggest(SourceLabel(data), conversation.StringOrEmpty(data[conversation.KeyMovementCount]), SuggestionLabel(data))
			},
			SkipIf: func(data conversation.Data) (string, bool) {
				if isEmpty(data) {
					return StepConfirmCategoryManage, true
				}
				if !hasSuggestion(data) {
					return StepPickTargetCategory, true
				}
				return "", false
			},
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				return []conversation.ChoiceOption{
					{Label: "✅ Sí, a «" + SuggestionLabel(data) + "»", Value: OptionAcceptSuggestion, NextStep: StepConfirmCategoryManage},
					{Label: "🔍 Elegir otra", Value: OptionChooseOther, NextStep: StepPickTargetCategory},
					CancelOption,
				}
			},
			DeclaredNextSteps: []string{StepConfirmCategoryManage, StepPickTargetCategory},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				switch value {
				case OptionAcceptSuggestion:
					next := conversation.CopyData(data)
					next[conversation.KeyTargetSubcategoryID] = conversation.StringOrEmpty(data[conversation.KeySuggestedSubcategoryID])
					next[conversation.KeyTargetCategory] = conversation.StringOrEmpty(data[conversation.KeySuggestedCategory])
					next[conversation.KeyTargetSubcategory] = conversation.StringOrEmpty(data[conversation.KeySuggestedSubcategory])
					next[conversation.KeyTargetOrigin] = TargetOriginSuggested
					return next
				case OptionChooseOther:
					return ClearTarget(data)
				}
				return OnCategoryManageCancel(value, data)
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},

		StepPickTargetCategory: conversation.ChoiceStep{
			PromptText: func(conversation.Data) string { return MsgCategoryManagePickTargetCat },
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				extra := make([]conversation.ChoiceOption, 0, 2)
				if hasSuggestion(data) {
					extra = append(extra, BackOptionTo(StepSuggestTarget))
				}
				extra = append(extra, CancelOption)
				return CategoryOptions(subs, data, StepPickTargetSubcategory, extra...)
			},
			DeclaredNextSteps: []string{StepPickTargetSubcategory, StepSuggestTarget},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == OptionCancel {
					return OnCategoryManageCancel(OptionCancel, data)
				}
				if value == OptionBack {
					return ClearTarget(data)
				}
				next := ClearTarget(data)
				next[conversation.KeyTargetCategory] = value
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},

		StepPickTargetSubcategory: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				return MsgCategoryManagePickTargetSub(conversation.StringOrEmpty(data[conversation.KeyTargetCategory]))
			},
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				all, _ := subs.FindAllForUser(data.UserID())
				wantCategory := conversation.StringOrEmpty(data[conversation.KeyTargetCategory])
				sourceID := conversation.StringOrEmpty(data[conversation.KeySourceSubcategoryID])

				opts := make([]conversation.ChoiceOption, 0, len(all)+2)
				for _, s := range all {
					if s.Category != wantCategory {
						continue
					}
					id := strconv.FormatUint(uint64(s.ID), 10)
					if id == sourceID {
						continue
					}
					opts = append(opts, conversation.ChoiceOption{
						Label:    SubcategoryIcon(s) + " " + s.Subcategory,
						Value:    id,
						NextStep: StepConfirmCategoryManage,
					})
				}
				return append(opts, BackOptionTo(StepPickTargetCategory), CancelOption)
			},
			DeclaredNextSteps: []string{StepConfirmCategoryManage, StepPickTargetCategory},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == OptionCancel || value == OptionBack {
					return OnCategoryManageCancel(value, data)
				}
				all, _ := subs.FindAllForUser(data.UserID())
				for _, s := range all {
					if strconv.FormatUint(uint64(s.ID), 10) != value {
						continue
					}
					next := conversation.CopyData(data)
					next[conversation.KeyTargetSubcategoryID] = value
					next[conversation.KeyTargetSubcategory] = s.Subcategory
					next[conversation.KeyTargetOrigin] = TargetOriginManual
					return next
				}
				return data
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},

		StepConfirmCategoryManage: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				if isEmpty(data) {
					return MsgCategoryManageConfirmDelete(SourceLabel(data))
				}
				return MsgCategoryManageConfirmMerge(conversation.StringOrEmpty(data[conversation.KeyMovementCount]), SourceLabel(data), TargetLabel(data))
			},
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				opts := []conversation.ChoiceOption{
					{Label: "✅ Confirmar", Value: OptionConfirm, Finish: true},
				}
				switch {
				case isEmpty(data):
				case conversation.StringOrEmpty(data[conversation.KeyTargetOrigin]) == TargetOriginSuggested:
					opts = append(opts, BackOptionTo(StepSuggestTarget))
				default:
					opts = append(opts, BackOptionTo(StepPickTargetSubcategory))
				}
				return append(opts, CancelOption)
			},
			DeclaredNextSteps: []string{StepSuggestTarget, StepPickTargetSubcategory},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				switch value {
				case OptionCancel:
					return OnCategoryManageCancel(OptionCancel, data)
				case OptionBack:
					if conversation.StringOrEmpty(data[conversation.KeyTargetOrigin]) == TargetOriginSuggested {
						return ClearTarget(data)
					}
					return ClearTargetSubcategory(data)
				}
				next := conversation.CopyData(data)
				conversation.SetFlag(next, conversation.KeyConfirmed)
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},
	}

	flow, err := conversation.NewFlow(CategoryManageTargetFlowName, StepSuggestTarget, steps)
	if err != nil {
		panic(err)
	}
	return flow
}
