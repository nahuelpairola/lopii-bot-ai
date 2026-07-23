package miniapp

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/currency"
)

func ctxFor(path, rawQuery string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", path+"?"+rawQuery, nil)
	return c
}

func TestPeriodFromQuery_DefaultsPerView(t *testing.T) {
	gin.SetMode(gin.TestMode)

	p := periodFromQuery(ctxFor(templates.RouteOverview, ""), templates.AllPresets, templates.PresetMonth)
	if p.Preset != templates.PresetMonth {
		t.Errorf("sin params: Preset = %q, want month", p.Preset)
	}
	if p.Route != templates.RouteOverview {
		t.Errorf("Route = %q, want %q", p.Route, templates.RouteOverview)
	}

	// Un preset que la vista no ofrece cae a su default, no da 400: la URL
	// puede venir de una tab que estaba en otra vista.
	p = periodFromQuery(ctxFor(templates.RouteEvolution, "p=month"), templates.TrendPresets, templates.Preset6M)
	if p.Preset != templates.Preset6M {
		t.Errorf("preset no permitido: Preset = %q, want 6m", p.Preset)
	}

	p = periodFromQuery(ctxFor(templates.RouteOverview, "p=basura"), templates.AllPresets, templates.PresetMonth)
	if p.Preset != templates.PresetMonth {
		t.Errorf("preset inválido: Preset = %q, want month", p.Preset)
	}
}

func TestPeriodFromQuery_AnchorAndCurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)

	p := periodFromQuery(ctxFor(templates.RouteOverview, "p=3m&m=2026-01&c=USD"), templates.AllPresets, templates.PresetMonth)
	if p.AnchorKey() != "2026-01" {
		t.Errorf("Anchor = %s, want 2026-01", p.AnchorKey())
	}
	if p.Currency != currency.USD {
		t.Errorf("Currency = %s, want USD", p.Currency)
	}

	// Un ancla futura se recorta al mes corriente.
	p = periodFromQuery(ctxFor(templates.RouteOverview, "m=2099-01"), templates.AllPresets, templates.PresetMonth)
	if p.Anchor.After(templates.CurrentMonth(nowInART())) {
		t.Errorf("Anchor = %s, no debe superar el mes corriente", p.AnchorKey())
	}

	// Una moneda desconocida cae a ARS, nunca a un tercer mundo.
	p = periodFromQuery(ctxFor(templates.RouteOverview, "c=EUR"), templates.AllPresets, templates.PresetMonth)
	if p.Currency != currency.ARS {
		t.Errorf("Currency = %s, want ARS", p.Currency)
	}
}
