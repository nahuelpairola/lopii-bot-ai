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

// resource is the Kubernetes-style envelope Grafana wraps a dashboard in.
// The UI importer validates it before it ever looks at the dashboard, and
// rejects a bare spec body with "Missing property metadata / Missing property
// spec" — which is exactly what this file shipped with from 2026-07-22 to
// 2026-08-13, parsing perfectly the whole time. A linter that only reads the
// spec cannot see that the file is unimportable, so it asserts the envelope
// first now.
type resource struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec dashboard `json:"spec"`
}

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
	var r resource
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("parse %s: %v", dashboardPath, err)
	}
	for _, problem := range envelopeProblems(r) {
		t.Error(problem)
	}
	return r.Spec
}

// envelopeProblems checks the resource envelope. Pure, so the test below can
// feed it the broken shape that actually shipped.
//
// Without these four the UI importer refuses the file outright and never gets
// as far as reporting a panel problem, so every other assertion in this file
// would be checking a dashboard nobody can load.
func envelopeProblems(r resource) []string {
	var out []string
	if r.APIVersion == "" {
		out = append(out, `apiVersion is empty, want a "dashboard.grafana.app/..." version`)
	}
	if r.Kind != "Dashboard" {
		out = append(out, "kind = "+r.Kind+", want Dashboard")
	}
	if r.Metadata.Name == "" {
		out = append(out, "metadata.name is empty: it IS the dashboard uid, and a re-import without it forks a duplicate")
	}
	if len(r.Spec.Elements) == 0 {
		out = append(out, "spec.elements is empty: the dashboard body has to live under spec, not at the root")
	}
	return out
}

// TestEnvelopeDetectsTheShippedBug is the same reassurance as
// TestBadAliasDetectsTheShippedBug: it pins the detector to the shape that
// really broke, so a future tidy-up of envelopeProblems cannot quietly make it
// useless.
//
// From 2026-07-22 to 2026-08-13 the file held the V2 spec body at the ROOT,
// with no envelope. It parsed perfectly, the linter was green, and Grafana
// refused it with "Missing property metadata. Missing property spec." — a file
// that is valid JSON is not a dashboard that imports.
func TestEnvelopeDetectsTheShippedBug(t *testing.T) {
	bare := []byte(`{"title":"x","elements":{"panel-1":{"kind":"Panel"}},"layout":{"kind":"RowsLayout"}}`)
	var r resource
	if err := json.Unmarshal(bare, &r); err != nil {
		t.Fatalf("the broken shape has to still be valid JSON — that was the whole problem: %v", err)
	}
	if len(envelopeProblems(r)) == 0 {
		t.Error("envelopeProblems accepted a bare spec body: the check that would have caught the 3-week bug is dead")
	}
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

// El importador de Grafana valida el JSON contra su schema y rechaza el archivo
// ENTERO por un solo panel mal formado. Los dos chequeos de abajo salieron de
// errores reales del import del 2026-08-13, con el linter en verde:
//
//	vizConfig linea 2029: Missing property "version".
//	linea 2051: Incorrect type. Expected "number".
//
// Los dos eran del mismo panel (104), el único de 28 fuera de convención. Es
// justo lo que un linter tiene que atrapar: no la falla vistosa, la fila que
// quedó distinta cuando el resto se migró.

// rawElements devuelve los paneles como mapas, para los chequeos que miran
// adentro de fieldConfig — tipar todo ese árbol para leer dos campos sería
// arrastrar medio schema de Grafana a este archivo.
func rawElements(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(dashboardPath)
	if err != nil {
		t.Fatalf("read %s: %v", dashboardPath, err)
	}
	var r struct {
		Spec struct {
			Elements map[string]any `json:"elements"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatalf("parse %s: %v", dashboardPath, err)
	}
	if len(r.Spec.Elements) == 0 {
		t.Fatal("no elements under spec")
	}
	return r.Spec.Elements
}

// dig baja por un camino de claves; devuelve nil si alguna falta.
func dig(v any, keys ...string) any {
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

func TestVizConfigDeclaresVersion(t *testing.T) {
	for name, el := range rawElements(t) {
		if dig(el, "spec", "vizConfig") == nil {
			continue // no todo elemento es un panel
		}
		ver, _ := dig(el, "spec", "vizConfig", "version").(string)
		if ver == "" {
			t.Errorf("%s: vizConfig sin \"version\" — el importador rechaza el archivo entero por esto", name)
		}
	}
}

// Un `"value": null` es el escalón base del schema clásico. En V2 el campo es
// numérico y null lo rechaza, así que el base va en 0 — que es lo que usan los
// otros 27 paneles.
func TestThresholdStepsAreNumeric(t *testing.T) {
	var walk func(v any, name string)
	walk = func(v any, name string) {
		switch x := v.(type) {
		case map[string]any:
			if steps, ok := x["steps"].([]any); ok && x["mode"] != nil {
				for i, s := range steps {
					val := dig(s, "value")
					if _, isNum := val.(float64); !isNum {
						t.Errorf("%s: thresholds.steps[%d].value = %v, quiere un número (null rompe el import)", name, i, val)
					}
				}
			}
			for _, vv := range x {
				walk(vv, name)
			}
		case []any:
			for _, vv := range x {
				walk(vv, name)
			}
		}
	}
	for name, el := range rawElements(t) {
		walk(el, name)
	}
}
