package miniapp

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func readAppCSS(t *testing.T) string {
	t.Helper()
	b, err := staticFS.ReadFile("static/app.css")
	if err != nil {
		t.Fatalf("no se pudo leer app.css del FS embebido: %v", err)
	}
	return string(b)
}

func ruleAt(t *testing.T, css, selector string) string {
	t.Helper()
	i := strings.Index(css, selector)
	if i < 0 {
		t.Fatalf("desaparecio la regla %s", selector)
	}
	j := strings.Index(css[i:], "}")
	if j < 0 {
		t.Fatalf("la regla %s no cierra", selector)
	}
	return css[i : i+j+1]
}

func TestRuleAt_CutsAtTheRulesOwnClosingBrace(t *testing.T) {
	css := ".a { color: red; } .b { color: blue; }"

	if got := ruleAt(t, css, ".a {"); got != ".a { color: red; }" {
		t.Errorf("ruleAt = %q, quiere solo el bloque de .a", got)
	}
	if strings.Contains(ruleAt(t, css, ".a {"), "blue") {
		t.Error("la ventana se comio la regla siguiente")
	}
}

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
	"--pico-primary-focus":                    "--tg-theme-accent-text-color",
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
		re := regexp.MustCompile(regexp.QuoteMeta(picoToken) + `\s*:[^;]*` + regexp.QuoteMeta(tgVar))
		if !re.MatchString(css) {
			t.Errorf("%s no está remapeado contra %s: sin eso ese token se queda en el azure de pico y no sigue el tema del usuario", picoToken, tgVar)
		}
	}
}

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

var telegramVarWithoutFallback = regexp.MustCompile(`var\(\s*--tg-[a-z0-9-]+\s*\)`)

func TestAppCSS_EveryTelegramVarHasFallback(t *testing.T) {
	css := readAppCSS(t)

	if found := telegramVarWithoutFallback.FindAllString(css, -1); len(found) > 0 {
		t.Errorf("estas referencias a Telegram no tienen fallback y dejan la app rota fuera del cliente: %v", found)
	}
}

func TestAppCSS_StatusColorsAreDerivedFromTheThemeText(t *testing.T) {
	css := readAppCSS(t)

	for _, tc := range []struct{ class, why string }{
		{".neto-good {", "el verde no tiene equivalente en Telegram, así que se deriva mezclando contra el texto del tema"},
		{".neto-critical {", "el rojo sale de --tg-theme-destructive-text-color y cae a la mezcla cuando el tema no lo trae"},
	} {
		rule := ruleAt(t, css, tc.class)
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

	rule := ruleAt(t, css, ".neto-critical {")
	if !strings.Contains(rule, "--tg-theme-destructive-text-color") {
		t.Errorf("el rojo tiene que preferir el color destructivo del tema; el verde no tiene contraparte y por eso es asimétrico\nregla:\n%s", rule)
	}
}

func TestAppCSS_HasASingleTapTargetRule(t *testing.T) {
	css := readAppCSS(t)

	rule := ruleAt(t, css, ".tappable {")
	for _, want := range []string{"min-height: 44px", "min-width: 44px"} {
		if !strings.Contains(rule, want) {
			t.Errorf(".tappable no declara %q:\n%s", want, rule)
		}
	}
}

func TestAppCSS_ChipsStayCompactButTappable(t *testing.T) {
	css := readAppCSS(t)

	rule := ruleAt(t, css, `a[role="button"].chip {`)

	if strings.Contains(rule, "min-height: 44px") {
		t.Errorf("la pildora volvio a medir 44px, y un segmented control no se mide asi:\n%s", rule)
	}
	if !strings.Contains(rule, "height: 32px") {
		t.Errorf("el chip perdio su alto explicito, y vuelve a depender de lo que aporte pico:\n%s", rule)
	}
	for _, want := range []string{"align-items: center", "justify-content: center"} {
		if !strings.Contains(rule, want) {
			t.Errorf("el chip no declara %q y la etiqueta queda descentrada:\n%s", want, rule)
		}
	}

	hit := ruleAt(t, css, `a[role="button"].chip::after`)
	for _, want := range []string{"position: absolute", "inset: -6px 0"} {
		if !strings.Contains(hit, want) {
			t.Errorf("el area tocable del chip no declara %q (6 + 32 + 6 = 44):\n%s", want, hit)
		}
	}
}

func TestAppCSS_VariacionIsASectionFooterNotASideTab(t *testing.T) {
	css := readAppCSS(t)

	rule := ruleAt(t, css, ".variacion {")

	if strings.Contains(rule, "border-left") {
		t.Errorf("la variacion sigue con el borde lateral de 3px:\n%s", rule)
	}
	for _, banned := range []string{"neto-good", "neto-critical"} {
		if strings.Contains(rule, banned) {
			t.Errorf("la variacion tomo un color de estado (%s), y no puede: The One Status Rule\n%s", banned, rule)
		}
	}
}

func TestAppCSS_TappableDoesNotCenterItsLabel(t *testing.T) {
	css := readAppCSS(t)

	rule := ruleAt(t, css, ".tappable {")

	if strings.Contains(rule, "justify-content") {
		t.Errorf(".tappable sigue centrando, y los links de fila que lo toman se leen desde la izquierda:\n%s", rule)
	}
	for _, want := range []string{"min-height: 44px", "align-items: center"} {
		if !strings.Contains(rule, want) {
			t.Errorf(".tappable perdio %q, que es lo que la hace servir para algo:\n%s", want, rule)
		}
	}
}

func TestAppCSS_RowLinksFillTheirCell(t *testing.T) {
	css := readAppCSS(t)

	rule := ruleAt(t, css, ".row-link {")
	for _, want := range []string{"display: flex", "min-height: 44px", "align-items: center"} {
		if !strings.Contains(rule, want) {
			t.Errorf(".row-link no declara %q:\n%s", want, rule)
		}
	}
}

func TestAppCSS_ChartsHaveABox(t *testing.T) {
	css := readAppCSS(t)

	rule := ruleAt(t, css, ".chart-box {")
	for _, want := range []string{"position: relative", "height:"} {
		if !strings.Contains(rule, want) {
			t.Errorf(".chart-box no declara %q:\n%s", want, rule)
		}
	}
}

func TestAppCSS_HasNoDeadChartCanvasRule(t *testing.T) {
	if strings.Contains(readAppCSS(t), ".chart-canvas") {
		t.Error("volvio .chart-canvas, que no matchea ningun elemento de la app")
	}
}

func TestAppCSS_CategoryTableIsCompacted(t *testing.T) {
	css := readAppCSS(t)

	if rule := ruleAt(t, css, ".cat-table {"); !strings.Contains(rule, "0.85rem") {
		t.Errorf(".cat-table no declara 0.85rem: la tabla de Categorias sigue en tamanio de cuerpo:\n%s", rule)
	}
	if rule := ruleAt(t, css, ".cat-table thead th {"); !strings.Contains(rule, "padding:") {
		t.Errorf("las celdas de .cat-table no declaran padding, y vuelven al de pico:\n%s", rule)
	}
}

func TestAppCSS_HorizontalScrollerDoesNotTrapVerticalScroll(t *testing.T) {
	css := readAppCSS(t)

	rule := ruleAt(t, css, ".evolution-scroll {")
	if strings.Contains(rule, "overscroll-behavior:") {
		t.Errorf("el scroller sigue conteniendo los DOS ejes y atrapa el scroll vertical de la pagina:\n%s", rule)
	}
	if !strings.Contains(rule, "overscroll-behavior-x: contain") {
		t.Errorf("se perdio la contencion horizontal, que si es intencional:\n%s", rule)
	}

	if strings.Contains(ruleAt(t, css, ".content-area {"), "overscroll-behavior") {
		t.Error(".content-area volvio a declarar overscroll-behavior, que ahi no hace nada")
	}
}

func fontSizeRem(t *testing.T, rule string) float64 {
	t.Helper()
	m := regexp.MustCompile(`font-size:\s*([0-9.]+)rem`).FindStringSubmatch(rule)
	if m == nil {
		t.Fatalf("la regla no declara font-size en rem:\n%s", rule)
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		t.Fatalf("font-size ilegible %q: %v", m[1], err)
	}
	return v
}

func TestAppCSS_TitlesAreSmallerThanMoney(t *testing.T) {
	css := readAppCSS(t)

	h1 := fontSizeRem(t, ruleAt(t, css, "#content h1 {"))
	kpi := fontSizeRem(t, ruleAt(t, css, ".grid article .money {"))
	total := fontSizeRem(t, ruleAt(t, css, ".page-total .money {"))

	if h1 >= kpi {
		t.Errorf("el titulo mide %grem y la cifra KPI %grem: un rotulo no puede empatarle a un numero", h1, kpi)
	}
	if h1 >= total {
		t.Errorf("el titulo mide %grem y el total %grem: mismo problema", h1, total)
	}
	if total > kpi {
		t.Errorf("el total (%grem) le paso a la cifra KPI (%grem), que es el techo de la rampa", total, kpi)
	}
}

func TestAppCSS_DataRowsHaveOneHeight(t *testing.T) {
	css := readAppCSS(t)

	for _, sel := range []string{".cat-table tbody th, .cat-table tbody td {", ".evolution tbody th, .evolution tbody td {"} {
		rule := ruleAt(t, css, sel)
		if !strings.Contains(rule, "height: 44px") {
			t.Errorf("%s no fija el alto en la celda, asi que la fila lo hereda del link mas el padding:\n%s", sel, rule)
		}
		if !strings.Contains(rule, "padding: 0 ") {
			t.Errorf("%s sigue cobrando padding vertical arriba de los 44px:\n%s", sel, rule)
		}
	}

	for _, sel := range []string{".cat-table thead th {", ".evolution thead th {"} {
		if rule := ruleAt(t, css, sel); !strings.Contains(rule, "padding: 0.35rem 0.4rem") {
			t.Errorf("%s perdio su padding compacto:\n%s", sel, rule)
		}
	}
}

func TestAppCSS_HeatCellsAreDerivedFromTheTextColour(t *testing.T) {
	css := readAppCSS(t)

	for _, sel := range []string{".cell-mild {", ".cell-high {"} {
		rule := ruleAt(t, css, sel)
		if strings.Contains(rule, "rgba(42, 120, 214") {
			t.Errorf("%s volvio a un color fijo: sobre un tema personalizado puede quedar ilegible:\n%s", sel, rule)
		}
		if strings.Contains(rule, "--pico-primary") {
			t.Errorf("%s mezcla contra el acento del usuario, que es arbitrario: el texto encima queda sin contraste garantizado:\n%s", sel, rule)
		}
		if !strings.Contains(rule, "--pico-color") {
			t.Errorf("%s no deriva del color de texto, asi que el tinte no sigue al tema:\n%s", sel, rule)
		}
	}
}
