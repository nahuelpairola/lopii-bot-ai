package templates

import (
	"context"
	"strings"
	"testing"
)

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
