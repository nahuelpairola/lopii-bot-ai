package flow

import (
	"lopiibot.com/internal/conversation"
)

const (
	stepCategoryMatchOffer      = "category_match_offer_step"
	stepCategoryProposalConfirm = "category_proposal_confirm_step"
)

// NewCategoryMatchOfferFlow is the single-step "ya existe algo parecido"
// gate: the LLM matched the user's CREATE_CATEGORY request to an existing
// taxonomy entry, so creating would be a duplicate. Seeded by
// startSubcategorySetup (free_text.go) with the matched entry.
func NewCategoryMatchOfferFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepCategoryMatchOffer: conversation.ChoiceStep{
			PromptText: MsgCategoryMatchOffer,
			Options: []conversation.ChoiceOption{
				{Label: "✅ Usar esa", Value: OptionUseExisting, Finish: true},
				{Label: "➕ Crear una distinta", Value: OptionCreateNew, Finish: true},
				CancelOption,
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == OptionCancel {
					return OnAccountCreateEscape(OptionCancel, data)
				}
				next := conversation.CopyData(data)
				next[conversation.KeyMatchChoice] = value
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},
	}
	flow, err := conversation.NewFlow(CategoryMatchOfferFlowName, stepCategoryMatchOffer, steps)
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
			PromptText: MsgCategoryProposalConfirm,
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: OptionConfirm, Finish: true},
				{Label: "✏️ Editar", Value: OptionEditProposal, Finish: true},
				CancelOption,
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				if value == OptionCancel {
					return OnAccountCreateEscape(OptionCancel, data)
				}
				next := conversation.CopyData(data)
				if value == OptionEditProposal {
					conversation.SetFlag(next, conversation.KeyEditProposal)
				}
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},
	}
	flow, err := conversation.NewFlow(CategoryProposalConfirmFlowName, stepCategoryProposalConfirm, steps)
	if err != nil {
		panic(err)
	}
	return flow
}
