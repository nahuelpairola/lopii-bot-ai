// Package grafana holds the admin Grafana dashboard and a linter for it.
//
// The dashboard is configuration, not code, so nothing here runs in
// production. The test exists because a Grafana macro bug is invisible until
// a human opens the dashboard: $__timeGroupAlias emits its own AS "time", so
// writing `$__timeGroupAlias(created_at,$__interval) AS time` expands to
// `... AS "time" AS time` — a Postgres syntax error. That shipped and broke
// all 8 timeseries panels without a single failing check anywhere.
//
// The file is Grafana dashboard schema V2 (elements + layout), which is what
// Grafana 13 emits and accepts. V1 (panels[] + gridPos) is rejected outright
// by this instance, so the linter reads V2 only.
package grafana

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"
)

const dashboardPath = "admin-dashboard.json"

// dsVariable is the only datasource reference an element may use. A literal
// UID is an environment-specific id inside a versioned file.
const dsVariable = "${DS_POSTGRES}"

// gridWidth is Grafana's fixed column count.
const gridWidth = 24

// dashboard is the subset of schema V2 this linter asserts on. Unknown fields
// are ignored by encoding/json, so the real file can carry far more.
type dashboard struct {
	Title    string             `json:"title"`
	Elements map[string]element `json:"elements"`
	Layout   layout             `json:"layout"`
}

type element struct {
	Kind string      `json:"kind"`
	Spec elementSpec `json:"spec"`
}

type elementSpec struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Data      dataGroup `json:"data"`
	VizConfig vizConfig `json:"vizConfig"`
}

type vizConfig struct {
	Group string `json:"group"`
}

type dataGroup struct {
	Spec struct {
		Queries []panelQuery `json:"queries"`
	} `json:"spec"`
}

type panelQuery struct {
	Spec struct {
		RefID string `json:"refId"`
		Query struct {
			Datasource struct {
				Name string `json:"name"`
			} `json:"datasource"`
			Spec struct {
				RawSQL string `json:"rawSql"`
			} `json:"spec"`
		} `json:"query"`
	} `json:"spec"`
}

type layout struct {
	Kind string `json:"kind"`
	Spec struct {
		Rows []row `json:"rows"`
	} `json:"spec"`
}

type row struct {
	Spec struct {
		Title  string `json:"title"`
		Layout struct {
			Spec struct {
				Items []gridItem `json:"items"`
			} `json:"spec"`
		} `json:"layout"`
	} `json:"spec"`
}

type gridItem struct {
	Spec struct {
		X       int `json:"x"`
		Y       int `json:"y"`
		Width   int `json:"width"`
		Height  int `json:"height"`
		Element struct {
			Name string `json:"name"`
		} `json:"element"`
	} `json:"spec"`
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

func TestDashboardParses(t *testing.T) {
	d := loadDashboard(t)
	if d.Title == "" {
		t.Error("dashboard has no title")
	}
	if len(d.Elements) == 0 {
		t.Fatal("dashboard has no elements")
	}
	if d.Layout.Kind != "RowsLayout" {
		t.Errorf("layout kind = %q, want RowsLayout", d.Layout.Kind)
	}
}

// badAlias matches the shipped bug: $__timeGroupAlias already emits AS "time",
// so a following `AS time` produces `AS "time" AS time`.
var badAlias = regexp.MustCompile(`(?i)\$__timeGroupAlias\([^)]*\)\s*AS\s+"?time"?`)

// plainGroup matches $__timeGroup( but not $__timeGroupAlias( — the literal
// paren is what separates them.
var (
	plainGroup      = regexp.MustCompile(`(?i)\$__timeGroup\(`)
	plainGroupAlias = regexp.MustCompile(`(?i)\$__timeGroup\([^)]*\)\s+AS\s+"?time"?`)
)

// TestBadAliasDetectsTheShippedBug pins the detector against the exact query
// that shipped broken, so the regex cannot be loosened into uselessness by a
// later edit. This is the only test here that does not read the dashboard.
func TestBadAliasDetectsTheShippedBug(t *testing.T) {
	shipped := `SELECT $__timeGroupAlias(created_at,$__interval) AS time, call_type, count(*) AS value FROM llm_calls WHERE $__timeFilter(created_at) GROUP BY 1, call_type ORDER BY 1`
	if !badAlias.MatchString(shipped) {
		t.Error("badAlias no longer detects the query that shipped broken")
	}

	// The two correct forms must stay clean.
	fixedCount := `SELECT $__timeGroupAlias(received_at,$__interval,0), count(*) AS "updates" FROM request_traces`
	if badAlias.MatchString(fixedCount) {
		t.Errorf("badAlias false-positives on the correct fill-zero form: %s", fixedCount)
	}
	if plainGroup.MatchString(fixedCount) {
		t.Error("plainGroup must not match $__timeGroupAlias(")
	}

	fixedLatency := `SELECT $__timeGroup(received_at,$__interval) AS time, max(latency_ms) AS "app máx" FROM request_traces`
	if !plainGroupAlias.MatchString(fixedLatency) {
		t.Errorf("plainGroupAlias should accept the correct bare-macro form: %s", fixedLatency)
	}
}

func TestNoDoubleTimeAlias(t *testing.T) {
	for name, el := range loadDashboard(t).Elements {
		for _, q := range el.Spec.Data.Spec.Queries {
			if badAlias.MatchString(q.Spec.Query.Spec.RawSQL) {
				t.Errorf("%s (%q) target %s: $__timeGroupAlias already emits AS \"time\"; drop the trailing AS time\n  %s",
					name, el.Spec.Title, q.Spec.RefID, q.Spec.Query.Spec.RawSQL)
			}
		}
	}
}

func TestPlainTimeGroupHasAlias(t *testing.T) {
	for name, el := range loadDashboard(t).Elements {
		for _, q := range el.Spec.Data.Spec.Queries {
			sql := q.Spec.Query.Spec.RawSQL
			if plainGroup.MatchString(sql) && !plainGroupAlias.MatchString(sql) {
				t.Errorf("%s (%q) target %s: $__timeGroup emits no alias; it needs a trailing AS time\n  %s",
					name, el.Spec.Title, q.Spec.RefID, sql)
			}
		}
	}
}

func TestDatasourceIsVariable(t *testing.T) {
	for name, el := range loadDashboard(t).Elements {
		if len(el.Spec.Data.Spec.Queries) == 0 {
			t.Errorf("%s (%q) has no queries", name, el.Spec.Title)
			continue
		}
		for _, q := range el.Spec.Data.Spec.Queries {
			if q.Spec.Query.Spec.RawSQL == "" {
				t.Errorf("%s (%q) target %s has an empty rawSql", name, el.Spec.Title, q.Spec.RefID)
			}
			if got := q.Spec.Query.Datasource.Name; got != dsVariable {
				t.Errorf("%s (%q) target %s must use datasource %s, got %q",
					name, el.Spec.Title, q.Spec.RefID, dsVariable, got)
			}
		}
	}
}

// TestLayoutReferencesEveryElement catches the failure mode unique to schema
// V2: elements and layout are separate, so a panel can exist with nothing
// placing it on screen, or the layout can point at a name that does not exist.
// Neither is a JSON error — both are an invisible panel.
func TestLayoutReferencesEveryElement(t *testing.T) {
	d := loadDashboard(t)

	placed := map[string]bool{}
	for _, r := range d.Layout.Spec.Rows {
		for _, it := range r.Spec.Layout.Spec.Items {
			name := it.Spec.Element.Name
			if _, ok := d.Elements[name]; !ok {
				t.Errorf("row %q places %q, which is not in elements", r.Spec.Title, name)
			}
			if placed[name] {
				t.Errorf("%q is placed more than once", name)
			}
			placed[name] = true
		}
	}
	for name := range d.Elements {
		if !placed[name] {
			t.Errorf("%q exists in elements but no layout item places it", name)
		}
	}
}

func TestGridGeometrySane(t *testing.T) {
	for _, r := range loadDashboard(t).Layout.Spec.Rows {
		for _, it := range r.Spec.Layout.Spec.Items {
			s := it.Spec
			if s.Width <= 0 || s.Height <= 0 {
				t.Errorf("row %q: %q has zero size (w=%d h=%d)", r.Spec.Title, s.Element.Name, s.Width, s.Height)
			}
			if s.X+s.Width > gridWidth {
				t.Errorf("row %q: %q overflows the %d-column grid (x=%d w=%d)",
					r.Spec.Title, s.Element.Name, gridWidth, s.X, s.Width)
			}
			if s.X < 0 || s.Y < 0 {
				t.Errorf("row %q: %q has a negative position (x=%d y=%d)", r.Spec.Title, s.Element.Name, s.X, s.Y)
			}
		}
	}
}
