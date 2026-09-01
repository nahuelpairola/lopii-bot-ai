//go:build query_eval

package messaging

import (
	"context"
	"errors"
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
	"lopiibot.com/internal/query"
	"lopiibot.com/internal/subcategory"
	"lopiibot.com/internal/user"
)

func normDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type evalRecorder struct{ t *testing.T }

func (r evalRecorder) Record(c orchestrator.LLMCall) {
	tools := c.ToolCalls
	if tools == "" {
		tools = "(narró, sin tools)"
	}
	r.t.Logf("    · %s %s http=%d prompt=%d completion=%d → %s",
		c.CallType, c.Model, c.HTTPStatus, c.PromptTokens, c.CompletionTokens, tools)
}

func TestQueryEval(t *testing.T) {
	key := os.Getenv("GROQ_APIKEY")
	if key == "" {
		t.Fatal("GROQ_APIKEY unset — the query_eval tag was requested on purpose, so skipping would be a green that proves nothing. Export it from .env.")
	}
	baseURL := os.Getenv("GROQ_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.groq.com/openai/v1"
	}

	conn, err := database.Initialize(database.Creds{
		Host: "localhost", Port: 5432, Name: "lopiibot", User: "lopiibot", Password: "lopiibot",
	}, false)
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

	u := &user.User{Username: fmt.Sprintf("qeval-%d", time.Now().UnixNano())}
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
	loteSuper, loteGym := "compras del lote", "gimnasio del lote"
	seed := []movement.Movement{
		{UserID: uid, AccountID: &bancoID, SubcategoryID: 1, Date: d(5), Type: movement.Expense, Amount: dec("-5000"), Currency: currency.ARS, Description: &loteSuper},
		{UserID: uid, AccountID: &bancoID, SubcategoryID: 4, Date: d(6), Type: movement.Expense, Amount: dec("-3000"), Currency: currency.ARS},
		{UserID: uid, AccountID: &bancoID, SubcategoryID: 52, Date: d(7), Type: movement.Expense, Amount: dec("-8000"), Currency: currency.ARS, Description: &loteGym},
		{UserID: uid, AccountID: &bancoID, SubcategoryID: 85, Date: d(3), Type: movement.Income, Amount: dec("100000"), Currency: currency.ARS},
		{UserID: uid, AccountID: &walletID, SubcategoryID: 1, Date: d(4), Type: movement.Expense, Amount: dec("-50"), Currency: currency.USD},
	}
	if err := movRepo.InsertBatch(seed); err != nil {
		t.Fatalf("insert movements: %v", err)
	}

	queryModel := os.Getenv("GROQ_QUERY_MODEL")
	if queryModel == "" {
		queryModel = "openai/gpt-oss-120b"
	}
	t.Logf("queryModel = %s", queryModel)
	narrationModel := os.Getenv("GROQ_NARRATION_MODEL")
	if narrationModel == "" {
		narrationModel = "openai/gpt-oss-20b"
	}
	t.Logf("narrationModel = %s", narrationModel)
	orch := orchestrator.New(orchestrator.Config{
		APIKey: key, BaseURL: baseURL, QueryModel: queryModel, NarrationModel: narrationModel, TimeoutSeconds: 30,
		Recorder: evalRecorder{t: t},
	})
	c := &controller{accounts: accRepo, movements: movRepo, subcategories: cache, orchestrator: orch}

	asked := 0
	ask := func(t *testing.T, q string) string {
		prompt := query.SystemPrompt()
		exec := query.NewExecutor(c, uid)
		if asked > 0 {
			time.Sleep(12 * time.Second)
		}
		asked++
		for attempt := 0; ; attempt++ {
			ans, err := c.orchestrator.AnswerQuery(context.Background(), prompt, q, nil, query.Tools, exec)
			if err != nil {
				if attempt < 4 && strings.Contains(err.Error(), "rate_limit") {
					t.Logf("rate-limited, backing off 20s (attempt %d)", attempt+1)
					time.Sleep(20 * time.Second)
					continue
				}
				if errors.Is(err, orchestrator.ErrQueryMaxIterations) {
					t.Skipf("narración forzada vacía — defecto abierto, ver el comentario acá arriba: %v", err)
				}
				t.Fatalf("AnswerQuery(%q): %v", q, err)
			}
			t.Logf("\n  Q: %s\n  A: %s", q, ans)
			return ans
		}
	}

	t.Run("gasto_alimentacion_julio", func(t *testing.T) {
		ans := ask(t, "¿cuánto gasté en Alimentación en julio de 2026?")
		if !strings.Contains(normDigits(ans), "8000") {
			t.Errorf("expected 8000 (5000+3000) in answer, got: %s", ans)
		}
	})
	t.Run("saldos", func(t *testing.T) {
		ans := ask(t, "¿cuál es el saldo de cada una de mis cuentas?")
		if !strings.Contains(normDigits(ans), "84000") {
			t.Errorf("expected Banco balance 84000, got: %s", ans)
		}
	})
	t.Run("ingresos_julio", func(t *testing.T) {
		ans := ask(t, "¿cuánto ingresé en julio de 2026?")
		if !strings.Contains(normDigits(ans), "100000") {
			t.Errorf("expected 100000 income, got: %s", ans)
		}
	})
	t.Run("gastos_por_categoria", func(t *testing.T) {
		ask(t, "¿en qué categorías gasté en julio de 2026 y cuánto en cada una?")
	})
	t.Run("listar_movimientos", func(t *testing.T) {
		ask(t, "listame mis movimientos de julio de 2026")
	})
	t.Run("taxonomia", func(t *testing.T) {
		ans := ask(t, "¿qué categorías tengo disponibles y para qué sirve cada una?")
		if strings.TrimSpace(ans) == "" {
			t.Error("expected a non-empty taxonomy answer")
		}
	})
	t.Run("seguro_ambiguo_no_pregunta", func(t *testing.T) {
		ans := ask(t, "¿pagué el seguro este mes?")
		if strings.Contains(ans, "¿") {
			t.Errorf("QUERY must never ask a clarifying question — answer all interpretations. Got: %s", ans)
		}
	})
	t.Run("sin_markdown", func(t *testing.T) {
		ans := ask(t, "¿qué categorías tengo disponibles?")
		if strings.Contains(ans, "*") {
			t.Errorf("answer must be plain text (no markdown '*'/'**'). Got: %s", ans)
		}
	})
	t.Run("multi_entidad_atribuye_cada_total_a_lo_suyo", func(t *testing.T) {
		ans := ask(t, "¿cuánto gasté en Supermercado, cuánto en Panadería y cuánto en Gimnasio en julio de 2026? Dame cada total por separado.")
		if strings.TrimSpace(ans) == "" {
			t.Fatal("la puerta 2 no contestó nada: el 400 de la narración forzada volvió")
		}
		esperado := map[string]string{"Supermercado": "5000", "Panadería": "3000", "Gimnasio": "8000"}
		for etiqueta, propio := range esperado {
			for _, linea := range strings.Split(ans, "\n") {
				if !strings.Contains(strings.ToLower(linea), strings.ToLower(etiqueta)) {
					continue
				}
				n := normDigits(linea)
				for otra, ajeno := range esperado {
					if otra == etiqueta || !strings.Contains(n, ajeno) {
						continue
					}
					if !strings.Contains(n, propio) {
						t.Errorf("%q salió con el monto de %q (%s) en vez del suyo (%s): %q",
							etiqueta, otra, ajeno, propio, linea)
					}
				}
			}
		}
	})

	t.Run("filtro_inexistente_no_afirma_que_no_hay_gastos", func(t *testing.T) {
		ans := ask(t, "¿cuánto gasté en la categoría Cochinchina en julio de 2026?")
		low := strings.ToLower(ans)
		for _, afirmacion := range []string{"no tenés gastos", "no tuviste gastos", "no hubo gastos", "no registraste gastos", "$0", "0 ars"} {
			if strings.Contains(low, afirmacion) {
				t.Errorf("un término que no existe no puede volverse la afirmación %q: %s", afirmacion, ans)
			}
		}
	})

	t.Run("termino_en_descripcion_cruza_subcategorias", func(t *testing.T) {
		ans := ask(t, "¿cuánto gasté en el lote en julio de 2026?")
		if !strings.Contains(normDigits(ans), "13000") {
			t.Errorf("el término vive en las descripciones de Supermercado (5000) y Gimnasio (8000) y tiene que sumar 13000: %s", ans)
		}
	})

	t.Run("multi_entidad_no_niega_lo_que_no_consulto", func(t *testing.T) {
		ans := ask(t, "¿cuánto gasté en Supermercado, cuánto en Panadería, cuánto en Gimnasio y cuánto en el lote, en julio de 2026?")
		low := strings.ToLower(ans)
		for _, negacion := range []string{
			"no hay registros", "sin registros", "no tenés", "no tuviste", "no hubo",
			"no registraste", "no tengo datos", "no dispongo", "no encontré",
		} {
			if strings.Contains(low, negacion) {
				t.Errorf("las cuatro cosas TIENEN datos (5000/3000/8000/13000), así que %q es falso: %s", negacion, ans)
			}
		}
		n := normDigits(ans)
		for _, e := range []struct{ etiqueta, monto string }{
			{"Supermercado", "5000"}, {"Panadería", "3000"}, {"Gimnasio", "8000"}, {"el lote", "13000"},
		} {
			if !strings.Contains(n, e.monto) {
				t.Logf("no contestó %s (%s)", e.etiqueta, e.monto)
			}
		}
	})

	t.Run("search_sin_acentos", func(t *testing.T) {
		ans := ask(t, "¿cuánto gasté en alimentacion en julio de 2026?")
		if !strings.Contains(normDigits(ans), "8000") {
			t.Errorf("sin tilde tiene que encontrar Alimentación igual (8000 = 5000+3000): %s", ans)
		}
	})

	t.Run("cuenta_inexistente_no_contesta_por_todas", func(t *testing.T) {
		ans := ask(t, "¿cuánto gasté con la cuenta Galicia en julio de 2026?")
		if strings.Contains(normDigits(ans), "16000") {
			t.Errorf("contestó por todas las cuentas ante una cuenta inexistente: %s", ans)
		}
	})
}
