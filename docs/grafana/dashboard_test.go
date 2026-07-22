// Package grafana holds the admin Grafana dashboard and a linter for it.
//
// The dashboard is configuration, not code, so nothing here runs in
// production. The test exists because a Grafana macro bug is invisible until
// a human opens the dashboard: $__timeGroupAlias emits its own AS "time", so
// writing `$__timeGroupAlias(created_at,$__interval) AS time` expands to
// `... AS "time" AS time` — a Postgres syntax error. That shipped and broke
// all 8 timeseries panels without a single failing check anywhere.
package grafana

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"
)

const dashboardPath = "admin-dashboard.json"

// dsVariable is the only datasource reference a panel may use. A literal UID
// is an environment-specific id inside a versioned file.
const dsVariable = "${DS_POSTGRES}"

// dashboard is the subset of the Grafana v1 schema this linter asserts on.
// Unknown fields are ignored by encoding/json, so the real file can carry far
// more than this without breaking the test.
type dashboard struct {
	Title  string  `json:"title"`
	Panels []panel `json:"panels"`
}

type panel struct {
	Type    string   `json:"type"`
	Title   string   `json:"title"`
	GridPos gridPos  `json:"gridPos"`
	Targets []target `json:"targets"`
	// Panels holds the children of a collapsed row panel.
	Panels []panel `json:"panels"`
}

type gridPos struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

type target struct {
	RefID      string      `json:"refId"`
	RawSQL     string      `json:"rawSql"`
	Datasource *datasource `json:"datasource"`
}

type datasource struct {
	UID string `json:"uid"`
}

func loadDashboard(t *testing.T) dashboard {
	t.Helper()
	raw, err := os.ReadFile(dashboardPath)
	if err != nil {
		t.Fatalf("read %s: %v", dashboardPath, err)
	}
	var d dashboard
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("parse %s: %v", dashboardPath, err)
	}
	return d
}

// flatPanels returns every panel, walking into collapsed rows. Row headers
// themselves are dropped: they carry no query and no meaningful gridPos.
func (d dashboard) flatPanels() []panel {
	var out []panel
	for _, p := range d.Panels {
		if p.Type == "row" {
			out = append(out, p.Panels...)
			continue
		}
		out = append(out, p)
	}
	return out
}

func TestDashboardParses(t *testing.T) {
	d := loadDashboard(t)
	if d.Title == "" {
		t.Error("dashboard has no title")
	}
	if got := len(d.flatPanels()); got == 0 {
		t.Fatal("dashboard has no panels")
	}
}

// badAlias matches the shipped bug: $__timeGroupAlias already emits AS "time",
// so a following `AS time` produces `AS "time" AS time`.
var badAlias = regexp.MustCompile(`(?i)\$__timeGroupAlias\([^)]*\)\s*AS\s+"?time"?`)

func TestNoDoubleTimeAlias(t *testing.T) {
	for _, p := range loadDashboard(t).flatPanels() {
		for _, tg := range p.Targets {
			if badAlias.MatchString(tg.RawSQL) {
				t.Errorf("panel %q target %s: $__timeGroupAlias already emits AS \"time\"; drop the trailing AS time\n  %s",
					p.Title, tg.RefID, tg.RawSQL)
			}
		}
	}
}

// plainGroup matches $__timeGroup( but not $__timeGroupAlias( — the trailing
// paren is what separates them.
var (
	plainGroup      = regexp.MustCompile(`(?i)\$__timeGroup\(`)
	plainGroupAlias = regexp.MustCompile(`(?i)\$__timeGroup\([^)]*\)\s+AS\s+"?time"?`)
)

func TestPlainTimeGroupHasAlias(t *testing.T) {
	for _, p := range loadDashboard(t).flatPanels() {
		for _, tg := range p.Targets {
			if plainGroup.MatchString(tg.RawSQL) && !plainGroupAlias.MatchString(tg.RawSQL) {
				t.Errorf("panel %q target %s: $__timeGroup emits no alias; it needs a trailing AS time\n  %s",
					p.Title, tg.RefID, tg.RawSQL)
			}
		}
	}
}

func TestDatasourceIsVariable(t *testing.T) {
	for _, p := range loadDashboard(t).flatPanels() {
		if len(p.Targets) == 0 {
			t.Errorf("panel %q has no targets", p.Title)
			continue
		}
		for _, tg := range p.Targets {
			if tg.RawSQL == "" {
				t.Errorf("panel %q target %s has an empty rawSql", p.Title, tg.RefID)
			}
			if tg.Datasource == nil || tg.Datasource.UID != dsVariable {
				t.Errorf("panel %q target %s must use datasource uid %s, got %+v",
					p.Title, tg.RefID, dsVariable, tg.Datasource)
			}
		}
	}
}

func TestGridPosSane(t *testing.T) {
	for _, p := range loadDashboard(t).flatPanels() {
		g := p.GridPos
		if g.W <= 0 || g.H <= 0 {
			t.Errorf("panel %q has zero-size gridPos %+v", p.Title, g)
		}
		if g.X+g.W > 24 {
			t.Errorf("panel %q overflows the 24-column grid: x=%d w=%d", p.Title, g.X, g.W)
		}
	}
}
