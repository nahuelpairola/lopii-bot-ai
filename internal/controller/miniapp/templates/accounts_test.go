package templates

import (
	"context"
	"strings"
	"testing"
)

func TestAccounts_StarsOnlyTheDefaultAccount(t *testing.T) {
	data := AccountsData{
		Snapshots: []AccountSnapshot{
			{Name: "Efectivo", Balance: "$1.000", IsDefault: true},
			{Name: "Banco", Balance: "$2.000"},
		},
	}

	var sb strings.Builder
	if err := Accounts(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if got := strings.Count(html, "★"); got != 1 {
		t.Errorf("estrellas = %d, want 1 (la default y sólo la default)", got)
	}
	if !strings.Contains(html, `aria-label="Cuenta por defecto"`) {
		t.Errorf("la estrella no tiene nombre accesible:\n%s", html)
	}
}

func TestAccounts_CardsLinkToTheirLeaf(t *testing.T) {
	data := AccountsData{
		Snapshots: []AccountSnapshot{
			{Name: "Efectivo", Balance: "$1.000", Href: "/app/accounts?p=6m&m=2026-08&c=ARS&account=1"},
		},
	}

	var sb strings.Builder
	if err := Accounts(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if !strings.Contains(html, "account=1") {
		t.Errorf("la tarjeta tiene que linkear a su hoja:\n%s", html)
	}
	if !strings.Contains(html, `hx-target="#content"`) {
		t.Errorf("el link tiene que navegar por htmx, no recargar la app:\n%s", html)
	}
	if !strings.Contains(html, `hx-push-url="true"`) {
		t.Errorf("sin push-url el botón Atrás de Telegram no vuelve:\n%s", html)
	}
}
