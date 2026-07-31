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
//
// Each entry gets its own subtest, named by `id`, so a run can be resumed one
// message at a time:
//
//	go test ... -run 'TestCreatePruningEval/prestamo'
//
// That matters because the Groq daily budget is the binding constraint: the
// first full run died at message 22 of 27 having spent ~180k of the 200k TPD.
// Re-running the whole corpus to reach the tail would spend it all again, and
// this model is the one production uses for the router and CREATE.
var pruningEvalMessages = []struct{ id, msg string }{
	// Ingresos
	{"sueldo", "me depositaron el sueldo, 900 mil"},
	{"changa", "cobré una changa de 150 mil"},
	{"reintegro", "me reintegraron 20 mil de la prepaga"},
	// Vivienda
	{"alquiler", "pagué el alquiler 450 mil"},
	{"expensas", "las expensas salieron 90 mil"},
	{"seguro_hogar", "pagué el seguro del depto 30 mil"},
	// Transporte
	{"seguro_auto", "el seguro del auto 45 mil"},
	{"peaje", "gasté 3 mil en peajes"},
	// Salud
	{"consulta", "fui al médico, 25 mil la consulta"},
	{"analisis", "me hice análisis de sangre 18 mil"},
	{"prepaga", "pagué la prepaga 120 mil"},
	{"dentista", "fui al dentista 60 mil"},
	{"anteojos", "compré anteojos 80 mil"},
	{"terapia", "sesión de terapia 30 mil"},
	// Ocio y salidas
	{"cine", "entradas al cine 12 mil"},
	{"vacaciones", "pagué el hotel de las vacaciones 300 mil"},
	// Bienestar
	{"gimnasio", "la cuota del gimnasio 25 mil"},
	// Indumentaria / Tecnología / Educación
	{"mochila", "me compré una mochila 45 mil"},
	{"notebook", "compré una notebook 900 mil"},
	{"libro", "compré un libro 20 mil"},
	// Mascotas / Deudas
	{"veterinario", "llevé el perro al veterinario 35 mil"},
	{"prestamo", "pagué la cuota del préstamo 80 mil"},
	// Inversiones / Finanzas / Otros
	{"plazofijo", "hice un plazo fijo de 500 mil"},
	{"cripto", "compré 200 mil en bitcoin"},
	{"dolares", "compré 100 dólares"},
	{"comisiones", "me cobraron 5 mil de mantenimiento de cuenta"},
	{"regalo", "le regalé 50 mil a mi hermana"},
	// Controls: notes KEPT, and only these notes make the pick unambiguous.
	{"sube", "cargué la SUBE 10 mil"},
	{"edenor", "pagué Edenor 40 mil"},
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

	for _, tc := range pruningEvalMessages {
		t.Run(tc.id, func(t *testing.T) {
			beforeRes, err := o.ClassifyCreate(context.Background(), tc.msg, full, accounts, today)
			if err != nil {
				// Fatal for THIS subtest only: a rate limit means "no answer
				// yet", not "the pruning is fine". Leaving it as a failure the
				// driver can retry is the whole point of the subtest split.
				t.Fatalf("%q with the full block: %v", tc.msg, err)
			}
			afterRes, err := o.ClassifyCreate(context.Background(), tc.msg, pruned, accounts, today)
			if err != nil {
				t.Fatalf("%q with the pruned block: %v", tc.msg, err)
			}

			before, after := firstPair(beforeRes), firstPair(afterRes)
			if before != after {
				t.Errorf("%q changed:\n  full   → %s\n  pruned → %s", tc.msg, before, after)
			}
		})
	}
}
