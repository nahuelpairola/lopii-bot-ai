package templates

import (
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"time"
	"unicode"

	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/currency"
)

const (
	PresetMonth = "month"
	Preset3M    = "3m"
	Preset6M    = "6m"
	PresetYear  = "year"
)

const (
	anchorLayout = "2006-01"
	dayLayout    = "2006-01-02"
)

var presetMonths = map[string]int{PresetMonth: 1, Preset3M: 3, Preset6M: 6, PresetYear: 12}

type PresetScope struct {
	Param   string
	Allowed []string
	Default string
}

var (
	SinglePeriodScope = PresetScope{
		Param:   "p",
		Allowed: []string{PresetMonth, Preset3M, Preset6M, PresetYear},
		Default: PresetMonth,
	}
	TrendScope = PresetScope{
		Param:   "pt",
		Allowed: []string{Preset3M, Preset6M, PresetYear},
		Default: Preset6M,
	}

	PresetScopes = []PresetScope{SinglePeriodScope, TrendScope}
)

func (s PresetScope) Resolve(raw string) string {
	if slices.Contains(s.Allowed, raw) {
		return raw
	}
	return s.Default
}

var monthShortEs = [...]string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"}

type Period struct {
	Route string
	Scope PresetScope

	Presets   map[string]string
	Preset    string
	Anchor    time.Time
	From, To  time.Time
	Months    int
	Label     string
	Currency  currency.Currency
	PrevQuery string
	NextQuery string

	Drill string

	HideCurrency bool
}

func CurrentMonth(now time.Time) time.Time {
	n := now.In(constants.ArgentinaZone)
	return time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, constants.ArgentinaZone)
}

func NewPeriod(route string, scope PresetScope, presets map[string]string,
	anchor, currentMonth time.Time, cur currency.Currency) Period {
	preset := presets[scope.Param]
	months := presetMonths[preset]
	from := anchor.AddDate(0, -(months - 1), 0)

	p := Period{
		Route:    route,
		Scope:    scope,
		Presets:  presets,
		Preset:   preset,
		Anchor:   anchor,
		From:     from,
		To:       anchor.AddDate(0, 1, 0).Add(-time.Nanosecond),
		Months:   months,
		Label:    periodLabel(from, anchor, months),
		Currency: cur,
	}

	p.PrevQuery = periodQuery(route, presets, anchor.AddDate(0, -months, 0), cur)
	if next := anchor.AddDate(0, months, 0); !next.After(currentMonth) {
		p.NextQuery = periodQuery(route, presets, next, cur)
	}
	return p
}

func (p Period) Query() string { return periodQuery(p.Route, p.Presets, p.Anchor, p.Currency) }

func (p Period) WithDrill(suffix string) Period {
	p.Drill = suffix
	if p.PrevQuery != "" {
		p.PrevQuery += suffix
	}
	if p.NextQuery != "" {
		p.NextQuery += suffix
	}
	return p
}

func (p Period) WithPreset(preset string) string {

	next := maps.Clone(p.Presets)
	next[p.Scope.Param] = preset
	return periodQuery(p.Route, next, p.Anchor, p.Currency) + p.Drill
}

func (p Period) WithCurrency(cur currency.Currency) string {
	return periodQuery(p.Route, p.Presets, p.Anchor, cur) + p.Drill
}

func (p Period) IsPreset(preset string) bool { return p.Preset == preset }

func (p Period) AnchorKey() string { return p.Anchor.Format(anchorLayout) }

func (p Period) MonthKeys() []string {
	keys := make([]string, p.Months)
	for i := range keys {
		keys[i] = p.From.AddDate(0, i, 0).Format(anchorLayout)
	}
	return keys
}

func PresetLabel(preset string) string {
	switch preset {
	case PresetMonth:
		return "Mes"
	case Preset3M:
		return "3M"
	case Preset6M:
		return "6M"
	default:
		return "Año"
	}
}

func ShortMonth(key string) string {
	t, err := time.Parse(anchorLayout, key)
	if err != nil {
		return key
	}
	return monthShortEs[t.Month()-1]
}

func ShortDay(key string) string {
	t, err := time.Parse(dayLayout, key)
	if err != nil {
		return key
	}
	return strconv.Itoa(t.Day())
}

func periodLabel(from, anchor time.Time, months int) string {
	var s string
	switch {
	case months == 1:
		s = fmt.Sprintf("%s %d", constants.MonthLongEs[anchor.Month()-1], anchor.Year())
	case from.Year() == anchor.Year():
		s = fmt.Sprintf("%s – %s %d", monthShortEs[from.Month()-1], monthShortEs[anchor.Month()-1], anchor.Year())
	default:
		s = fmt.Sprintf("%s %d – %s %d",
			monthShortEs[from.Month()-1], from.Year(), monthShortEs[anchor.Month()-1], anchor.Year())
	}
	return upperFirst(s)
}

func upperFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func periodQuery(route string, presets map[string]string, anchor time.Time, cur currency.Currency) string {
	v := url.Values{}

	for param, preset := range presets {
		v.Set(param, preset)
	}
	v.Set("m", anchor.Format(anchorLayout))
	v.Set("c", cur.String())
	return route + "?" + v.Encode()
}
