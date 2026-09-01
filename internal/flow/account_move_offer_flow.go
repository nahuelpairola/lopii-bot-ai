package flow

import (
	"fmt"

	"lopiibot.com/internal/conversation"
)

const (
	stepAccountMoveOffer = "account_move_offer_ask"
)

func NewAccountMoveOfferFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepAccountMoveOffer: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				return fmt.Sprintf("Tu default anterior era %s (saldo %s).\n¿Movés sus movimientos a %s? Así todo tu historial queda en la cuenta que vas a usar, y %s queda en 0.",
					conversation.StringOrEmpty(data[conversation.KeyMoveFromName]), conversation.StringOrEmpty(data[conversation.KeyMoveFromBalance]),
					conversation.StringOrEmpty(data[conversation.KeyMoveToName]), conversation.StringOrEmpty(data[conversation.KeyMoveFromName]))
			},
			Options: []conversation.ChoiceOption{
				{Label: "✅ Sí, mover", Value: MoveChoiceMove, Finish: true},
				{Label: "✋ No, dejar como está", Value: MoveChoiceKeep, Finish: true},
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := conversation.CopyData(data)
				next[conversation.KeyMoveChoice] = value
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},
	}
	flow, err := conversation.NewFlow(AccountMoveOfferFlowName, stepAccountMoveOffer, steps)
	if err != nil {
		panic(err)
	}
	return flow
}
