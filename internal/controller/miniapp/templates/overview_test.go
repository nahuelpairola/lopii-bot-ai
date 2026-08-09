package templates

import (
	"context"
	"strings"
	"testing"
)

// La pestaña de admin viaja en el partial del Resumen y no en el TabBar: el
// TabBar lo renderiza el Shell sin autenticar, así que no puede saber quién
// mira. Llega por hx-swap-oob al slot que el Shell dejó vacío.
func TestOverview_AdminTabOnlyForAdmins(t *testing.T) {
	var admin, plain strings.Builder
	if err := Overview(OverviewData{IsAdmin: true}).Render(context.Background(), &admin); err != nil {
		t.Fatalf("render admin: %v", err)
	}
	if err := Overview(OverviewData{}).Render(context.Background(), &plain); err != nil {
		t.Fatalf("render común: %v", err)
	}

	for _, want := range []string{RouteAdmin, `id="` + AdminTabSlotID + `"`, `hx-swap-oob="true"`} {
		if !strings.Contains(admin.String(), want) {
			t.Errorf("falta %q en el resumen de un admin:\n%s", want, admin.String())
		}
	}
	// Sin el swap OOB el slot del Shell queda vacío y no hay pestaña: es la
	// única cosa que separa a un admin de un usuario común en esta vista.
	if strings.Contains(plain.String(), RouteAdmin) {
		t.Errorf("un usuario común no tendría que recibir la pestaña de admin:\n%s", plain.String())
	}
}

// El slot tiene que existir en el TabBar o el swap OOB no tiene dónde entrar:
// htmx descarta un OOB cuyo id no está en el DOM, sin error visible.
func TestTabBar_HasTheAdminSlot(t *testing.T) {
	var sb strings.Builder
	if err := TabBar(TabOverview).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if !strings.Contains(html, `id="`+AdminTabSlotID+`"`) {
		t.Errorf("el TabBar necesita el slot %q para el swap OOB:\n%s", AdminTabSlotID, html)
	}
	// Vacío y oculto: un no-admin no ve nada, y `hidden` lo saca del flex para
	// que las cuatro pestañas reales no queden descentradas.
	if !strings.Contains(html, "hidden") {
		t.Errorf("el slot vacío tiene que estar oculto:\n%s", html)
	}
	if strings.Contains(html, RouteAdmin) {
		t.Errorf("el TabBar se renderiza sin autenticar y no puede traer la ruta de admin:\n%s", html)
	}
}
