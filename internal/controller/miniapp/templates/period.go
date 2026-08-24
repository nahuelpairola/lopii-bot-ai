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

// Preset* are the selectable window lengths. The value is what travels in the
// "p" query param.
const (
	PresetMonth = "month"
	Preset3M    = "3m"
	Preset6M    = "6m"
	PresetYear  = "year"
)

// anchorLayout is how the "m" param encodes the anchor month. dayLayout is the
// daily bucket key SumForUser's GroupByDay produces.
const (
	anchorLayout = "2006-01"
	dayLayout    = "2006-01-02"
)

// presetMonths is each preset's window length in months.
var presetMonths = map[string]int{PresetMonth: 1, Preset3M: 3, Preset6M: 6, PresetYear: 12}

// A PresetScope is one independent preset memory: the query param it travels
// in, the presets it offers and the one it falls back to. Two views in the same
// scope share a preset; two views in different scopes never do.
type PresetScope struct {
	Param   string
	Allowed []string
	Default string
}

var (
	// SinglePeriodScope is every view whose window the user reads as "this
	// period": Resumen, Categorías, Cuentas and both leaves. TrendScope is
	// Evolución alone, and it drops "Mes" because a one-month window would
	// leave that matrix with a single column.
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

	// PresetScopes is every scope, and every link plus #app-state carry ALL of
	// them. A link that dropped the scope its own view does not use would erase
	// the other view's memory on the next tab tap — see miniapp/AGENTS.md.
	PresetScopes = []PresetScope{SinglePeriodScope, TrendScope}
)

// Resolve maps a raw query value to a preset this scope offers. Nothing 400s:
// the params travel between views whose scopes differ, so anything
// unrecognized falls back to this scope's default.
func (s PresetScope) Resolve(raw string) string {
	if slices.Contains(s.Allowed, raw) {
		return raw
	}
	return s.Default
}

var monthShortEs = [...]string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"}

// Period is the window every view renders against, plus the links its header
// controls point at. Built by NewPeriod; the handlers only parse query params.
type Period struct {
	Route string      // the view this period belongs to, so links stay absolute
	Scope PresetScope // which slot this view reads and writes
	// Presets is param -> preset for EVERY scope, already resolved. It is what
	// periodQuery writes and what #app-state renders, so the scope this view
	// does not use survives the round trip untouched.
	Presets   map[string]string
	Preset    string    // == Presets[Scope.Param]: the one this view renders against
	Anchor    time.Time // first instant of the anchor month, ART
	From, To  time.Time
	Months    int
	Label     string
	Currency  currency.Currency
	PrevQuery string
	NextQuery string // "" when the anchor is already the current month
	// Drill is the already-encoded suffix of the leaf being viewed
	// ("&account=12"). The HEADER links carry it; Query() does not, because
	// Query() is the link back out.
	Drill string
	// HideCurrency drops the ARS/USD chips. Set by the account leaf: an account
	// holds one currency, so the chip would leave an ARS leaf rendering a USD
	// period. The subcategory leaf keeps them — a category does span both.
	HideCurrency bool
}

// CurrentMonth returns the first instant of now's month on the Argentine wall
// clock. Computing this in UTC would roll the month over three hours early.
func CurrentMonth(now time.Time) time.Time {
	n := now.In(constants.ArgentinaZone)
	return time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, constants.ArgentinaZone)
}

// NewPeriod builds the window of `presets[scope.Param]` months ending at
// `anchor` (inclusive). currentMonth is passed in rather than read from the
// clock so the math is testable.
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
	// The cursor steps by the window length, so consecutive windows do not
	// overlap. Stepping forward past the current month is not offered.
	p.PrevQuery = periodQuery(route, presets, anchor.AddDate(0, -months, 0), cur)
	if next := anchor.AddDate(0, months, 0); !next.After(currentMonth) {
		p.NextQuery = periodQuery(route, presets, next, cur)
	}
	return p
}

// Query is the current state as a link, for callers that append their own
// params on top of it.
func (p Period) Query() string { return periodQuery(p.Route, p.Presets, p.Anchor, p.Currency) }

// WithDrill points the period controls at the leaf we are inside. Without it,
// tapping a chip or an arrow in a leaf lands on the index: periodQuery only
// knows about the preset scopes plus m/c, and the drill rides as its own param.
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
	// Sobre una copia: Period es un valor pero su map no, y sin clonar un chip
	// pisaría el estado del render que lo está dibujando.
	next := maps.Clone(p.Presets)
	next[p.Scope.Param] = preset
	return periodQuery(p.Route, next, p.Anchor, p.Currency) + p.Drill
}

func (p Period) WithCurrency(cur currency.Currency) string {
	return periodQuery(p.Route, p.Presets, p.Anchor, cur) + p.Drill
}

func (p Period) IsPreset(preset string) bool { return p.Preset == preset }

// AnchorKey is the anchor as the "m" param carries it.
func (p Period) AnchorKey() string { return p.Anchor.Format(anchorLayout) }

// MonthKeys returns the window's months as "YYYY-MM", oldest first — the same
// label SumForUser's month grouping produces.
func (p Period) MonthKeys() []string {
	keys := make([]string, p.Months)
	for i := range keys {
		keys[i] = p.From.AddDate(0, i, 0).Format(anchorLayout)
	}
	return keys
}

// PresetLabel is the chip text for a preset.
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

// ShortMonth renders a "YYYY-MM" key as a column header ("jul").
func ShortMonth(key string) string {
	t, err := time.Parse(anchorLayout, key)
	if err != nil {
		return key
	}
	return monthShortEs[t.Month()-1]
}

// ShortDay renders a "YYYY-MM-DD" key as a column header ("7"). Just the day
// number: the period header already says which month you are looking at, and a
// one-character label is what lets a month's worth of columns stay horizontal.
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

// upperFirst pone en mayúscula sólo la primera letra. En castellano los meses
// van en minúscula: la mayúscula acá es porque el label abre su propia línea,
// no porque un mes sea nombre propio — por eso "Feb – jul" y no "Feb – Jul".
//
// Sobre runas y no s[:1]: hoy todos los meses arrancan con ASCII, así que las
// dos versiones andan, pero ésta no depende de que eso siga siendo cierto.
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
	// Todos los ámbitos, no sólo el de esta vista: el que no usamos tiene que
	// llegar entero a la vista que sí lo usa.
	for param, preset := range presets {
		v.Set(param, preset)
	}
	v.Set("m", anchor.Format(anchorLayout))
	v.Set("c", cur.String())
	return route + "?" + v.Encode()
}
