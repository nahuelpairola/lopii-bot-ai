package miniapp

import (
	"time"

	"github.com/gin-gonic/gin"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/currency"
)

// Query params carrying the window state across every view. The preset params
// are not here: they belong to their PresetScope (templates/period.go).
const (
	anchorParam   = "m"
	currencyParam = "c"
)

// nowInART is the clock every view reads.
func nowInART() time.Time { return time.Now().In(constants.ArgentinaZone) }

// periodFromQuery reads the window state off the request. It resolves EVERY
// scope, not just this view's: the inactive slot has to reach the view that
// owns it, and resolving it here means no unvalidated value ever travels.
func periodFromQuery(ctx *gin.Context, scope templates.PresetScope) templates.Period {
	presets := make(map[string]string, len(templates.PresetScopes))
	for _, s := range templates.PresetScopes {
		presets[s.Param] = s.Resolve(ctx.Query(s.Param))
	}

	current := templates.CurrentMonth(nowInART())
	anchor := current
	parsed, err := time.ParseInLocation("2006-01", ctx.Query(anchorParam), constants.ArgentinaZone)
	if err == nil && !parsed.After(current) {
		anchor = parsed
	}

	cur := currency.ARS
	if ctx.Query(currencyParam) == constants.USD {
		cur = currency.USD
	}

	return templates.NewPeriod(ctx.Request.URL.Path, scope, presets, anchor, current, cur)
}
