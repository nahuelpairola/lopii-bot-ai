package miniapp

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"lopiibot.com/internal/controller/miniapp/templates"
)

func staticRequest(t *testing.T, target, acceptEncoding string) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerStatic(engine.Group(templates.AppPrefix))

	req := httptest.NewRequest(http.MethodGet, target, nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestServeStatic_CompressesWhenTheClientAcceptsIt(t *testing.T) {
	raw := staticAssets["chart.umd.min.js"].raw
	if len(raw) == 0 {
		t.Fatal("no se embebio chart.umd.min.js")
	}

	rec := staticRequest(t, templates.AppPrefix+"/static/chart.umd.min.js", "gzip, deflate, br")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q: sin compresion el webview se baja %d KB de mas", got, len(raw)/1024)
	}
	if got := rec.Header().Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
		t.Errorf("Vary = %q: un proxy puede servirle el gzip a un cliente que no lo acepta", got)
	}
	if rec.Body.Len() >= len(raw) {
		t.Errorf("el cuerpo comprimido (%d) no es mas chico que el crudo (%d)", rec.Body.Len(), len(raw))
	}

	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("el cuerpo no es gzip valido: %v", err)
	}
	got, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("descomprimir: %v", err)
	}
	if len(got) != len(raw) {
		t.Errorf("descomprimido = %d bytes, want %d: el asset servido no es el embebido", len(got), len(raw))
	}
}

func TestServeStatic_FallsBackToRawWithoutAcceptEncoding(t *testing.T) {
	rec := staticRequest(t, templates.AppPrefix+"/static/app.css", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q para un cliente que no pidio gzip: el cuerpo llega ilegible", got)
	}
	if rec.Body.Len() != len(staticAssets["app.css"].raw) {
		t.Errorf("cuerpo = %d bytes, want %d", rec.Body.Len(), len(staticAssets["app.css"].raw))
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestServeStatic_CachesForeverOnlyWhenTheURLCarriesTheVersion(t *testing.T) {
	versioned := staticRequest(t, templates.StaticURL("app.js"), "gzip")
	if got := versioned.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Errorf("Cache-Control = %q para la URL versionada: cada apertura vuelve a pedir los 5 assets", got)
	}

	bare := staticRequest(t, templates.AppPrefix+"/static/app.js", "gzip")
	if got := bare.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q para la URL sin version: asi es como el webview se queda con un asset viejo para siempre", got)
	}

	stale := staticRequest(t, templates.AppPrefix+"/static/app.js?"+templates.AssetVersionParam+"=viejo", "gzip")
	if got := stale.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q para una version que ya no es la de este build", got)
	}
}

func TestServeStatic_404sOnAnUnknownAsset(t *testing.T) {
	rec := staticRequest(t, templates.AppPrefix+"/static/no-existe.js", "gzip")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestAssetVersion_IsDerivedFromTheEmbeddedBytes(t *testing.T) {
	if templates.AssetVersion == "dev" || len(templates.AssetVersion) != 12 {
		t.Fatalf("AssetVersion = %q: tiene que salir del hash de los assets embebidos, no del default", templates.AssetVersion)
	}
	for _, name := range []string{"app.css", "app.js", "chart.umd.min.js", "htmx.min.js", "pico.min.css"} {
		if _, ok := staticAssets[name]; !ok {
			t.Errorf("%s no entro al mapa de assets, asi que no cuenta para la version", name)
		}
	}
}
