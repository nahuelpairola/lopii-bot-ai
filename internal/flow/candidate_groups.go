package flow

import (
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
)

// CandidateGroup is the row-based shape a picker candidate travels in through
// conversation.Data — distinct from transactionGroup (edge, agent_executor.go),
// which holds real movement.Movement rows straight from the DB. El encode (del
// lado del edge, que parte de transactionGroup) se queda en messaging; acá vive
// el tipo y el decode, que son puros y los comparten flow (MsgConfirmDelete) y
// el borde (finish de DELETE, applyStructuredCorrection).
type CandidateGroup struct {
	TransactionID string
	OldIDs        []string
	Rows          []movement.MovementRow
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
