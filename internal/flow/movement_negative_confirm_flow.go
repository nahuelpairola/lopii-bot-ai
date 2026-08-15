package flow

import "lopiibot.com/internal/conversation"

const stepNegativeConfirm = "negative_confirm"

// gateChoiceKey guarda la elección del gate ("register"/"rewrite"/"missing")
// en el Data. La lee el finish (movement_finish.go) — misma pareja
// builder/finish que en el resto de los flujos.
const gateChoiceKey = "_gate_choice"

// NewMovementNegativeConfirmFlow is the insufficient-funds gate: a well-formed
// CREATE that would drive an account negative stops here instead of inserting.
// Three exits — register as-is, rewrite, or "something's missing" (abort with a
// hint). Seeded (via StartWithData) with the pending movement rows so
// "Registrar igual" can re-insert them with the balance check skipped.
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
