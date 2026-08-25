package templates

import (
	"context"
	"strings"
	"testing"
)

// La pestania de admin se saco de la Mini App por decision del usuario
// (2026-08-25). Con ella se fue toda su maquinaria: el ancla hx-swap-oob del
// partial del Resumen, el slot vacio del TabBar y la const del id.
//
// La ruta /app/admin SIGUE existiendo y sigue cerrada por requireAdmin: lo que
// se fue es la unica forma de llegar desde la UI. Un webview no tiene barra de
// direcciones, asi que en la practica la vista quedo inalcanzable.
func TestOverview_NoLongerCarriesTheAdminTab(t *testing.T) {
	var sb strings.Builder
	if err := Overview(OverviewData{}).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if strings.Contains(html, RouteAdmin) {
		t.Errorf("volvio la pestania de admin al Resumen:\n%s", html)
	}
	if strings.Contains(html, "hx-swap-oob") {
		t.Errorf("quedo un swap OOB en el Resumen, y el unico que habia era el de admin:\n%s", html)
	}
}

func TestTabBar_HasNoAdminSlot(t *testing.T) {
	var sb strings.Builder
	if err := TabBar(TabOverview).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if strings.Contains(html, "tab-admin") {
		t.Errorf("quedo el slot vacio de la pestania de admin:\n%s", html)
	}
	if strings.Contains(html, RouteAdmin) {
		t.Errorf("el TabBar trae la ruta de admin:\n%s", html)
	}
}
