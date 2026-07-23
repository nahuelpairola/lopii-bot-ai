//go:build llm_eval

package orchestrator

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestRouterHistoryEval corre el router contra TODOS los mensajes reales de la
// tabla intent_events (exportada a CSV), con throttle para no reventar el
// TPM/RPM de Groq. NO es un pass/fail estricto: los intents del CSV son la
// salida del router VIEJO, no ground-truth. Sirve como DIFF — dónde el router
// actual diverge de esa salida histórica. Algunos diffs son mejoras buscadas
// (ahora UNCLEAR); otros, regresiones a cazar.
//
// Run:
//
//	GROQ_API_KEY=... GROQ_BASE_URL=https://api.groq.com/openai/v1 \
//	GROQ_ROUTER_MODEL=openai/gpt-oss-20b \
//	ROUTER_EVAL_CSV=../../intent_events_rows.csv \
//	go test -tags llm_eval ./internal/orchestrator/ -run TestRouterHistoryEval -v -timeout 30m
//
// Env opcionales:
//   - ROUTER_EVAL_RPM   (default 15): requests por minuto. ~1.8k tok/call ⇒
//     15 RPM ≈ 27k TPM, bajo el límite típico de gpt-oss-20b. Bajalo si ves 429
//     (el client ya reintenta 429 con backoff, así que picos se auto-curan).
//   - ROUTER_EVAL_LIMIT (default 0 = todos): cap de mensajes únicos, para probar
//     un subconjunto antes de quemar quota.
//   - ROUTER_EVAL_MIN_AGREEMENT (default 0 = solo reporta): si se setea (ej. 0.8),
//     el test falla si el acuerdo cae por debajo.
func TestRouterHistoryEval(t *testing.T) {
	key := os.Getenv("GROQ_API_KEY")
	if key == "" {
		t.Skip("GROQ_API_KEY unset — real-LLM history eval skipped")
	}

	csvPath := os.Getenv("ROUTER_EVAL_CSV")
	if csvPath == "" {
		csvPath = "../../intent_events_rows.csv"
	}
	cases, err := loadHistoryCases(csvPath)
	if err != nil {
		t.Fatalf("load history CSV %q: %v", csvPath, err)
	}
	if len(cases) == 0 {
		t.Fatalf("no usable rows in %q", csvPath)
	}

	if limit := envInt("ROUTER_EVAL_LIMIT", 0); limit > 0 && limit < len(cases) {
		cases = cases[:limit]
	}
	rpm := envInt("ROUTER_EVAL_RPM", 15)
	if rpm < 1 {
		rpm = 1
	}
	interval := time.Minute / time.Duration(rpm)

	o := New(Config{
		APIKey:         key,
		BaseURL:        os.Getenv("GROQ_BASE_URL"),
		RouterModel:    os.Getenv("GROQ_ROUTER_MODEL"),
		TimeoutSeconds: 30,
	})

	t.Logf("history eval: %d mensajes únicos, %d RPM (1 cada %s)", len(cases), rpm, interval)

	var matches, diffs, errs int
	confusion := map[string]int{} // "EXPECTED -> GOT" -> count
	var diffLines []string

	for i, c := range cases {
		if i > 0 {
			time.Sleep(interval)
		}
		res, cerr := o.ClassifyIntent(context.Background(), c.msg)
		if cerr != nil {
			errs++
			t.Logf("[ERR] %q: %v", truncate(c.msg), cerr)
			continue
		}
		got := string(res.Intent)
		if got == c.expected {
			matches++
			continue
		}
		diffs++
		key := c.expected + " -> " + got
		confusion[key]++
		diffLines = append(diffLines, fmt.Sprintf("  %-14s -> %-14s | %s", c.expected, got, truncate(c.msg)))
	}

	scored := matches + diffs
	agreement := 0.0
	if scored > 0 {
		agreement = float64(matches) / float64(scored)
	}

	t.Logf("=== RESUMEN ===")
	t.Logf("clasificados: %d | acuerdo con histórico: %d (%.1f%%) | diffs: %d | errores: %d",
		scored, matches, agreement*100, diffs, errs)

	if len(confusion) > 0 {
		t.Logf("=== CONFUSIÓN (histórico -> nuevo) ===")
		for _, line := range sortedCounts(confusion) {
			t.Logf("  %s", line)
		}
	}
	if len(diffLines) > 0 {
		t.Logf("=== DIFFS (revisar a ojo) ===")
		for _, l := range diffLines {
			t.Log(l)
		}
	}

	if min := envFloat("ROUTER_EVAL_MIN_AGREEMENT", 0); min > 0 && agreement < min {
		t.Errorf("acuerdo %.1f%% por debajo del mínimo %.1f%%", agreement*100, min*100)
	}
}

type historyCase struct {
	msg      string
	expected string // intent histórico normalizado al vocabulario actual
}

// loadHistoryCases lee raw_message + intent del export de intent_events,
// deduplica por mensaje (menos llamadas = menos quota) y normaliza los labels
// viejos. Usa encoding/csv para respetar comas dentro de campos citados.
func loadHistoryCases(path string) ([]historyCase, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // tolera filas con conteo irregular

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	msgCol, intentCol := colIndex(header, "raw_message"), colIndex(header, "intent")
	if msgCol < 0 || intentCol < 0 {
		return nil, fmt.Errorf("faltan columnas raw_message/intent en el header: %v", header)
	}

	seen := map[string]bool{}
	var cases []historyCase
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // fila corrupta: saltar, no abortar
		}
		if msgCol >= len(rec) || intentCol >= len(rec) {
			continue
		}
		msg := strings.TrimSpace(rec[msgCol])
		if msg == "" || seen[msg] {
			continue
		}
		seen[msg] = true
		cases = append(cases, historyCase{msg: msg, expected: normalizeHistoricIntent(rec[intentCol])})
	}
	return cases, nil
}

// normalizeHistoricIntent mapea labels viejos al vocabulario actual. Un label
// desconocido se devuelve tal cual (nunca matcheará → aparece en los diffs).
func normalizeHistoricIntent(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if s == "ACCOUNT_CREATE" {
		return string(IntentAccountManage)
	}
	return s
}

func colIndex(header []string, name string) int {
	for i, h := range header {
		if strings.EqualFold(strings.TrimSpace(h), name) {
			return i
		}
	}
	return -1
}

func sortedCounts(m map[string]int) []string {
	lines := make([]string, 0, len(m))
	for k, v := range m {
		lines = append(lines, fmt.Sprintf("%4d  %s", v, k))
	}
	sort.Sort(sort.Reverse(sort.StringSlice(lines)))
	return lines
}

func truncate(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len([]rune(s)) > 60 {
		return string([]rune(s)[:57]) + "..."
	}
	return s
}

func envInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envFloat(name string, def float64) float64 {
	if v := os.Getenv(name); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
