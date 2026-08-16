package flow

import (
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
)

// CandidateGroup is the row-based shape a picker candidate travels in through
// conversation.Data — distinct from transactionGroup (agent), which holds real
// movement.Movement rows straight from the DB. Acá viven el tipo, el encode y el
// decode, que son puros y los comparten flow (MsgConfirmDelete) y el borde
// (finish de DELETE, applyStructuredCorrection). El edge parte de
// transactionGroup y convierte ANTES de encodear.
type CandidateGroup struct {
	TransactionID string
	OldIDs        []string
	Rows          []movement.MovementRow
}

// EncodeCandidateGroups serializa grupos a la forma de Data que lee
// DecodeCandidateGroups. El borde (agent) la usa para seeds del picker y el
// payload parkeado; flow para los tests del flujo de DELETE.
func EncodeCandidateGroups(groups []CandidateGroup) []interface{} {
	encoded := make([]interface{}, 0, len(groups))
	for _, g := range groups {
		encoded = append(encoded, map[string]interface{}{
			"transaction_id": g.TransactionID,
			"old_ids":        conversation.EncodeStringSlice(g.OldIDs),
			"rows":           movement.EncodeMovementRows(g.Rows),
		})
	}
	return encoded
}

func DecodeCandidateGroups(data conversation.Data) []CandidateGroup {
	raw, _ := data[conversation.KeyCandidateGroups].([]interface{})
	groups := make([]CandidateGroup, 0, len(raw))
	for _, r := range raw {
		m, _ := r.(map[string]interface{})
		groups = append(groups, CandidateGroup{
			TransactionID: conversation.StringOrEmpty(m["transaction_id"]),
			OldIDs:        conversation.DecodeStringSlice(conversation.Data{"ids": m["old_ids"]}, "ids"),
			Rows:          movement.DecodeMovementRows(conversation.Data{conversation.KeyMovements: m["rows"]}),
		})
	}
	return groups
}
