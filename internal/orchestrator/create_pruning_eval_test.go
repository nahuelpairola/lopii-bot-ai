//go:build llm_eval

package orchestrator

import (
	"context"
	"fmt"
	"os"
	"testing"
)

// This file is the quality gate for the taxonomy-pruning migration
// (20260730120000_prune_global_subcategory_descriptions.sql).
//
// TestCreateEval could not be that gate: it feeds a hand-written 7-entry
// fixture taxonomy, not the seeded global one, and it only asserts the movement
// TYPE — never the category. Running it before and after the migration returns
// the same answer by construction, because the migration does not touch
// anything it reads.
//
// So the gate has to be differential: same message, same model, two taxonomy
// blocks — the one the prompt carried before pruning and the one it carries
// now — and compare the (category, subcategory) the model picks. No golden
// labels needed; the question is not "is this the right category" but "did
// blanking a description change the answer".
//
// A disagreement is not automatically a bug — the pruned pick is occasionally
// the better one — but every disagreement is a description that was load-bearing
// and must be reviewed by a human before this merges.
//
// Run it with real creds:
//
//	GROQ_API_KEY=... GROQ_BASE_URL=https://api.groq.com/openai/v1 \
//	GROQ_CREATE_MODEL=openai/gpt-oss-20b \
//	go test -tags llm_eval ./internal/orchestrator/ -run TestCreatePruningEval -v -timeout 20m
//
// It costs 2 calls per message on the same daily budget as the live bot.

// pruningEvalMessages targets the pairs whose description the migration blanked
// — that is where a regression can hide. The last two are controls: their notes
// were KEPT, and they only resolve correctly if the note is still readable in
// the pruned block.
var pruningEvalMessages = []string{
	// Ingresos
	"me depositaron el sueldo, 900 mil",
	"cobré una changa de 150 mil",
	"me reintegraron 20 mil de la prepaga",
	// Vivienda
	"pagué el alquiler 450 mil",
	"las expensas salieron 90 mil",
	"pagué el seguro del depto 30 mil",
	// Transporte
	"el seguro del auto 45 mil",
	"gasté 3 mil en peajes",
	// Salud
	"fui al médico, 25 mil la consulta",
	"me hice análisis de sangre 18 mil",
	"pagué la prepaga 120 mil",
	"fui al dentista 60 mil",
	"compré anteojos 80 mil",
	"sesión de terapia 30 mil",
	// Ocio y salidas
	"entradas al cine 12 mil",
	"pagué el hotel de las vacaciones 300 mil",
	// Bienestar
	"la cuota del gimnasio 25 mil",
	// Indumentaria / Tecnología / Educación
	"me compré una mochila 45 mil",
	"compré una notebook 900 mil",
	"compré un libro 20 mil",
	// Mascotas / Deudas
	"llevé el perro al veterinario 35 mil",
	"pagué la cuota del préstamo 80 mil",
	// Inversiones / Finanzas / Otros
	"hice un plazo fijo de 500 mil",
	"me cobraron 5 mil de mantenimiento de cuenta",
	"le regalé 50 mil a mi hermana",
	// Controls: notes KEPT, and only these notes make the pick unambiguous.
	"cargué la SUBE 10 mil",
	"pagué Edenor 40 mil",
}

// firstPair is what the differential compares: the category/subcategory of the
// first movement. Multi-leg results (a dollar purchase, a transfer) still have a
// meaningful first leg, and the pruning cannot change the leg count.
func firstPair(res CreateResult) string {
	if len(res.Movements) == 0 {
		return "(no movements)"
	}
	return fmt.Sprintf("%s | %s", res.Movements[0].Category, res.Movements[0].Subcategory)
}

func TestCreatePruningEval(t *testing.T) {
	key := os.Getenv("GROQ_API_KEY")
	if key == "" {
		t.Skip("GROQ_API_KEY unset — real-LLM eval skipped")
	}

	full, pruned := seededTaxonomy(t)
	t.Logf("taxonomy: %d pairs · full block %d runes · pruned block %d runes",
		len(full), len([]rune(buildTaxonomyBlock(full))), len([]rune(buildTaxonomyBlock(pruned))))

	o := New(Config{
		APIKey:         key,
		BaseURL:        os.Getenv("GROQ_BASE_URL"),
		CreateModel:    os.Getenv("GROQ_CREATE_MODEL"),
		TimeoutSeconds: 30,
	})
	accounts := evalAccounts()
	const today = "2026-07-30"

	agreed := 0
	for _, msg := range pruningEvalMessages {
		beforeRes, err := o.ClassifyCreate(context.Background(), msg, full, accounts, today)
		if err != nil {
			t.Fatalf("%q with the full block: %v", msg, err)
		}
		afterRes, err := o.ClassifyCreate(context.Background(), msg, pruned, accounts, today)
		if err != nil {
			t.Fatalf("%q with the pruned block: %v", msg, err)
		}

		before, after := firstPair(beforeRes), firstPair(afterRes)
		if before == after {
			agreed++
			continue
		}
		// Not t.Fatal: the whole corpus has to run so a human sees every
		// disagreement at once and can judge which blanked note to restore.
		t.Errorf("%q changed:\n  full   → %s\n  pruned → %s", msg, before, after)
	}

	t.Logf("agreement: %d/%d", agreed, len(pruningEvalMessages))
}
