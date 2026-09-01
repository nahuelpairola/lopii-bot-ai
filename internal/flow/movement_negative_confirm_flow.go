package flow

import "lopiibot.com/internal/conversation"

const stepNegativeConfirm = "negative_confirm"

const gateChoiceKey = "_gate_choice"

func NewMovementNegativeConfirmFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepNegativeConfirm: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string {
				return conversation.StringOrEmpty(data[conversation.KeyGatePrompt])
			},
			Options: []conversation.ChoiceOption{
				{Label: "✅ Registrar igual", Value: "register", Finish: true},
				{Label: "✍️ Reescribir", Value: "rewrite", Finish: true},
				{Label: "➕ Falta registrar algo", Value: "missing", Finish: true},
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := conversation.CopyData(data)
				next[gateChoiceKey] = value
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},
	}
	flow, err := conversation.NewFlow(MovementNegativeConfirmFlowName, stepNegativeConfirm, steps)
	if err != nil {
		panic(err)
	}
	return flow
}
