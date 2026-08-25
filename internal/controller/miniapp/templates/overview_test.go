package templates

import (
	"context"
	"strings"
	"testing"
)

// La vista de admin se saco de la Mini App entera por decision del usuario
// (2026-08-25): la pestania OOB, /app/admin y toda su maquinaria del lado del
// handler. Este test se queda como guarda de que no vuelva ningun residuo
// visible — el swap OOB era el unico que existia en el Resumen.
func TestOverview_CarriesNoOOBSwap(t *testing.T) {
	var sb strings.Builder
	if err := Overview(OverviewData{}).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(sb.String(), "hx-swap-oob") {
		t.Errorf("volvio un swap OOB al Resumen, y el unico que habia era el de admin:\n%s", sb.String())
	}
}

func TestTabBar_HasNoAdminSlot(t *testing.T) {
	var sb strings.Builder
	if err := TabBar(TabOverview).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(sb.String(), "tab-admin") {
		t.Errorf("quedo el slot vacio de la pestania de admin:\n%s", sb.String())
	}
}
