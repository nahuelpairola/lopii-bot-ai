package templates

import (
	"context"
	"strings"
	"testing"
)

func TestMovementFragment_RendersRowsAndSentinel(t *testing.T) {
	rows := []MovementRow{{Title: "Coto", Date: "12 ago", Amount: "-$1"}}

	var sb strings.Builder
	if err := MovementFragment(rows, "/app/accounts?account=1&offset=100").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, "Coto") {
		t.Error("el fragmento tiene que traer las filas nuevas")
	}
	if !strings.Contains(html, `id="mov-more"`) {
		t.Error("el fragmento tiene que traer el próximo sentinel")
	}
	if !strings.Contains(html, `hx-trigger="revealed"`) {
		t.Error("el sentinel dispara al entrar en viewport, no por click")
	}
}

func TestMovementFragment_NoSentinelWhenLastPage(t *testing.T) {
	rows := []MovementRow{{Title: "Coto", Date: "12 ago", Amount: "-$1"}}

	var sb strings.Builder
	if err := MovementFragment(rows, "").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(sb.String(), `id="mov-more"`) {
		t.Error("en la última página no debe volver a pedirse más")
	}
}
