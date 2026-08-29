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

func TestPeriodFromQuery_DefaultsPerScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	p := periodFromQuery(ctxFor(templates.RouteOverview, ""), templates.SinglePeriodScope)
	if p.Preset != templates.PresetMonth {
		t.Errorf("sin params: Preset = %q, want month", p.Preset)
	}
	if p.Route != templates.RouteOverview {
		t.Errorf("Route = %q, want %q", p.Route, templates.RouteOverview)
	}

	p = periodFromQuery(ctxFor(templates.RouteEvolution, "pt=month"), templates.TrendScope)
	if p.Preset != templates.Preset6M {
		t.Errorf("preset no permitido: Preset = %q, want 6m", p.Preset)
	}

	p = periodFromQuery(ctxFor(templates.RouteOverview, "p=basura"), templates.SinglePeriodScope)
	if p.Preset != templates.PresetMonth {
		t.Errorf("preset inválido: Preset = %q, want month", p.Preset)
	}
}

func TestPeriodFromQuery_AnchorAndCurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)

	p := periodFromQuery(ctxFor(templates.RouteOverview, "p=3m&m=2026-01&c=USD"), templates.SinglePeriodScope)
	if p.AnchorKey() != "2026-01" {
		t.Errorf("Anchor = %s, want 2026-01", p.AnchorKey())
	}
	if p.Currency != currency.USD {
		t.Errorf("Currency = %s, want USD", p.Currency)
	}

	p = periodFromQuery(ctxFor(templates.RouteOverview, "m=2099-01"), templates.SinglePeriodScope)
	if p.Anchor.After(templates.CurrentMonth(nowInART())) {
		t.Errorf("Anchor = %s, no debe superar el mes corriente", p.AnchorKey())
	}

	p = periodFromQuery(ctxFor(templates.RouteOverview, "c=EUR"), templates.SinglePeriodScope)
	if p.Currency != currency.ARS {
		t.Errorf("Currency = %s, want ARS", p.Currency)
	}
}

func TestPeriodFromQuery_ScopesDoNotOverwriteEachOther(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ev := periodFromQuery(ctxFor(templates.RouteEvolution, "p=month&pt=6m"), templates.TrendScope)
	if ev.Preset != templates.Preset6M {
		t.Errorf("Evolución: Preset = %q, want 6m", ev.Preset)
	}
	if got := ev.Presets[templates.SinglePeriodScope.Param]; got != templates.PresetMonth {
		t.Errorf("Evolución pisó el slot single-period: %q, want month", got)
	}

	ov := periodFromQuery(ctxFor(templates.RouteOverview, "p=month&pt=6m"), templates.SinglePeriodScope)
	if ov.Preset != templates.PresetMonth {
		t.Errorf("Resumen: Preset = %q, want month", ov.Preset)
	}
	if got := ov.Presets[templates.TrendScope.Param]; got != templates.Preset6M {
		t.Errorf("Resumen pisó el slot trend: %q, want 6m", got)
	}

	p := periodFromQuery(ctxFor(templates.RouteOverview, "pt=basura"), templates.SinglePeriodScope)
	if got := p.Presets[templates.TrendScope.Param]; got != templates.Preset6M {
		t.Errorf("slot inactivo sin validar: %q, want 6m", got)
	}
}

func appStateHas(body, param, value string) bool {
	return bodyContains(body, `name="`+param+`" value="`+value+`"`)
}
