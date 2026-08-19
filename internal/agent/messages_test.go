package agent

import (
	"strings"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
)

// TestMsgConfirmDelete_IncludesSubcategoryDescriptionDate: el único camino que
// hoy pasa por MsgConfirmDelete con filas completas es el borrado "desde adentro"
// del grupo (MovementDeleteFlowFinish) — el bot borró, confirmó, y no puede
// confirmar nada que no tenga. Pero nada impone que el paso de confirmación sea
// sordo a lo que ya sabe la fila, y en un DELETEs "desde la lista" la fila va a
// venir completa. Si el mensaje repite una subcategoría que nunca se muestra en
// ese flujo, es un bug visible; si no, es un detalle que nadie nota.
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
