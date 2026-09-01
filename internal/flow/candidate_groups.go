package flow

import (
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
)

type CandidateGroup struct {
	TransactionID string
	OldIDs        []string
	Rows          []movement.MovementRow
}

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
