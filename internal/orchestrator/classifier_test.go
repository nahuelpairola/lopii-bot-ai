package orchestrator

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lopiibot.com/internal/constants"
)

func classifierStub(t *testing.T, status int, body string, calls *int) *Orchestrator {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return New(Config{APIKey: "k", BaseURL: srv.URL, ClassifierModel: "m", TimeoutSeconds: 5})
}

var testTaxonomy = []TaxonomyEntry{
	{Category: "Alimentación", Subcategory: "Supermercado"},
	{Category: "Transporte", Subcategory: "Combustible"},
}

// UNA llamada por MENSAJE, no una por movimiento: comparten el mismo texto, y
// separarlas multiplicaría el costo quitándole a cada una el contexto que las
// otras aportan.
func TestClassifyCategories_OneCallForEveryRow(t *testing.T) {
	calls := 0
	o := classifierStub(t, 200, `{"choices":[{"message":{"tool_calls":[{"function":{"name":"classify",
		"arguments":"{\"pairs\":[{\"category\":\"Alimentación\",\"subcategory\":\"Supermercado\"},{\"category\":\"Transporte\",\"subcategory\":\"Combustible\"}]}"}}]}}]}`, &calls)

	got := o.ClassifyCategories(context.Background(), "super 20 mil y nafta 15 mil",
		[]ClassifyRow{{Description: "super"}, {Description: "nafta"}}, testTaxonomy)

	if calls != 1 {
		t.Errorf("llamadas = %d, want 1", calls)
	}
	if len(got) != 2 || got[0].Subcategory != "Supermercado" || got[1].Subcategory != "Combustible" {
		t.Errorf("pares = %+v", got)
	}
}

// Una falla del modelo degrada a PREGUNTA, nunca a un dato inventado. Es el
// mismo camino que el usuario ya ve cuando el modelo duda.
func TestClassifyCategories_FailureDegradesToPendingReview(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"429", 429, `{"error":{"message":"rate limited"}}`},
		{"500", 500, `{}`},
		{"json roto", 200, `{"choices":[{"message":{"tool_calls":[{"function":{"name":"classify","arguments":"no soy json"}}]}}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			o := classifierStub(t, tc.status, tc.body, &calls)

			got := o.ClassifyCategories(context.Background(), "algo",
				[]ClassifyRow{{Description: "x"}, {Description: "y"}}, testTaxonomy)

			if len(got) != 2 {
				t.Fatalf("pares = %d, want 2", len(got))
			}
			for i, p := range got {
				if p.Category != constants.PendingReview {
					t.Errorf("par %d = %+v, want PENDING_REVIEW", i, p)
				}
			}
		})
	}
}

// De menos también es una falla parcial: las filas que faltan preguntan.
func TestClassifyCategories_ShortAnswerPadsWithPendingReview(t *testing.T) {
	calls := 0
	o := classifierStub(t, 200, `{"choices":[{"message":{"tool_calls":[{"function":{"name":"classify",
		"arguments":"{\"pairs\":[{\"category\":\"Alimentación\",\"subcategory\":\"Supermercado\"}]}"}}]}}]}`, &calls)

	got := o.ClassifyCategories(context.Background(), "dos cosas",
		[]ClassifyRow{{Description: "a"}, {Description: "b"}}, testTaxonomy)

	if len(got) != 2 || got[1].Category != constants.PendingReview {
		t.Errorf("pares = %+v: la fila sin respuesta tiene que preguntar", got)
	}
}

// Sin taxonomía no hay contra qué clasificar, y no se gasta una llamada.
func TestClassifyCategories_NoTaxonomyAsksWithoutCallingTheModel(t *testing.T) {
	calls := 0
	o := classifierStub(t, 200, `{}`, &calls)

	got := o.ClassifyCategories(context.Background(), "algo", []ClassifyRow{{Description: "x"}}, nil)

	if calls != 0 {
		t.Errorf("llamó al modelo %d veces sin taxonomía", calls)
	}
	if len(got) != 1 || got[0].Category != constants.PendingReview {
		t.Errorf("pares = %+v", got)
	}
}

// El mensaje original va SIEMPRE: "el asado del domingo con los chicos" dice
// mucho más que "asado", y esa es la razón de mandarlo.
func TestRenderClassifyInput_CarriesTheOriginalMessage(t *testing.T) {
	in := renderClassifyInput("el asado del domingo con los chicos",
		[]ClassifyRow{{Description: "asado", Type: "expense", AccountName: "Galicia"}})

	for _, want := range []string{"el asado del domingo con los chicos", "1. asado", "expense", "Galicia"} {
		if !strings.Contains(in, want) {
			t.Errorf("falta %q en:\n%s", want, in)
		}
	}
}
