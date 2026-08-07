package quote

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const listBody = `[
  {"casa":"oficial","compra":1465,"venta":1515,"fecha":"2026-08-04"},
  {"casa":"blue","compra":1525,"venta":1545,"fecha":"2026-08-04"},
  {"casa":"oficial","compra":1470,"venta":1520,"fecha":"2026-08-05"}
]`

const todayBody = `[
  {"moneda":"USD","casa":"oficial","nombre":"Oficial","compra":1470,"venta":1520,
   "fechaActualizacion":"2026-08-07T01:30:00.000Z"}
]`

const cpiBody = `[{"fecha":"2026-05-31","valor":2.1},{"fecha":"2026-06-30","valor":-0.9}]`

func TestFetchAll_DecodesEveryRow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(listBody))
	}))
	defer srv.Close()

	qs, err := NewClient(Config{ArgentinaDatosURL: srv.URL, TimeoutSeconds: 5}).FetchAll()
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(qs) != 3 {
		t.Fatalf("expected 3 quotes, got %d", len(qs))
	}
	if qs[0].RateType != "oficial" || qs[0].Ask.String() != "1515" {
		t.Errorf("bad first quote: %+v", qs[0])
	}
	if got := qs[0].Date.Format("2006-01-02"); got != "2026-08-04" {
		t.Errorf("date = %s, want 2026-08-04", got)
	}
}

func TestFetchDate_404ReturnsNoRowsNoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"Not found"}`))
	}))
	defer srv.Close()

	qs, err := NewClient(Config{ArgentinaDatosURL: srv.URL, TimeoutSeconds: 5}).
		FetchDate(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("404 must not be an error, got %v", err)
	}
	if qs != nil {
		t.Errorf("expected no quotes, got %v", qs)
	}
}

func TestFetchDate_BuildsThePathFromTheDate(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`[{"casa":"oficial","compra":1,"venta":2,"fecha":"2026-08-04"}]`))
	}))
	defer srv.Close()

	_, err := NewClient(Config{ArgentinaDatosURL: srv.URL, TimeoutSeconds: 5}).
		FetchDate(time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FetchDate: %v", err)
	}
	if gotPath != "/v1/cotizaciones/dolares/2026/08/04" {
		t.Errorf("path = %s", gotPath)
	}
}

func TestFetchToday_StampsTheARTCalendarDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(todayBody))
	}))
	defer srv.Close()

	// 2026-08-07T01:30Z es todavía el 2026-08-06 en ART. Sin la conversión,
	// la fila entra con la fecha de mañana.
	qs, err := NewClient(Config{DolarAPIURL: srv.URL, TimeoutSeconds: 5}).
		FetchToday(time.Date(2026, 8, 7, 1, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FetchToday: %v", err)
	}
	if len(qs) != 1 {
		t.Fatalf("expected 1 quote, got %d", len(qs))
	}
	if got := qs[0].Date.Format("2006-01-02"); got != "2026-08-06" {
		t.Errorf("date = %s, want 2026-08-06 (ART)", got)
	}
}

func TestFetchCPI_MonthIsFirstOfMonthAndValueMayBeNegative(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(cpiBody))
	}))
	defer srv.Close()

	cs, err := NewClient(Config{ArgentinaDatosURL: srv.URL, TimeoutSeconds: 5}).FetchCPI()
	if err != nil {
		t.Fatalf("FetchCPI: %v", err)
	}
	if len(cs) != 2 {
		t.Fatalf("expected 2 months, got %d", len(cs))
	}
	if got := cs[0].Month.Format("2006-01-02"); got != "2026-05-01" {
		t.Errorf("month = %s, want 2026-05-01", got)
	}
	if cs[1].Value.String() != "-0.9" {
		t.Errorf("value = %s, want -0.9", cs[1].Value.String())
	}
}

func TestFetch_ServerErrorIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := NewClient(Config{ArgentinaDatosURL: srv.URL, TimeoutSeconds: 5}).FetchAll(); err == nil {
		t.Error("expected an error on HTTP 500")
	}
}
