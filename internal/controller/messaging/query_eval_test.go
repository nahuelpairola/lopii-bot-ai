//go:build query_eval

package messaging

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/database"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
	"lopiibot.com/internal/user"
)

// End-to-end QUERY test against REAL Groq + the local Docker Postgres. It
// seeds a fresh throwaway user with a known, clean movement set, runs the real
// agent loop (real tool calls, real SQL, real narration), prints every answer,
// asserts the clearest numbers, then hard-deletes everything it created.
//
// Run:
//   set -a; . ./.env; set +a
//   go test -tags query_eval ./internal/controller/messaging/ -run TestQueryEval -v
// Excluded from the default `go test ./...` (no tag) — needs a live DB + key.

// normDigits keeps only the digit runs of a string, so a numeric assertion
// survives whatever thousands separator or (unicode) space the model narrated
// with -- "8 000", "8.000", "$8,000" all normalize to a stream containing "8000".
func normDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestQueryEval(t *testing.T) {
	key := os.Getenv("GROQ_APIKEY")
	if key == "" {
		t.Skip("GROQ_APIKEY unset — real-LLM eval skipped")
	}
	baseURL := os.Getenv("GROQ_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.groq.com/openai/v1"
	}

	conn, err := database.Initialize(database.Creds{
		Host: "localhost", Port: 5432, Name: "lopiibot", User: "lopiibot", Password: "lopiibot",
	})
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}

	userRepo := user.NewRepository(conn)
	accRepo := account.NewRepository(conn)
	movRepo := movement.InitRepository(conn)
	subRepo := subcategory.NewRepository(conn)
	cache, err := subcategory.NewCache(subRepo)
	if err != nil {
		t.Fatalf("subcategory cache: %v", err)
	}

	// --- seed a throwaway user with a clean, known dataset ---
	u := &user.User{TelegramID: fmt.Sprintf("qeval-%d", time.Now().UnixNano())}
	if err := userRepo.Insert(u); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	uid := u.ID
	t.Cleanup(func() {
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&movement.Movement{})
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&account.Account{})
		conn.DB.Unscoped().Where("id = ?", uid).Delete(&user.User{})
	})

	banco := &account.Account{UserID: uid, Name: "Banco Test", Type: account.StandardType, Currency: currency.ARS, IsDefault: true}
	if err := accRepo.Insert(banco); err != nil {
		t.Fatalf("insert banco: %v", err)
	}
	wallet := &account.Account{UserID: uid, Name: "Wallet Test", Type: account.StandardType, Currency: currency.USD, IsDefault: true}
	if err := accRepo.Insert(wallet); err != nil {
		t.Fatalf("insert wallet: %v", err)
	}
	bancoID, walletID := uint64(banco.ID), uint64(wallet.ID)

	d := func(day int) time.Time { return time.Date(2026, 7, day, 12, 0, 0, 0, time.UTC) }
	dec := func(s string) decimal.Decimal { return decimal.RequireFromString(s) }
	// Subcategory ids from the seeded global taxonomy (verified in DB):
	// 1=Alimentación|Supermercado, 4=Alimentación|Panadería,
	// 52=Bienestar|Gimnasio, 85=Ingresos|Freelance.
	seed := []movement.Movement{
		{UserID: uid, AccountID: &bancoID, SubcategoryID: 1, Date: d(5), Type: movement.Expense, Amount: dec("-5000"), Currency: currency.ARS},
		{UserID: uid, AccountID: &bancoID, SubcategoryID: 4, Date: d(6), Type: movement.Expense, Amount: dec("-3000"), Currency: currency.ARS},
		{UserID: uid, AccountID: &bancoID, SubcategoryID: 52, Date: d(7), Type: movement.Expense, Amount: dec("-8000"), Currency: currency.ARS},
		{UserID: uid, AccountID: &bancoID, SubcategoryID: 85, Date: d(3), Type: movement.Income, Amount: dec("100000"), Currency: currency.ARS},
		{UserID: uid, AccountID: &walletID, SubcategoryID: 1, Date: d(4), Type: movement.Expense, Amount: dec("-50"), Currency: currency.USD},
	}
	if err := movRepo.InsertBatch(seed); err != nil {
		t.Fatalf("insert movements: %v", err)
	}
	// Expected (July 2026, ARS, transfers excluded): Alimentación=8000,
	// Bienestar=8000, income=100000, Banco balance=84000, Wallet=-50 USD.

	queryModel := os.Getenv("GROQ_QUERY_MODEL")
	if queryModel == "" {
		queryModel = "openai/gpt-oss-120b"
	}
	t.Logf("queryModel = %s", queryModel)
	orch := orchestrator.New(orchestrator.Config{
		APIKey: key, BaseURL: baseURL, QueryModel: queryModel, TimeoutSeconds: 30,
	})
	c := &controller{accounts: accRepo, movements: movRepo, subcategories: cache, orchestrator: orch}

	// 8b-instant free tier is 6000 TPM and each request is ~2.3k tokens, so
	// pace the calls and back off on a 429 — otherwise the test self-throttles.
	asked := 0
	ask := func(q string) string {
		prompt := c.buildQuerySystemPrompt()
		exec := c.buildQueryExecutor(uid)
		if asked > 0 {
			time.Sleep(12 * time.Second)
		}
		asked++
		for attempt := 0; ; attempt++ {
			ans, err := c.orchestrator.AnswerQuery(context.Background(), prompt, q, queryTools, exec)
			if err != nil {
				if attempt < 4 && strings.Contains(err.Error(), "rate_limit") {
					t.Logf("rate-limited, backing off 20s (attempt %d)", attempt+1)
					time.Sleep(20 * time.Second)
					continue
				}
				t.Fatalf("AnswerQuery(%q): %v", q, err)
			}
			t.Logf("\n  Q: %s\n  A: %s", q, ans)
			return ans
		}
	}

	t.Run("gasto_alimentacion_julio", func(t *testing.T) {
		ans := ask("¿cuánto gasté en Alimentación en julio de 2026?")
		if !strings.Contains(normDigits(ans), "8000") {
			t.Errorf("expected 8000 (5000+3000) in answer, got: %s", ans)
		}
	})
	t.Run("saldos", func(t *testing.T) {
		ans := ask("¿cuál es el saldo de cada una de mis cuentas?")
		if !strings.Contains(normDigits(ans), "84000") {
			t.Errorf("expected Banco balance 84000, got: %s", ans)
		}
	})
	t.Run("ingresos_julio", func(t *testing.T) {
		ans := ask("¿cuánto ingresé en julio de 2026?")
		if !strings.Contains(normDigits(ans), "100000") {
			t.Errorf("expected 100000 income, got: %s", ans)
		}
	})
	// The remaining cases are printed for inspection (LLM phrasing varies too
	// much for a strict assert); they exercise group_by, list, and taxonomy.
	t.Run("gastos_por_categoria", func(t *testing.T) {
		ask("¿en qué categorías gasté en julio de 2026 y cuánto en cada una?")
	})
	t.Run("listar_movimientos", func(t *testing.T) {
		ask("listame mis movimientos de julio de 2026")
	})
	t.Run("taxonomia", func(t *testing.T) {
		ans := ask("¿qué categorías tengo disponibles y para qué sirve cada una?")
		if strings.TrimSpace(ans) == "" {
			t.Error("expected a non-empty taxonomy answer")
		}
	})
	t.Run("seguro_ambiguo_no_pregunta", func(t *testing.T) {
		ans := ask("¿pagué el seguro este mes?")
		if strings.Contains(ans, "¿") {
			t.Errorf("QUERY must never ask a clarifying question — answer all interpretations. Got: %s", ans)
		}
	})
	t.Run("sin_markdown", func(t *testing.T) {
		ans := ask("¿qué categorías tengo disponibles?")
		if strings.Contains(ans, "*") {
			t.Errorf("answer must be plain text (no markdown '*'/'**'). Got: %s", ans)
		}
	})
}
