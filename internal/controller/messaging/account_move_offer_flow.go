package messaging

import (
	"fmt"

	"lopiibot.com/internal/conversation"
)

const (
	accountMoveOfferFlowName = "account_move_offer"
	stepAccountMoveOffer     = "account_move_offer_ask"

	// move-choice button values stored under keyMoveChoice.
	moveChoiceMove = "move"
	moveChoiceKeep = "keep"
)

// NewAccountMoveOfferFlow is the one-question follow-up after a default
// change: move the old default's movements into the new one?
func NewAccountMoveOfferFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepAccountMoveOffer: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				return fmt.Sprintf("Tu default anterior era %s (saldo %s).\n¿Movés sus movimientos a %s? Así todo tu historial queda en la cuenta que vas a usar, y %s queda en 0.",
					stringOrEmpty(data[keyMoveFromName]), stringOrEmpty(data[keyMoveFromBalance]),
					stringOrEmpty(data[keyMoveToName]), stringOrEmpty(data[keyMoveFromName]))
			},
			Options: []conversation.ChoiceOption{
				{Label: "✅ Sí, mover", Value: moveChoiceMove, Finish: true},
				{Label: "✋ No, dejar como está", Value: moveChoiceKeep, Finish: true},
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := copyData(data)
				next[keyMoveChoice] = value
				return next
			},
			InvalidChoiceMessage: msgInvalidChoice,
		},
	}
	flow, err := conversation.NewFlow(accountMoveOfferFlowName, stepAccountMoveOffer, steps)
	if err != nil {
		panic(err)
	}
	return flow
}
