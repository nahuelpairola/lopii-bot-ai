//go:build llm_eval

package orchestrator

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"
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
// expect is the pair the pruned block MUST still produce. It exists to halve
// the cost: the differential only needs the full block when the pruned one
// already missed. On a match the note was not load-bearing and one call
// settles it; on a miss the second call says whether the note was what carried
// the answer, or whether the model gets this wrong either way.
//
// An empty expect falls back to a pure differential (both calls, always) — for
// messages where there is no single obviously-right pair.
var pruningEvalMessages = []struct{ id, msg, expect string }{
	// Ingresos
	{"sueldo", "me depositaron el sueldo, 900 mil", ""},
	{"changa", "cobré una changa de 150 mil", ""},
	{"reintegro", "me reintegraron 20 mil de la prepaga", ""},
	// Vivienda
	{"alquiler", "pagué el alquiler 450 mil", ""},
	{"expensas", "las expensas salieron 90 mil", ""},
	{"seguro_hogar", "pagué el seguro del depto 30 mil", ""},
	// Transporte
	{"seguro_auto", "el seguro del auto 45 mil", ""},
	{"peaje", "gasté 3 mil en peajes", ""},
	// Salud
	{"consulta", "fui al médico, 25 mil la consulta", ""},
	{"analisis", "me hice análisis de sangre 18 mil", ""},
	{"prepaga", "pagué la prepaga 120 mil", ""},
	{"dentista", "fui al dentista 60 mil", ""},
	{"anteojos", "compré anteojos 80 mil", ""},
	{"terapia", "sesión de terapia 30 mil", ""},
	// Ocio y salidas
	{"cine", "entradas al cine 12 mil", ""},
	{"vacaciones", "pagué el hotel de las vacaciones 300 mil", ""},
	// Bienestar
	{"gimnasio", "la cuota del gimnasio 25 mil", ""},
	// Indumentaria / Tecnología / Educación
	{"mochila", "me compré una mochila 45 mil", ""},
	{"notebook", "compré una notebook 900 mil", ""},
	{"libro", "compré un libro 20 mil", ""},
	// Mascotas / Deudas
	{"veterinario", "llevé el perro al veterinario 35 mil", ""},
	{"prestamo", "pagué la cuota del préstamo 80 mil", "Deudas / préstamos | Cuota préstamo"},
	// Inversiones / Finanzas / Otros
	{"plazofijo", "hice un plazo fijo de 500 mil", "Inversiones | Plazo fijo"},
	{"cripto", "compré 200 mil en bitcoin", "Inversiones | Cripto"},
	{"dolares", "compré 100 dólares", "Inversiones | Dólares"},
	{"comisiones", "me cobraron 5 mil de mantenimiento de cuenta", "Finanzas | Cargos / comisiones bancarias"},
	{"regalo", "le regalé 50 mil a mi hermana", "Otros | Regalos / donaciones"},
	// Controls: notes KEPT, and only these notes make the pick unambiguous.
	{"sube", "cargué la SUBE 10 mil", "Transporte | Transporte público"},
	{"edenor", "pagué Edenor 40 mil", "Vivienda | Luz"},
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
			// The pruned block is what ships, so it is what gets asked first.
			// A rate limit is Fatal for THIS subtest only: it means "no answer
			// yet", not "the pruning is fine". Leaving it as a retryable failure
			// is the whole point of the subtest split.
			afterRes, err := o.ClassifyCreate(context.Background(), tc.msg, pruned, accounts, today)
			if err != nil {
				t.Fatalf("%q with the pruned block: %v", tc.msg, err)
			}
			after := firstPair(afterRes)

			// One call settles it when the pruned block already lands on the
			// pair the note was supposed to protect. Half the corpus costs half
			// the tokens, and the daily budget is the binding constraint.
			if tc.expect != "" && after == tc.expect {
				t.Logf("%q → %s (sin la nota, en una llamada)", tc.msg, after)
				return
			}

			pace(t)

			beforeRes, err := o.ClassifyCreate(context.Background(), tc.msg, full, accounts, today)
			if err != nil {
				t.Fatalf("%q with the full block: %v", tc.msg, err)
			}
			before := firstPair(beforeRes)

			switch {
			case before == after:
				// Both blocks agree. If that disagrees with expect, the model is
				// simply wrong here and the description was never the reason —
				// not a pruning regression.
				t.Logf("%q → %s con los dos bloques (expect %q: la nota no era la causa)", tc.msg, after, tc.expect)
			default:
				t.Errorf("%q changed:\n  full   → %s\n  pruned → %s", tc.msg, before, after)
			}
		})
	}
}

// pace sleeps between the two calls of a subtest when PRUNING_EVAL_PACE_SECONDS
// is set. Groq's binding limit is tokens per DAY, and the window refills in a
// trickle: asking for two ~4k calls back to back is what makes a request bounce
// while the live bot's single call gets through. Spacing them lets the eval feed
// on the trickle instead of competing with a human waiting on a reply.
func pace(t *testing.T) {
	t.Helper()
	secs, err := strconv.Atoi(os.Getenv("PRUNING_EVAL_PACE_SECONDS"))
	if err != nil || secs <= 0 {
		return
	}
	t.Logf("pace: esperando %ds antes de la segunda llamada", secs)
	time.Sleep(time.Duration(secs) * time.Second)
}
