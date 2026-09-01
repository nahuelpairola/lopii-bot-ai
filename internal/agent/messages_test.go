package agent

import (
	"strings"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
)

func TestMsgConfirmDelete_IncludesSubcategoryDescriptionDate(t *testing.T) {
	data := conversation.Data{
		conversation.KeyCandidateGroups: flow.EncodeCandidateGroups([]flow.CandidateGroup{
			{
				OldIDs: []string{"1"},
				Rows:   []movement.MovementRow{{Category: "Alimentación", Subcategory: "Café", Amount: "3000", Currency: "ARS", Description: "Café con Juan", Date: "2026-07-04"}},
			},
		}),
	}

	msg := flow.MsgConfirmDelete(data)
	for _, want := range []string{"Alimentación", "Café", "Café con Juan", "2026-07-04"} {
		if !strings.Contains(msg, want) {
			t.Errorf("delete message %q missing %q", msg, want)
		}
	}
}
