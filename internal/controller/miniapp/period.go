package miniapp

import (
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/currency"
)

// Query params carrying the window state across every view.
const (
	presetParam   = "p"
	anchorParam   = "m"
	currencyParam = "c"
)

// nowInART is the clock every view reads.
func nowInART() time.Time { return time.Now().In(constants.ArgentinaZone) }

// periodFromQuery reads the window state off the request. Nothing here 400s:
// the params travel between views whose allowed presets differ, so anything
// unrecognized falls back to this view's default.
func periodFromQuery(ctx *gin.Context, allowed []string, def string) templates.Period {
	preset := ctx.Query(presetParam)
	if !slices.Contains(allowed, preset) {
		preset = def
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

	return templates.NewPeriod(ctx.Request.URL.Path, preset, anchor, current, cur, allowed)
}
