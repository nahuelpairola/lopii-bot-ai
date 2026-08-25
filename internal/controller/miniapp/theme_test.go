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
