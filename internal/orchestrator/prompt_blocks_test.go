package orchestrator

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

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

var seedRowRe = regexp.MustCompile(`\(NULL, '([^']*)', '([^']*)',\s*'([^']*)', TRUE`)

var prunedNameRe = regexp.MustCompile(`^\s*'([^']*)',?\s*$`)

const globalSeedEpoch = "20260710130000_reseed_global_subcategories.sql"

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
	_, pruned := seededTaxonomy(t)
	block := buildTaxonomyBlock(pruned)

	if got := len([]rune(block)); got > 5000 {
		t.Errorf("taxonomy block = %d runes, want <= 5000 — the unpruned block was 5764", got)
	}
}
