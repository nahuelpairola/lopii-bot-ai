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

func TestAccountLeaf_ReconcilesAndFlagsUnclassified(t *testing.T) {
	data := AccountLeafData{
		AccountName:  "Galicia",
		OpeningLabel: "Saldo al 01/08",
		Opening:      "$382.500",
		ClosingLabel: "Saldo al 31/08",
		Closing:      "$450.000",
		BackQuery:    "/app/accounts?p=6m&m=2026-08&c=ARS",
		Rows: []MovementRow{
			{Icon: "🏦", Title: "Compra de dólares", Date: "10 ago", Amount: "-$100.000"},
			{Icon: "🏦", Title: "Ajuste", Date: "1 ago", Note: "Sin clasificar", Amount: "$2.000"},
		},
	}

	var sb strings.Builder
	if err := AccountLeaf(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	for _, want := range []string{"Galicia", "Saldo al 01/08", "$382.500", "Saldo al 31/08", "$450.000", "Compra de dólares", "Sin clasificar", "10 ago"} {
		if !strings.Contains(html, want) {
			t.Errorf("falta %q en la hoja:\n%s", want, html)
		}
	}
	if strings.Contains(html, "<table") {
		t.Error("la hoja es una lista, no una tabla: cuatro datos por fila en un webview angosto obligan a scrollear en horizontal")
	}
	if strings.Contains(html, MsgLeafCapped) {
		t.Error("con 2 filas no se avisa de ningún tope")
	}
}

func TestAccountLeaf_WarnsWhenCapped(t *testing.T) {
	data := AccountLeafData{
		AccountName: "Galicia",
		Capped:      true,
		Rows:        []MovementRow{{Title: "Coto", Date: "12 ago", Amount: "-$1"}},
	}

	var sb strings.Builder
	if err := AccountLeaf(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(sb.String(), MsgLeafCapped) {
		t.Error("si la lista quedó cortada hay que decirlo: si no, el saldo final no cuadra con lo que se ve y parece un error de la app")
	}
}

// El buscador y su cartel de "sin resultados" los maneja app.js por id. Si un
// `templ generate` mal corrido se come el input, no hay nada más que lo note.
func TestAccountLeaf_RendersFilterInput(t *testing.T) {
	data := AccountLeafData{
		AccountName: "Galicia",
		Rows:        []MovementRow{{Title: "Panadería", Date: "12 ago", Amount: "-$1"}},
	}

	var sb strings.Builder
	if err := AccountLeaf(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if !strings.Contains(html, `id="`+MovFilterID+`"`) {
		t.Errorf("falta el buscador, que app.js engancha por ese id:\n%s", html)
	}
	if !strings.Contains(html, `id="`+MovFilterEmptyID+`"`) {
		t.Errorf("falta el cartel de sin resultados: filtrar a cero deja la pantalla en blanco y parece roto:\n%s", html)
	}
	if !strings.Contains(html, "hidden") {
		t.Errorf("el cartel de sin resultados arranca oculto, lo muestra app.js:\n%s", html)
	}
}

func TestAccountLeaf_NoFilterInputWhenEmpty(t *testing.T) {
	data := AccountLeafData{AccountName: "Galicia", Empty: true}

	var sb strings.Builder
	if err := AccountLeaf(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(sb.String(), `id="`+MovFilterID+`"`) {
		t.Error("un buscador sobre una lista vacía es ruido")
	}
}

func TestAccountLeaf_EmptyStillShowsBalances(t *testing.T) {
	data := AccountLeafData{
		AccountName:  "Galicia",
		Empty:        true,
		OpeningLabel: "Saldo al 01/08",
		Opening:      "$382.500",
	}

	var sb strings.Builder
	if err := AccountLeaf(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, "Sin movimientos en esta cuenta en el período.") {
		t.Errorf("falta el estado vacío:\n%s", html)
	}
	if !strings.Contains(html, "$382.500") {
		t.Errorf("un mes sin movimientos igual tiene saldo, y es la respuesta a la pregunta que trajo al usuario acá:\n%s", html)
	}
}

// La fecha sale de las 50 filas y sube a un encabezado por dia. Lo que queda
// en la meta es solo "Categoria › Subcategoria", que es lo que distingue una
// fila de otra dentro del mismo dia.
func TestAccountLeaf_GroupsMovementsByDay(t *testing.T) {
	data := AccountLeafData{
		AccountName: "Efectivo",
		Rows: []MovementRow{
			{Title: "Super", Date: "12 ago", Note: "Comida › Super", Amount: "$-24.500"},
			{Title: "Nafta", Date: "12 ago", Note: "Auto › Combustible", Amount: "$-18.000"},
			{Title: "Sueldo", Date: "11 ago", Note: "Ingresos › Sueldo", Amount: "$900.000"},
		},
	}

	var sb strings.Builder
	if err := AccountLeaf(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if got := strings.Count(html, `class="group-title"`); got != 2 {
		t.Errorf("encabezados de dia = %d, want 2 (12 ago y 11 ago):\n%s", got, html)
	}
	if got := strings.Count(html, "12 ago"); got != 1 {
		t.Errorf("\"12 ago\" aparece %d veces, want 1: la fecha sube al encabezado y deja de repetirse por fila:\n%s", got, html)
	}
	if !strings.Contains(html, "Comida › Super") {
		t.Errorf("la meta perdio la categoria, que es lo unico que le queda:\n%s", html)
	}
}

// Cada grupo es su propio contenedor. Sin eso el buscador no tiene que
// esconder cuando ninguna fila de ese dia matchea, y el encabezado queda
// flotando solo.
func TestAccountLeaf_EachDayIsItsOwnGroupElement(t *testing.T) {
	data := AccountLeafData{
		AccountName: "Efectivo",
		Rows: []MovementRow{
			{Title: "Super", Date: "12 ago", Amount: "$-24.500"},
			{Title: "Sueldo", Date: "11 ago", Amount: "$900.000"},
		},
	}

	var sb strings.Builder
	if err := AccountLeaf(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if got := strings.Count(html, `class="mov-group"`); got != 2 {
		t.Errorf("contenedores de grupo = %d, want 2: el buscador esconde el grupo entero, no solo las filas:\n%s", got, html)
	}
}

// Una vista titulada abre con UN h1. No habia ningun h1 en la app: cuatro
// vistas titulaban con h2 y Evolucion con un <caption>. El tamanio no cambia
// —el h1 se estila al 1.75rem que tenia el h2, que es el techo de la rampa—
// asi que esto es estructura, no enfasis.
func TestAccountLeaf_OpensWithASingleH1(t *testing.T) {
	data := AccountLeafData{AccountName: "Galicia", BackQuery: "/app/accounts?p=6m"}

	var sb strings.Builder
	if err := AccountLeaf(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if got := strings.Count(html, "<h1>"); got != 1 {
		t.Errorf("h1 = %d, want 1:\n%s", got, html)
	}
	if strings.Contains(html, "<h2>") {
		t.Errorf("quedo un h2: la vista titula con h1 ahora:\n%s", html)
	}
	if strings.Index(html, "Volver") > strings.Index(html, "<h1>") {
		t.Errorf("el Volver quedo abajo del titulo:\n%s", html)
	}
	if !strings.Contains(html, "tappable") {
		t.Errorf("el Volver no toma .tappable, y median ~24px, el peor objetivo tactil de la app:\n%s", html)
	}
}
