package miniapp

import (
	"time"

	"github.com/gin-gonic/gin"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/currency"
)

const (
	anchorParam   = "m"
	currencyParam = "c"
)

func nowInART() time.Time { return time.Now().In(constants.ArgentinaZone) }

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
