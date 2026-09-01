package flow

import (
	"strconv"

	"lopiibot.com/internal/conversation"
)

const (
	stepPickUpdateCandidate = "pick_update_candidate"
	stepConfirmUpdate       = "confirm_update"
)

func NewMovementUpdatePickFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepPickUpdateCandidate: conversation.ChoiceStep{
			PromptText: MsgPickUpdateCandidate,
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				labels := conversation.DecodeStringSlice(data, conversation.KeyCandidateLabels)
				opts := make([]conversation.ChoiceOption, 0, len(labels))
				for i, label := range labels {
					opts = append(opts, conversation.ChoiceOption{
						Label:  label,
						Value:  strconv.Itoa(i),
						Finish: true,
					})
				}
				return opts
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := conversation.CopyData(data)
				next["chosen_index"] = value
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},
	}

	flow, err := conversation.NewFlow(MovementUpdatePickFlowName, stepPickUpdateCandidate, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

func NewMovementUpdateConfirmFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepConfirmUpdate: conversation.ChoiceStep{
			PromptText: MsgConfirmUpdateDiff,
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: OptionConfirm, Finish: true},
				{Label: "❌ Cancelar", Value: "cancel", Finish: true},
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := conversation.CopyData(data)
				next[conversation.KeyConfirmed] = strconv.FormatBool(value == OptionConfirm)
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},
	}

	flow, err := conversation.NewFlow(MovementUpdateConfirmFlowName, stepConfirmUpdate, steps)
	if err != nil {
		panic(err)
	}
	return flow
}
