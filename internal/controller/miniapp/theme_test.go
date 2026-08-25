package miniapp

import (
	"regexp"
	"strings"
	"testing"
)

// readAppCSS devuelve app.css desde el FS embebido — los mismos bytes exactos
// que se sirven en /app/static/app.css. Leerlo por staticFS y no por una ruta
// relativa es lo que hace que el test no dependa del working directory.
func readAppCSS(t *testing.T) string {
	t.Helper()
	b, err := staticFS.ReadFile("static/app.css")
	if err != nil {
		t.Fatalf("no se pudo leer app.css del FS embebido: %v", err)
	}
	return string(b)
}

// picoTokensMappedToTelegram es el contrato de la capa de tema: qué token de
// pico tiene que terminar leyendo qué variable de Telegram. Si alguien borra
// una línea del remapeo, esto se pone en rojo y dice cuál.
var picoTokensMappedToTelegram = map[string]string{
	"--pico-background-color":                 "--tg-theme-secondary-bg-color",
	"--pico-card-background-color":            "--tg-theme-section-bg-color",
	"--pico-card-sectioning-background-color": "--tg-theme-section-bg-color",
	"--pico-color":                            "--tg-theme-text-color",
	"--pico-h1-color":                         "--tg-theme-text-color",
	"--pico-h2-color":                         "--tg-theme-text-color",
	"--pico-h3-color":                         "--tg-theme-text-color",
	"--pico-muted-color":                      "--tg-theme-hint-color",
	"--pico-muted-border-color":               "--tg-theme-section-separator-color",
	"--pico-primary":                          "--tg-theme-accent-text-color",
	"--pico-primary-hover":                    "--tg-theme-accent-text-color",
	"--pico-primary-background":               "--tg-theme-button-color",
	"--pico-primary-inverse":                  "--tg-theme-button-text-color",
	"--pico-form-element-background-color":    "--tg-theme-section-bg-color",
	"--pico-form-element-border-color":        "--tg-theme-section-separator-color",
	"--pico-form-element-color":               "--tg-theme-text-color",
	"--pico-form-element-placeholder-color":   "--tg-theme-hint-color",
	"--pico-code-background-color":            "--tg-theme-section-bg-color",
	"--pico-mark-color":                       "--tg-theme-text-color",
}

func TestAppCSS_MapsPicoTokensToTelegramTheme(t *testing.T) {
	css := readAppCSS(t)

	for picoToken, tgVar := range picoTokensMappedToTelegram {
		// La declaración puede estar en cualquiera de los tres bloques de tema;
		// lo que se exige es que exista al menos una que ate los dos nombres.
		re := regexp.MustCompile(regexp.QuoteMeta(picoToken) + `\s*:[^;]*` + regexp.QuoteMeta(tgVar))
		if !re.MatchString(css) {
			t.Errorf("%s no está remapeado contra %s: sin eso ese token se queda en el azure de pico y no sigue el tema del usuario", picoToken, tgVar)
		}
	}
}

// Los tres contextos de tema de pico. El remapeo tiene que estar en los tres:
// en uno solo, o gana pico por especificidad, o nuestros fallbacks de un tema
// pisan la paleta del otro fuera de Telegram.
func TestAppCSS_RemapsAllThreePicoThemeContexts(t *testing.T) {
	css := readAppCSS(t)

	contexts := []struct {
		name    string
		snippet string
	}{
		{"claro", ":root:not([data-theme=dark])"},
		{"oscuro por sistema", "prefers-color-scheme: dark"},
		{"oscuro explícito", "[data-theme=dark]"},
	}
	for _, c := range contexts {
		if !strings.Contains(css, c.snippet) {
			t.Errorf("falta el bloque de tema %q (%s): sin él ese tema se queda sin remapear y se rompe fuera de Telegram", c.name, c.snippet)
		}
	}
}

// telegramVarWithoutFallback matchea var(--tg-…) SIN coma, o sea sin fallback.
var telegramVarWithoutFallback = regexp.MustCompile(`var\(\s*--tg-[a-z0-9-]+\s*\)`)

// Este no maneja el ciclo TDD: nace en verde y se queda de guardia. Un
// var(--tg-…) sin fallback rompe la app fuera de Telegram y en cualquier
// cliente anterior a Bot API 7.0, y no falla ruidosamente: simplemente el
// valor queda vacío y el elemento se pinta transparente o negro.
func TestAppCSS_EveryTelegramVarHasFallback(t *testing.T) {
	css := readAppCSS(t)

	if found := telegramVarWithoutFallback.FindAllString(css, -1); len(found) > 0 {
		t.Errorf("estas referencias a Telegram no tienen fallback y dejan la app rota fuera del cliente: %v", found)
	}
}

// El par de estado no puede ser un hex fijo una vez que el fondo lo elige el
// usuario: medido, el verde daba 3.35:1 en claro y el rojo 3.75:1 en oscuro,
// que pasan AA sólo por contar como texto grande (28px). Mezclarlos contra
// --pico-color —que es el texto del tema, y que Telegram ya garantizó legible
// contra su fondo— los sube a 4.82 y 4.83, arriba del 4.5 de texto normal.
func TestAppCSS_StatusColorsAreDerivedFromTheThemeText(t *testing.T) {
	css := readAppCSS(t)

	for _, tc := range []struct{ class, why string }{
		{".neto-good", "el verde no tiene equivalente en Telegram, así que se deriva mezclando contra el texto del tema"},
		{".neto-critical", "el rojo sale de --tg-theme-destructive-text-color y cae a la mezcla cuando el tema no lo trae"},
	} {
		i := strings.Index(css, tc.class)
		if i < 0 {
			t.Fatalf("desapareció la regla %s", tc.class)
		}
		rule := css[i:min(i+240, len(css))]
		if !strings.Contains(rule, "color-mix(") {
			t.Errorf("%s sigue con un color fijo: %s\nregla:\n%s", tc.class, tc.why, rule)
		}
		if !strings.Contains(rule, "var(--pico-color") {
			t.Errorf("%s no se mezcla contra el texto del tema, así que no sigue al fondo del usuario\nregla:\n%s", tc.class, rule)
		}
	}
}

func TestAppCSS_CriticalPrefersTelegramDestructiveColor(t *testing.T) {
	css := readAppCSS(t)

	i := strings.Index(css, ".neto-critical")
	if i < 0 {
		t.Fatal("desapareció la regla .neto-critical")
	}
	rule := css[i:min(i+240, len(css))]
	if !strings.Contains(rule, "--tg-theme-destructive-text-color") {
		t.Errorf("el rojo tiene que preferir el color destructivo del tema; el verde no tiene contraparte y por eso es asimétrico\nregla:\n%s", rule)
	}
}

// El sombreado de Evolución era Data Blue a alpha fijo sobre un fondo que
// ahora elige el usuario. Sigue siendo de dos pasos y sigue topeado —el techo
// existe porque una rampa vieja llegaba a 1.0 y el texto oscuro encima no
// pasaba contraste—, pero el tono ahora sale del acento del tema.
func TestAppCSS_HeatCellsUseTheThemeAccent(t *testing.T) {
	css := readAppCSS(t)

	for _, class := range []string{".cell-mild", ".cell-high"} {
		i := strings.Index(css, class)
		if i < 0 {
			t.Fatalf("desapareció la regla %s", class)
		}
		rule := css[i:min(i+200, len(css))]
		if strings.Contains(rule, "rgba(42, 120, 214") {
			t.Errorf("%s sigue clavada en Data Blue: sobre un tema personalizado puede quedar ilegible\nregla:\n%s", class, rule)
		}
		if !strings.Contains(rule, "var(--pico-primary") {
			t.Errorf("%s no sigue al acento del tema\nregla:\n%s", class, rule)
		}
	}
}

// La regla de 44px estaba escrita a mano en cuatro lugares y los chips de
// periodo —el control mas tocado de la app— se la perdian: 0.25rem de padding
// sobre texto de 0.8rem da unos 29px. .tappable la deja en un solo lugar.
func TestAppCSS_HasASingleTapTargetRule(t *testing.T) {
	css := readAppCSS(t)

	i := strings.Index(css, ".tappable")
	if i < 0 {
		t.Fatal("no existe .tappable: la regla de 44px sigue repetida en cada selector")
	}
	rule := css[i:min(i+300, len(css))]
	for _, want := range []string{"min-height: 44px", "min-width: 44px"} {
		if !strings.Contains(rule, want) {
			t.Errorf(".tappable no declara %q:\n%s", want, rule)
		}
	}
}

// Un chip es un segmented control, no un boton suelto. Medirlo 44px hizo dos
// danios a la vez: la fila de chips paso a ser lo mas pesado de la pantalla, y
// el min-height estiro la caja dejando la etiqueta arriba en vez de centrada.
// La pildora se queda compacta y el piso tactil sale de un ::after que no ocupa
// layout: 6 + 32 + 6 = 44.
func TestAppCSS_ChipsStayCompactButTappable(t *testing.T) {
	css := readAppCSS(t)

	i := strings.Index(css, `a[role="button"].chip {`)
	if i < 0 {
		t.Fatal("desaparecio la regla del chip, o volvio a ser un selector de clase sola: pico trae a[role=button]{display:inline-block} con (0,2,0) y le gana")
	}
	rule := css[i:min(i+700, len(css))]

	if strings.Contains(rule, "min-height: 44px") {
		t.Errorf("la pildora volvio a medir 44px, y un segmented control no se mide asi:\n%s", rule)
	}
	if !strings.Contains(rule, "height: 32px") {
		t.Errorf("el chip perdio su alto explicito, y vuelve a depender de lo que aporte pico:\n%s", rule)
	}
	// Los dos ejes: sin justify-content la etiqueta se va a la izquierda apenas
	// el chip crece de ancho, que es lo que hace [role=group] con flex: 1 1 auto.
	for _, want := range []string{"align-items: center", "justify-content: center"} {
		if !strings.Contains(rule, want) {
			t.Errorf("el chip no declara %q y la etiqueta queda descentrada:\n%s", want, rule)
		}
	}

	j := strings.Index(css, `a[role="button"].chip::after`)
	if j < 0 {
		t.Fatal("no existe el ::after del chip: la pildora es compacta pero el area tocable se quedo en 32px")
	}
	hit := css[j:min(j+300, len(css))]
	for _, want := range []string{"position: absolute", "inset: -6px 0"} {
		if !strings.Contains(hit, want) {
			t.Errorf("el area tocable del chip no declara %q (6 + 32 + 6 = 44):\n%s", want, hit)
		}
	}
}

// La variacion pasa a leerse como el pie de una seccion, que es lo que Telegram
// usa para exactamente este papel: texto en hint abajo del grupo, sin borde y
// sin relleno. El borde de 3px que tenia es lo que el detector de Impeccable
// marca como el tell mas reconocible de UI generada.
//
// Lo que NO cambia: sigue sin color de estado. Una variacion positiva no es
// "bien" como lo es un neto positivo, y The One Status Rule no se toca.
func TestAppCSS_VariacionIsASectionFooterNotASideTab(t *testing.T) {
	css := readAppCSS(t)

	i := strings.Index(css, ".variacion {")
	if i < 0 {
		t.Fatal("desaparecio la regla .variacion")
	}
	rule := css[i:min(i+400, len(css))]

	if strings.Contains(rule, "border-left") {
		t.Errorf("la variacion sigue con el borde lateral de 3px:\n%s", rule)
	}
	for _, banned := range []string{"neto-good", "neto-critical"} {
		if strings.Contains(rule, banned) {
			t.Errorf("la variacion tomo un color de estado (%s), y no puede: The One Status Rule\n%s", banned, rule)
		}
	}
}

// .tappable da el PISO tactil y nada mas. Centrar era correcto para su unico
// consumidor de la fase 2 (una tarjeta) y es incorrecto para todos los que
// entran ahora: un link de fila de Categorias, uno de Evolucion y un "Volver"
// se leen desde el principio de su celda. Centrarlos parece un error.
func TestAppCSS_TappableDoesNotCenterItsLabel(t *testing.T) {
	css := readAppCSS(t)

	i := strings.Index(css, ".tappable {")
	if i < 0 {
		t.Fatal("desaparecio la regla .tappable")
	}
	rule := css[i:min(i+300, len(css))]

	if strings.Contains(rule, "justify-content") {
		t.Errorf(".tappable sigue centrando, y los links de fila que lo toman se leen desde la izquierda:\n%s", rule)
	}
	for _, want := range []string{"min-height: 44px", "align-items: center"} {
		if !strings.Contains(rule, want) {
			t.Errorf(".tappable perdio %q, que es lo que la hace servir para algo:\n%s", want, rule)
		}
	}
}

// Un link adentro de una celda tiene que OCUPAR la celda: con display inline el
// min-height no hace nada y el area tocable sigue siendo la altura del texto.
// Es el motivo por el que estos tres median 24, 32 y 36px con la regla de 44px
// ya escrita en el archivo.
func TestAppCSS_RowLinksFillTheirCell(t *testing.T) {
	css := readAppCSS(t)

	i := strings.Index(css, ".row-link {")
	if i < 0 {
		t.Fatal("no existe .row-link: un link de fila con display inline ignora min-height")
	}
	rule := css[i:min(i+300, len(css))]
	for _, want := range []string{"display: flex", "min-height: 44px", "align-items: center"} {
		if !strings.Contains(rule, want) {
			t.Errorf(".row-link no declara %q:\n%s", want, rule)
		}
	}
}

// Ningun canvas tenia alto ni relacion de aspecto: Chart.js cae a 2:1 sobre el
// ancho que le toque. .chart-box es el padre posicionado con alto que Chart.js
// pide para poder soltar el aspecto (responsive:true pisa width/height del
// propio canvas en cada resize, asi que ponerlos ahi no sirve).
func TestAppCSS_ChartsHaveABox(t *testing.T) {
	css := readAppCSS(t)

	i := strings.Index(css, ".chart-box {")
	if i < 0 {
		t.Fatal("no existe .chart-box: sin un padre con alto, Chart.js dibuja todo 2:1")
	}
	rule := css[i:min(i+300, len(css))]
	for _, want := range []string{"position: relative", "height:"} {
		if !strings.Contains(rule, want) {
			t.Errorf(".chart-box no declara %q:\n%s", want, rule)
		}
	}
}

// La regla estaba escrita para una clase que ningun canvas lleva, asi que no
// matcheaba nada. El comportamiento igual era correcto —initCharts apaga la
// animacion por su cuenta bajo prefers-reduced-motion— asi que esto es codigo
// muerto que aparenta una garantia, no una garantia rota.
func TestAppCSS_HasNoDeadChartCanvasRule(t *testing.T) {
	if strings.Contains(readAppCSS(t), ".chart-canvas") {
		t.Error("volvio .chart-canvas, que no matchea ningun elemento de la app")
	}
}

// Evolucion se compacto a 0.85rem con 0.35rem de padding porque el td/th de
// pico desborda un telefono. Categorias tiene la MISMA forma de tres columnas
// y no habia recibido nada de eso: era una tabla pelada de pico a 360px.
func TestAppCSS_CategoryTableIsCompacted(t *testing.T) {
	css := readAppCSS(t)

	i := strings.Index(css, ".cat-table")
	if i < 0 {
		t.Fatal("no existe .cat-table: la tabla de Categorias sigue con el padding de pico")
	}
	rule := css[i:min(i+400, len(css))]
	for _, want := range []string{"0.85rem", "padding:"} {
		if !strings.Contains(rule, want) {
			t.Errorf(".cat-table no declara %q:\n%s", want, rule)
		}
	}
}
