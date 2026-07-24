package messaging

import (
	"lopiibot.com/internal/conversation"
)

const (
	categoryMatchOfferFlowName      = "category_match_offer"
	stepCategoryMatchOffer          = "category_match_offer_step"
	categoryProposalConfirmFlowName = "category_proposal_confirm"
	stepCategoryProposalConfirm     = "category_proposal_confirm_step"

	optionUseExisting  = "use_existing"
	optionCreateNew    = "create_new"
	optionEditProposal = "edit_proposal"
)

// NewCategoryMatchOfferFlow is the single-step "ya existe algo parecido"
// gate: the LLM matched the user's CREATE_CATEGORY request to an existing
// taxonomy entry, so creating would be a duplicate. Seeded by
// startSubcategorySetup (free_text.go) with the matched entry.
func NewCategoryMatchOfferFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepCategoryMatchOffer: conversation.ChoiceStep{
			PromptText: msgCategoryMatchOffer,
			Options: []conversation.ChoiceOption{
				{Label: "✅ Usar esa", Value: optionUseExisting, Finish: true},
				{Label: "➕ Crear una distinta", Value: optionCreateNew, Finish: true},
				cancelOption,
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == optionCancel {
					return onAccountCreateEscape(optionCancel, data)
				}
				next := copyData(data)
				next[keyMatchChoice] = value
				return next
			},
			InvalidChoiceMessage: msgInvalidChoice,
		},
	}
	flow, err := conversation.NewFlow(categoryMatchOfferFlowName, stepCategoryMatchOffer, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

// NewCategoryProposalConfirmFlow is the single-step confirm over the LLM's
// complete proposal (placement + name + icon + description). Confirmar
// inserts as-is; Editar re-launches the classic wizard seeded with the
// proposal so every field is one tap to keep or a free-text answer to change.
func NewCategoryProposalConfirmFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepCategoryProposalConfirm: conversation.ChoiceStep{
			PromptText: msgCategoryProposalConfirm,
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: optionConfirm, Finish: true},
				{Label: "✏️ Editar", Value: optionEditProposal, Finish: true},
				cancelOption,
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == optionCancel {
					return onAccountCreateEscape(optionCancel, data)
				}
				next := copyData(data)
				if value == optionEditProposal {
					setFlag(next, keyEditProposal)
				}
				return next
			},
			InvalidChoiceMessage: msgInvalidChoice,
		},
	}
	flow, err := conversation.NewFlow(categoryProposalConfirmFlowName, stepCategoryProposalConfirm, steps)
	if err != nil {
		panic(err)
	}
	return flow
}
