package templates

import (
	"fmt"
	"net/url"
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

// anchorLayout is how the "m" param encodes the anchor month.
const anchorLayout = "2006-01"

// presetMonths is each preset's window length in months.
var presetMonths = map[string]int{PresetMonth: 1, Preset3M: 3, Preset6M: 6, PresetYear: 12}

// AllPresets is what a single-period view offers. TrendPresets drops the
// one-month option: it would leave the evolution with a single column and the
// accounts trend with a single point.
var (
	AllPresets   = []string{PresetMonth, Preset3M, Preset6M, PresetYear}
	TrendPresets = []string{Preset3M, Preset6M, PresetYear}
)

var monthShortEs = [...]string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"}
var monthLongEs = [...]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}

// Period is the window every view renders against, plus the links its header
// controls point at. Built by NewPeriod; the handlers only parse query params.
type Period struct {
	Route     string // the view this period belongs to, so links stay absolute
	Preset    string
	Anchor    time.Time // first instant of the anchor month, ART
	From, To  time.Time
	Months    int
	Label     string
	Currency  currency.Currency
	Allowed   []string
	PrevQuery string
	NextQuery string // "" when the anchor is already the current month
}

// CurrentMonth returns the first instant of now's month on the Argentine wall
// clock. Computing this in UTC would roll the month over three hours early.
func CurrentMonth(now time.Time) time.Time {
	n := now.In(constants.ArgentinaZone)
	return time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, constants.ArgentinaZone)
}

// NewPeriod builds the window of `preset` months ending at `anchor`
// (inclusive). currentMonth is passed in rather than read from the clock so
// the math is testable.
func NewPeriod(route, preset string, anchor, currentMonth time.Time, cur currency.Currency, allowed []string) Period {
	months := presetMonths[preset]
	from := anchor.AddDate(0, -(months - 1), 0)

	p := Period{
		Route:    route,
		Preset:   preset,
		Anchor:   anchor,
		From:     from,
		To:       anchor.AddDate(0, 1, 0).Add(-time.Nanosecond),
		Months:   months,
		Label:    periodLabel(from, anchor, months),
		Currency: cur,
		Allowed:  allowed,
	}
	// The cursor steps by the window length, so consecutive windows do not
	// overlap. Stepping forward past the current month is not offered.
	p.PrevQuery = periodQuery(route, preset, anchor.AddDate(0, -months, 0), cur)
	if next := anchor.AddDate(0, months, 0); !next.After(currentMonth) {
		p.NextQuery = periodQuery(route, preset, next, cur)
	}
	return p
}

// Query is the current state as a link, for callers that append their own
// params on top of it.
func (p Period) Query() string { return periodQuery(p.Route, p.Preset, p.Anchor, p.Currency) }

func (p Period) WithPreset(preset string) string {
	return periodQuery(p.Route, preset, p.Anchor, p.Currency)
}

func (p Period) WithCurrency(cur currency.Currency) string {
	return periodQuery(p.Route, p.Preset, p.Anchor, cur)
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

func periodLabel(from, anchor time.Time, months int) string {
	var s string
	switch {
	case months == 1:
		s = fmt.Sprintf("%s %d", monthLongEs[anchor.Month()-1], anchor.Year())
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

func periodQuery(route, preset string, anchor time.Time, cur currency.Currency) string {
	v := url.Values{}
	v.Set("p", preset)
	v.Set("m", anchor.Format(anchorLayout))
	v.Set("c", cur.String())
	return route + "?" + v.Encode()
}
