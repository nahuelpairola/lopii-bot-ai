package templates

import (
	"context"
	"strings"
	"testing"
)

// El link sólo tiene sentido en una invitación pendiente: una usada o
// vencida no se puede compartir, y ofrecer el link igual invita a mandarlo.
func TestAdmin_LinkOnlyOnPendingRows(t *testing.T) {
	data := AdminData{Rows: []InvitationRow{
		{Code: "ABC123", State: "pendiente", Date: "09/08/2026", Link: "https://t.me/testbot?start=ABC123"},
		{Code: "USED11", State: "usada", Date: "03/08/2026"},
		{Code: "OLD222", State: "expirada", Date: "01/08/2026"},
	}}

	var sb strings.Builder
	if err := Admin(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	for _, want := range []string{"ABC123", "USED11", "OLD222", "pendiente", "usada", "expirada"} {
		if !strings.Contains(html, want) {
			t.Errorf("falta %q en el render:\n%s", want, html)
		}
	}
	// Aparece dos veces: en el href y como texto copiable.
	if got := strings.Count(html, "start=ABC123"); got != 2 {
		t.Errorf("link de la pendiente = %d apariciones, want 2 (href + texto)", got)
	}
	for _, code := range []string{"USED11", "OLD222"} {
		if strings.Contains(html, "start="+code) {
			t.Errorf("la fila %s no es pendiente y no tendría que traer link:\n%s", code, html)
		}
	}
}

func TestAdmin_EmptyState(t *testing.T) {
	var sb strings.Builder
	if err := Admin(AdminData{}).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if !strings.Contains(html, "Todavía no generaste") {
		t.Errorf("sin invitaciones tendría que explicarlo:\n%s", html)
	}
	// El botón de generar es lo único que la vista vacía tiene que ofrecer.
	if !strings.Contains(html, RouteAdminInvitations) {
		t.Errorf("falta el botón de generar:\n%s", html)
	}
}
