package templates

import (
	"context"
	"strings"
	"testing"
)

func renderShell(t *testing.T) string {
	t.Helper()

	var sb strings.Builder
	if err := Shell(TabOverview, RouteOverview).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

func TestShell_DoesNotShipChartJSOnEveryView(t *testing.T) {
	html := renderShell(t)

	if strings.Contains(html, `<script defer src="`+AppPrefix+`/static/chart.umd.min.js`) {
		t.Error("chart.umd.min.js vuelve a cargarse en el shell: son 204 KB que Evolucion paga sin tener ningun grafico")
	}
	if !strings.Contains(html, `data-chart-src=`) {
		t.Error("el shell no publica data-chart-src, asi que app.js no tiene de donde cargar la libreria bajo demanda")
	}
}

func TestShell_DefersEveryScript(t *testing.T) {
	html := renderShell(t)

	for i := 0; ; {
		at := strings.Index(html[i:], "<script")
		if at < 0 {
			break
		}
		at += i
		end := strings.Index(html[at:], ">")
		if end < 0 {
			t.Fatalf("tag <script> sin cerrar:\n%s", html)
		}
		tag := html[at : at+end]
		if !strings.Contains(tag, "defer") {
			t.Errorf("%s bloquea el parseo del HTML", tag)
		}
		i = at + end
	}
}

func TestShell_VersionsEveryLocalAsset(t *testing.T) {
	html := renderShell(t)

	for _, name := range []string{"pico.min.css", "app.css", "htmx.min.js", "app.js", "chart.umd.min.js"} {
		want := AppPrefix + "/static/" + name + "?" + AssetVersionParam + "="
		if !strings.Contains(html, want) {
			t.Errorf("%s no viaja versionado: sin cache-busting el webview se queda con la copia vieja para siempre", name)
		}
	}
	if strings.Contains(html, `"`+AppPrefix+`/static/app.css"`) {
		t.Error("quedo una URL de asset sin el parametro de version")
	}
}
