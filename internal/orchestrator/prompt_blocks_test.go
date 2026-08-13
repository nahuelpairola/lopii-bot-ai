package orchestrator

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Los tests de buildTaxonomyBlock, rescatados de create_test.go cuando la etapa
// 5 borró ClassifyCreate.
//
// El bloque de taxonomía NO murió con ese camino: lo sigue armando el
// clasificador, que es el único que clasifica ahora. Y ahí pesa MÁS que antes,
// porque el clasificador manda las descripciones completas — son lo único que
// desambigua, ya que la spec descartó los few-shots.

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
	// the CREATE prompt silently gets more expensive on every single call.
	//
	// The ceiling is deliberately loose. It exists to catch a SYSTEMIC regression
	// — the unpruned block is 5764 runes — not to police individual notes. Five
	// pruned pairs are still unverified by TestCreatePruningEval (it ran out of
	// daily Groq quota mid-corpus), and restoring any of them is the CORRECT
	// outcome of that eval, so a threshold hugging today's 4584 would fail on the
	// right fix. Pruning further is not a goal: it means deleting the contrast
	// notes and the Argentine vocabulary that decide the category.
	_, pruned := seededTaxonomy(t)
	block := buildTaxonomyBlock(pruned)

	if got := len([]rune(block)); got > 5000 {
		t.Errorf("taxonomy block = %d runes, want <= 5000 — the unpruned block was 5764", got)
	}
}
