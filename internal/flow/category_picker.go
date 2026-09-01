package flow

import "lopiibot.com/internal/conversation"

func CategoryOptions(subs categoryLister, data conversation.Data, nextStep string, extra ...conversation.ChoiceOption) []conversation.ChoiceOption {
	cats, _ := subs.DistinctCategoriesForUser(data.UserID())
	opts := make([]conversation.ChoiceOption, 0, len(cats)+len(extra))
	for _, cat := range cats {
		opts = append(opts, conversation.ChoiceOption{
			Label:    subs.IconForCategory(data.UserID(), cat) + " " + cat,
			Value:    cat,
			NextStep: nextStep,
		})
	}
	return append(opts, extra...)
}

func BackOptionTo(step string) conversation.ChoiceOption {
	return conversation.ChoiceOption{Label: "⬅️ Atrás", Value: OptionBack, NextStep: step}
}
