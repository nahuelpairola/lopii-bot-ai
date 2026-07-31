package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestClassifyCreate_SingleMovement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		if !strings.Contains(req.Messages[0].Content, "Alimentación") {
			t.Error("expected the taxonomy block to be included in the system prompt")
		}
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"movements\":[{\"type\":\"expense\",\"amount\":\"3000\",\"currency\":\"ARS\",\"category\":\"Alimentación\",\"subcategory\":\"Café\",\"payment_method\":\"cash\",\"description\":\"Café\",\"date\":\"2026-07-02\"}]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, CreateModel: "test-model", TimeoutSeconds: 5})
	taxonomy := []TaxonomyEntry{{Category: "Alimentación", Subcategory: "Café", Description: "Cafeterías"}}

	result, err := o.ClassifyCreate(context.Background(), "café 3000 efectivo", taxonomy, nil, "2026-07-02")
	if err != nil {
		t.Fatalf("ClassifyCreate: %v", err)
	}
	if len(result.Movements) != 1 {
		t.Fatalf("got %d movements, want 1", len(result.Movements))
	}
	if result.Movements[0].Amount != "3000" {
		t.Errorf("amount = %q, want 3000", result.Movements[0].Amount)
	}
}

func TestClassifyCreate_PromptIncludesTransferAndFXPatterns(t *testing.T) {
	var captured string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		captured = req.Messages[0].Content
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"movements\":[{\"type\":\"expense\",\"amount\":\"1\",\"currency\":\"ARS\",\"category\":\"Otros\",\"subcategory\":\"Otros\",\"payment_method\":\"cash\",\"description\":\"x\",\"date\":\"2026-07-07\"}]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, CreateModel: "test-model", TimeoutSeconds: 5})
	if _, err := o.ClassifyCreate(context.Background(), "pasé 50 mil del banco a mp", nil, nil, "2026-07-07"); err != nil {
		t.Fatalf("ClassifyCreate: %v", err)
	}

	for _, want := range []string{
		"Transferencia entre cuentas propias",
		"Sistema | Transferencia",
		"Inversiones | Dólares",
		"Compra/venta de USD",
	} {
		if !strings.Contains(captured, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}

func TestClassifyCreate_CompoundUSDPurchase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"movements\":[{\"type\":\"transfer\",\"amount\":\"-150000\",\"currency\":\"ARS\",\"account_id\":3,\"category\":\"Inversiones\",\"subcategory\":\"Dólares\",\"payment_method\":\"transfer\",\"description\":\"Compra 100 USD a 1500\",\"date\":\"2026-07-07\"},{\"type\":\"transfer\",\"amount\":\"100\",\"currency\":\"USD\",\"account_id\":5,\"category\":\"Inversiones\",\"subcategory\":\"Dólares\",\"payment_method\":\"transfer\",\"description\":\"Compra 100 USD a 1500\",\"date\":\"2026-07-07\"}]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, CreateModel: "test-model", TimeoutSeconds: 5})
	result, err := o.ClassifyCreate(context.Background(), "compré 100 usd a 1500", nil, nil, "2026-07-07")
	if err != nil {
		t.Fatalf("ClassifyCreate: %v", err)
	}
	if len(result.Movements) != 2 {
		t.Fatalf("got %d movements, want 2 (ARS transfer + USD transfer)", len(result.Movements))
	}
	for i, m := range result.Movements {
		if m.Type != "transfer" {
			t.Errorf("movement %d type = %q, want transfer (no expense leg anymore)", i, m.Type)
		}
		if m.AccountID == nil {
			t.Errorf("movement %d has nil account_id, want both legs attributed", i)
		}
		if m.Subcategory != "Dólares" {
			t.Errorf("movement %d subcategory = %q, want Dólares", i, m.Subcategory)
		}
	}
}

func TestClassifyCreate_ErrorsOnEmptyMovements(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"movements\":[]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, CreateModel: "test-model", TimeoutSeconds: 5})
	if _, err := o.ClassifyCreate(context.Background(), "hola", nil, nil, "2026-07-02"); err == nil {
		t.Fatal("expected an error when the result has no movements")
	}
}

func TestCreateTool_OptionalFieldsAllowNull(t *testing.T) {
	// Groq validates tool-call arguments against our JSON schema server-side
	// (see flexBool comment) — gpt-oss-20b emits explicit null for optional
	// properties it doesn't fill in (same behavior already handled for
	// mentioned_date_from/to in update.go/delete.go). account_id,
	// account_name_guess and group are optional (not in "required") but were
	// declared single-type, so a real user with real accounts still 400s:
	// "expected integer, but got null" / "expected string, but got null".
	// Regression for a live 400 observed 2026-07-13 ("gaste 500 pesos en el
	// super", user had accounts, model still emitted null for all three).
	var schema struct {
		Properties struct {
			Movements struct {
				Items struct {
					Properties map[string]struct {
						Type json.RawMessage `json:"type"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"movements"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(createTool.Parameters, &schema); err != nil {
		t.Fatalf("parse createTool schema: %v", err)
	}
	for _, field := range []string{"account_id", "account_name_guess", "group"} {
		prop, ok := schema.Properties.Movements.Items.Properties[field]
		if !ok {
			t.Fatalf("schema missing property %q", field)
		}
		if !strings.Contains(string(prop.Type), `"null"`) {
			t.Errorf("%s.type = %s, want it to include \"null\" (optional field, model emits explicit null when unset)", field, prop.Type)
		}
	}
}

func TestCreatePrompt_ContainsNewRules(t *testing.T) {
	var captured string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		captured = req.Messages[0].Content
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"movements\":[{\"type\":\"expense\",\"amount\":\"500\",\"currency\":\"ARS\",\"category\":\"x\",\"subcategory\":\"y\",\"payment_method\":\"cash\",\"description\":\"z\",\"date\":\"2026-07-07\"}]}"}}]}}]}`))
	}))
	defer server.Close()
	o := New(Config{BaseURL: server.URL, CreateModel: "m", TimeoutSeconds: 5})
	_, _ = o.ClassifyCreate(context.Background(), "gasté 500", nil, nil, "2026-07-07")
	for _, want := range []string{"el destino decide el tipo", "Rendimiento inversión", "campo group", "van en POSITIVO"} {
		if !strings.Contains(captured, want) {
			t.Errorf("create prompt missing %q", want)
		}
	}
}

func TestBuildTaxonomyBlock_OmitsEmptyDescription(t *testing.T) {
	block := buildTaxonomyBlock([]TaxonomyEntry{
		{Category: "Comida", Subcategory: "Supermercado", Description: "Compra grande. NO incluye carnicería."},
		{Category: "Ingresos", Subcategory: "Sueldo"},
	})

	if !strings.Contains(block, "Comida | Supermercado | Compra grande. NO incluye carnicería.") {
		t.Errorf("a described pair must keep its note:\n%s", block)
	}
	if !strings.Contains(block, "Ingresos | Sueldo\n") && !strings.HasSuffix(block, "Ingresos | Sueldo") {
		t.Errorf("an undescribed pair must render without a trailing separator:\n%s", block)
	}
	if strings.Contains(block, "Sueldo | ") {
		t.Errorf("no dangling separator allowed:\n%s", block)
	}
}

// seedRowRe matches one seeded global subcategory in the reseed migration:
// (NULL, 'Categoría', 'Subcategoría', 'Descripción', TRUE, '🍔').
var seedRowRe = regexp.MustCompile(`\(NULL, '([^']*)', '([^']*)',\s*'([^']*)', TRUE`)

// prunedNameRe matches one quoted subcategory name inside the prune migration's
// IN list. It over-matches by design: every quoted literal in that file is a
// subcategory name, and a name that is not in the seed simply never fires.
var prunedNameRe = regexp.MustCompile(`^\s*'([^']*)',?\s*$`)

// globalSeedEpoch is the reseed that soft-deletes every earlier global
// subcategory and writes today's set. Migrations older than it seeded rows that
// no longer exist, so the taxonomy is "this file plus everything after it".
const globalSeedEpoch = "20260710130000_reseed_global_subcategories.sql"

// seededTaxonomy rebuilds the taxonomy the CREATE prompt actually carries, by
// reading the migrations the database runs: the reseed epoch, every later
// migration that inserts a global pair (Sistema | Ajuste de saldo arrived that
// way), and the prune that blanks the descriptions which do not disambiguate.
// Reading the real files is the point — a hand-copied literal here would drift
// from the seed silently, which is exactly the regression this guards against.
//
// It returns both states: `full` is what the prompt carried before the pruning
// migration, `pruned` is what it carries now. The pruning eval needs both to
// compare what the model picks with each block; the size guard only needs the
// second.
func seededTaxonomy(t *testing.T) (full, pruned []TaxonomyEntry) {
	t.Helper()

	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.sql"))
	if err != nil {
		t.Fatalf("globbing migrations: %v", err)
	}
	sort.Strings(files)

	blanked := make(map[string]bool)
	var entries []TaxonomyEntry

	for _, f := range files {
		base := filepath.Base(f)
		if base < globalSeedEpoch {
			continue
		}
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", base, err)
		}

		for _, m := range seedRowRe.FindAllStringSubmatch(string(body), -1) {
			entries = append(entries, TaxonomyEntry{Category: m[1], Subcategory: m[2], Description: m[3]})
		}

		if !strings.Contains(base, "prune_global_subcategory_descriptions") {
			continue
		}
		for _, line := range strings.Split(string(body), "\n") {
			if m := prunedNameRe.FindStringSubmatch(line); m != nil {
				blanked[m[1]] = true
			}
		}
	}

	if len(entries) == 0 {
		t.Fatal("no seeded subcategories parsed — the migration's INSERT shape changed")
	}
	if len(blanked) == 0 {
		t.Fatal("no pruned names parsed — the prune migration's IN list shape changed")
	}

	pruned = make([]TaxonomyEntry, len(entries))
	copy(pruned, entries)
	for i := range pruned {
		if blanked[pruned[i].Subcategory] {
			pruned[i].Description = ""
		}
	}
	return entries, pruned
}

func TestBuildTaxonomyBlock_PrunedBlockStaysSmall(t *testing.T) {
	// Guards the pruning migration: if someone re-adds prose to the global seed,
	// the CREATE prompt silently gets more expensive on every single call. The
	// number is the measured post-pruning size with headroom, not a target to
	// optimise against — pruning further means deleting contrast notes, which
	// degrades categorization instead of just costing tokens.
	_, pruned := seededTaxonomy(t)
	block := buildTaxonomyBlock(pruned)

	if got := len([]rune(block)); got > 4600 {
		t.Errorf("taxonomy block = %d runes, want <= 4600 after pruning", got)
	}
}
