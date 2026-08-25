package templates

import (
	"context"
	"strings"
	"testing"
	"time"

	"lopiibot.com/internal/currency"
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

// El tabbar saca el período de #app-state con hx-include (tabbar.templ). Admin
// no renderizaba PeriodHeader, así que mientras Admin estaba en pantalla ese
// elemento no existía y salir por cualquier pestaña mandaba la request sin
// p/pt/m/c: periodFromQuery caía a los defaults, en silencio, y el filtro
// cambiaba solo. Es la clase de bug que AGENTS.md llama la más cara de esta app.
// adminPeriod arma un Period cualquiera con los mismos helpers que ya usa
// period_test.go (presetsFor y art, del mismo paquete). Admin no lo muestra:
// sólo tiene que viajar para que @AppState pueda emitir los inputs.
func adminPeriod() Period {
	return NewPeriod(RouteAdmin, SinglePeriodScope, presetsFor(PresetMonth, Preset6M),
		art(2026, time.July), art(2026, time.July), currency.ARS)
}

func TestAdmin_CarriesAppStateSoLeavingItKeepsThePeriod(t *testing.T) {
	data := AdminData{Period: adminPeriod()}

	var sb strings.Builder
	if err := Admin(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if !strings.Contains(html, `id="app-state"`) {
		t.Fatalf("Admin no emite #app-state: salir de aca le resetea el periodo a todas las demas vistas\n%s", html)
	}
	for _, s := range PresetScopes {
		if !strings.Contains(html, `name="`+s.Param+`"`) {
			t.Errorf("falta el input del scope %q: ese slot de preset se pierde al salir de Admin\n%s", s.Param, html)
		}
	}
	for _, name := range []string{"m", "c"} {
		if !strings.Contains(html, `name="`+name+`"`) {
			t.Errorf("falta el input %q en #app-state\n%s", name, html)
		}
	}
}

// Admin no tiene contenido que dependa del período, así que los controles
// visibles serían un control que no hace nada. Lleva el estado, no la chrome.
func TestAdmin_DoesNotRenderVisiblePeriodControls(t *testing.T) {
	data := AdminData{Period: adminPeriod()}

	var sb strings.Builder
	if err := Admin(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if strings.Contains(html, `class="period-cursor"`) || strings.Contains(html, `class="chips"`) {
		t.Errorf("Admin no deberia mostrar controles de periodo: no tiene nada que filtrar\n%s", html)
	}
}
