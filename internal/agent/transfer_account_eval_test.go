//go:build llm_eval

package agent

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
)

func transferEvalKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("GROQ_APIKEY")
	if key == "" {
		t.Fatal("GROQ_APIKEY unset — the llm_eval tag was requested on purpose, so skipping would be a green that proves nothing. Export it from .env.")
	}
	return key
}

func transferEvalRuns() int {
	if n, err := strconv.Atoi(os.Getenv("EVAL_RUNS")); err == nil && n > 0 {
		return n
	}
	return 3
}

func accountsByCurrency(rows []movement.MovementRow, accounts []account.Account) map[string][]string {
	out := map[string][]string{}
	for _, r := range rows {
		id := r.AccountID
		if id == "" {
			id = defaultAccountID(accounts, r.Currency)
		}
		out[r.Currency] = append(out[r.Currency], id)
	}
	for cur := range out {
		slices.Sort(out[cur])
	}
	return out
}

func TestTransferAccountEval(t *testing.T) {
	key := transferEvalKey(t)
	model := os.Getenv("GROQ_AGENT_MODEL")
	if model == "" {
		model = "openai/gpt-oss-20b"
	}
	baseURL := os.Getenv("GROQ_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.groq.com/openai/v1"
	}
	o := orchestrator.New(orchestrator.Config{APIKey: key, BaseURL: baseURL, AgentModel: model, TimeoutSeconds: 60})

	accs := accountsUser2()
	options := make([]orchestrator.AccountOption, 0, len(accs))
	for _, a := range accs {
		options = append(options, orchestrator.AccountOption{ID: uint64(a.ID), Name: a.Name, Currency: a.Currency.String()})
	}
	tools := wiredAgentTools()
	prompt := orchestrator.BuildAgentPrompt("jueves 2026-10-29", options, nil, "", tools, "")
	fciMorning := []orchestrator.QueryTurn{
		{Question: "Sueldo Epsilon 3571267", Answer: "Listo, anoté el ingreso de $3.571.267 en Mercado Pago."},
		{Question: "Transferencia de 1200000 a fci", Answer: "Listo, pasé $1.200.000 de Mercado Pago a FCI."},
	}

	cases := []struct {
		id      string
		msg     string
		history []orchestrator.QueryTurn
		want    map[string][]string
	}{
		{"compra_usd_sin_cuenta_va_a_la_default", "Compra de 1129,48 usd por 1586 pesos cada dólar", fciMorning,
			map[string][]string{"ARS": {"43"}, "USD": {"44"}}},
		{"transferencia_usd_a_fci_nombrado", "Transferencia de 1129,48 usd a fci", fciMorning,
			map[string][]string{"USD": {"44", "46"}}},
		{"compra_usd_en_efectivo_nombrado", "compré 100 usd en efectivo a 1500", nil,
			map[string][]string{"ARS": {"43"}, "USD": {"48"}}},
	}

	calls := 0
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			runs, modelWrong := transferEvalRuns(), 0
			for run := 0; run < runs; run++ {
				if calls > 0 {
					time.Sleep(62 * time.Second)
				}
				calls++
				var recorded *orchestrator.CreateResult
				execute := func(name string, args json.RawMessage) (string, error) {
					if name != orchestrator.ToolRecordMovements {
						return "pendiente: la app se encarga", nil
					}
					var r orchestrator.CreateResult
					if err := json.Unmarshal(args, &r); err != nil {
						return "", err
					}
					recorded = &r
					return "", orchestrator.ErrAgentTurnDone
				}
				if _, err := o.Run(context.Background(), prompt, tc.msg, tc.history, tools, execute); err != nil && recorded == nil {
					t.Errorf("run %d: Run: %v", run, err)
					continue
				}
				if recorded == nil {
					t.Errorf("run %d: the model never called record_movements", run)
					continue
				}

				var modelIDs []string
				for _, m := range recorded.Movements {
					id := "nil"
					if m.AccountID != nil {
						id = strconv.FormatUint(*m.AccountID, 10)
					}
					modelIDs = append(modelIDs, m.Currency+":"+id)
				}
				if strings.Contains(strings.Join(modelIDs, ","), "USD:46") && tc.id == "compra_usd_sin_cuenta_va_a_la_default" {
					modelWrong++
				}

				data := buildCreateSeed(*recorded, nil, accs, tc.msg)
				got := accountsByCurrency(movement.DecodeMovementRows(data), accs)
				gaps := conversation.DecodeStringSlice(data, conversation.KeyPendingAccountGaps)
				t.Logf("run %d: model sent %v → app resolved %v (gaps %v)", run, modelIDs, got, gaps)
				for cur, want := range tc.want {
					if !slices.Equal(got[cur], want) {
						t.Errorf("run %d: %s legs = %v, want %v", run, cur, got[cur], want)
					}
				}
				if len(gaps) != 0 {
					t.Errorf("run %d: asked gaps %v, want none", run, gaps)
				}
			}
			t.Logf("%s on %s: model sent FCI USD unasked in %d/%d runs", tc.id, model, modelWrong, runs)
		})
	}
}
