package flow

import (
	"strconv"

	"lopiibot.com/internal/conversation"
)

const (
	StepPickDeleteCandidate = "pick_delete_candidate"
	StepConfirmDelete       = "confirm_delete"
)

func NewMovementDeleteFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		StepPickDeleteCandidate: conversation.ChoiceStep{
			PromptText: MsgPickDeleteCandidate,
			SkipIf: func(data conversation.Data) (string, bool) {
				if conversation.StringOrEmpty(data[conversation.KeyResolvedIndex]) != "" {
					return StepConfirmDelete, true
				}
				return "", false
			},
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				labels := conversation.DecodeStringSlice(data, conversation.KeyCandidateLabels)
				opts := make([]conversation.ChoiceOption, 0, len(labels))
				for i, label := range labels {
					opts = append(opts, conversation.ChoiceOption{
						Label:    label,
						Value:    strconv.Itoa(i),
						NextStep: StepConfirmDelete,
					})
				}
				return opts
			},
			DeclaredNextSteps: []string{StepConfirmDelete},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := conversation.CopyData(data)
				next[conversation.KeyResolvedIndex] = value
				return next
			},
			InvalidChoiceMessage: MsgInvalidChoice,
		},
		StepConfirmDelete: conversation.ChoiceStep{
			PromptText: MsgConfirmDelete,
			Options: []conversation.ChoiceOption{
				{Label: "🗑️ Confirmar borrado", Value: OptionConfirm, Finish: true},
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

	flow, err := conversation.NewFlow(MovementDeleteFlowName, StepPickDeleteCandidate, steps)
	if err != nil {
		panic(err)
	}
	return flow
}
