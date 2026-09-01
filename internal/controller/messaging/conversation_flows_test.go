//go:build conv_test

package messaging

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"lopiibot.com/internal/agent"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messages"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
)

func (h *convHarness) subID(category, subcategory string) uint64 {
	h.t.Helper()
	var id uint64
	err := h.conn.DB.Raw(
		`SELECT id FROM subcategories
		 WHERE category = ? AND subcategory = ? AND user_id IS NULL AND deleted_at IS NULL`,
		category, subcategory,
	).Scan(&id).Error
	if err != nil || id == 0 {
		h.t.Fatalf("subID(%q, %q): no está en la taxonomía viva (err=%v)", category, subcategory, err)
	}
	return id
}

func (h *convHarness) SeedMovementOn(description, amount string, subcategoryID uint64, date time.Time) uint {
	h.t.Helper()
	accID := h.accounts["Banco Test"]
	movs := []movement.Movement{{
		UserID: h.userID, AccountID: &accID, SubcategoryID: subcategoryID,
		Date: date, Type: movement.Expense,
		Amount: mustDec(h.t, amount), Currency: currency.ARS,
		Description: &description,
	}}
	if err := movement.InitRepository(h.conn).InsertBatch(movs); err != nil {
		h.t.Fatalf("SeedMovementOn: %v", err)
	}
	return movs[0].ID
}

func (h *convHarness) SeedCorpus() {
	h.t.Helper()
	today := agent.StartOfTodayArgentina()
	super := h.subID("Alimentación", "Supermercado")
	h.SeedMovementOn("Carrefour", "-12700", super, today)
	h.SeedMovementOn("Coto", "-3000", super, today.AddDate(0, 0, -2))
	h.SeedMovementOn("Edesur", "-45000", h.subID("Vivienda", "Luz"), today.AddDate(0, 0, -5))
	h.SeedMovementOn("YPF", "-30000", h.subID("Transporte", "Combustible"), today.AddDate(0, 0, -7))
}

func (h *convHarness) ScriptQuery(fn func(execute func(string, json.RawMessage) (string, error)) (string, error)) {
	h.orc.queryRunFn = fn
}

func (h *convHarness) FlowState() (flow, step string) {
	h.t.Helper()
	var row struct {
		FlowName string
		StepName string
	}
	err := h.conn.DB.Raw(
		"SELECT flow_name, step_name FROM conversation_states WHERE user_id = ?", h.userID,
	).Scan(&row).Error
	if err != nil {
		h.t.Fatalf("FlowState: %v", err)
	}
	return row.FlowName, row.StepName
}

func answerQueryCall() scriptedCall {
	return scriptedCall{orchestrator.ToolAnswerQuery, `{}`}
}

func manageSettingsCall(area string) scriptedCall {
	return scriptedCall{orchestrator.ToolManageSettings, `{"area":"` + area + `"}`}
}

func TestConversation_AnswerQuerySumsRealData(t *testing.T) {
	h := newConversationHarness(t)
	h.SeedCorpus()

	var toolOut string
	h.ScriptQuery(func(execute func(string, json.RawMessage) (string, error)) (string, error) {
		today := agent.StartOfTodayArgentina()
		out, err := execute("sum_movements", json.RawMessage(fmt.Sprintf(
			`{"from":%q,"to":%q,"currency":"ARS","group_by":"category"}`,
			today.AddDate(0, 0, -30).Format("2006-01-02"), today.Format("2006-01-02"))))
		if err != nil {
			return "", err
		}
		toolOut = out
		return "Gastaste esto:\n" + out, nil
	})
	h.ScriptToolCalls(answerQueryCall())

	h.SendText("cuánto gasté este mes por categoría")

	for _, want := range []string{"15700.00", "45000.00", "30000.00"} {
		if !strings.Contains(toolOut, want) {
			t.Errorf("falta %s en el total por categoría:\n%s", want, toolOut)
		}
	}
	if strings.Contains(toolOut, "1000000") {
		t.Errorf("la apertura de la cuenta entró en el total:\n%s", toolOut)
	}
	if !strings.Contains(h.LastMessage(), "Gastaste esto") {
		t.Errorf("la respuesta no llegó al usuario, salió: %q", h.LastMessage())
	}
}

func TestConversation_AnswerQueryReadsAccountBalance(t *testing.T) {
	h := newConversationHarness(t)
	h.SeedCorpus()

	var toolOut string
	h.ScriptQuery(func(execute func(string, json.RawMessage) (string, error)) (string, error) {
		out, err := execute("account_balance", json.RawMessage(`{"account":"Banco Test"}`))
		toolOut = out
		return out, err
	})
	h.ScriptToolCalls(answerQueryCall())

	h.SendText("cuánto tengo en el banco")

	if !strings.Contains(toolOut, "909300") {
		t.Errorf("saldo mal calculado, la tool devolvió:\n%s", toolOut)
	}
}

func TestConversation_ManageSettingsReminder_OpensTheWizard(t *testing.T) {
	h := newConversationHarness(t)
	h.ScriptToolCalls(manageSettingsCall(agent.SettingsAreaReminder))

	h.SendText("quiero que me recuerdes cargar los gastos")

	if fl, _ := h.FlowState(); fl != flow.ReminderSetupFlowName {
		t.Fatalf("flow abierto = %q, want %q. Copia: %v", fl, flow.ReminderSetupFlowName, h.Messages())
	}
}

func TestConversation_ManageSettingsAccount_OpensTheManageFlow(t *testing.T) {
	h := newConversationHarness(t)
	matched := h.accounts["Banco Test"]
	h.orc.accountManageResult = orchestrator.AccountManageResult{MatchedAccountID: &matched}
	h.ScriptToolCalls(manageSettingsCall(agent.SettingsAreaAccount))

	h.SendText("renombrá la cuenta del banco")

	if fl, _ := h.FlowState(); fl != flow.AccountManageFlowName {
		t.Fatalf("flow abierto = %q, want %q. Copia: %v", fl, flow.AccountManageFlowName, h.Messages())
	}
}

func TestConversation_ManageSettingsCategory_OpensTheProposalConfirm(t *testing.T) {
	h := newConversationHarness(t)
	h.orc.categoryResult = orchestrator.CategoryCreateResult{Proposal: &orchestrator.CategoryProposal{
		Category: "Ocio y salidas", Subcategory: "Escalada indoor", Icon: "🧗",
		Description: "Entradas y alquiler de equipo en el rocódromo",
	}}
	h.ScriptToolCalls(manageSettingsCall(agent.SettingsAreaCategory))

	h.SendText("quiero una categoría para la escalada")

	if fl, _ := h.FlowState(); fl != flow.CategoryProposalConfirmFlowName {
		t.Fatalf("flow abierto = %q, want %q. Copia: %v", fl, flow.CategoryProposalConfirmFlowName, h.Messages())
	}
}

func TestConversation_ManageSettingsCategory_DuplicateOffersTheExistingOne(t *testing.T) {
	h := newConversationHarness(t)
	h.orc.categoryResult = orchestrator.CategoryCreateResult{Proposal: &orchestrator.CategoryProposal{
		Category: "Mascotas", Subcategory: "Veterinaria", Icon: "🐶",
		Description: "Consultas y vacunas del perro",
	}}
	h.ScriptToolCalls(manageSettingsCall(agent.SettingsAreaCategory))

	h.SendText("quiero una categoría para el veterinario")

	if fl, _ := h.FlowState(); fl != flow.CategoryMatchOfferFlowName {
		t.Fatalf("flow abierto = %q, want %q. Copia: %v", fl, flow.CategoryMatchOfferFlowName, h.Messages())
	}
}

func TestConversation_ManageSettingsUnknownArea_AsksForARewrite(t *testing.T) {
	h := newConversationHarness(t)
	h.ScriptToolCalls(manageSettingsCall("presupuesto"))

	h.SendText("configurame un presupuesto")

	if flow, _ := h.FlowState(); flow != "" {
		t.Errorf("un área desconocida abrió el flow %q", flow)
	}
	if h.LastMessage() != messages.MsgAskRewrite {
		t.Errorf("último mensaje = %q, want msgAskRewrite", h.LastMessage())
	}
}

func TestConversation_RecategorizeABatch(t *testing.T) {
	h := newConversationHarness(t)
	today := agent.StartOfTodayArgentina()
	super := h.subID("Alimentación", "Supermercado")
	destino := h.subID("Vivienda", "Mantenimiento hogar")
	ids := []uint{
		h.SeedMovementOn("lote materiales", "-80000", super, today),
		h.SeedMovementOn("lote cemento", "-45000", super, today),
		h.SeedMovementOn("lote arena", "-25000", super, today),
	}

	h.ScriptToolCalls(correctMovementCall(`{
		"change":"mové los movimientos del lote a mantenimiento hogar",
		"scope":"all",
		"changes":[{"field":"category","op":"set","value":"mantenimiento hogar"}]}`))

	h.SendText("mové los movimientos del lote a mantenimiento hogar")
	h.TapButton(flow.OptionConfirm)

	var moved int
	for _, m := range h.Movements() {
		if m.SubcategoryID == destino {
			moved++
		}
	}
	if moved != len(ids) {
		t.Fatalf("quedaron %d de %d filas en Mantenimiento hogar. Base: %s. Copia: %v",
			moved, len(ids), describeRows(h.Movements()), h.Messages())
	}
	if got := len(h.Movements()); got != len(ids) {
		t.Errorf("quedaron %d movimientos, want %d", got, len(ids))
	}
}

func describeRows(ms []movement.Movement) string {
	var out []string
	for _, m := range ms {
		desc := ""
		if m.Description != nil {
			desc = *m.Description
		}
		out = append(out, fmt.Sprintf("#%d %s sub=%d %s", m.ID, desc, m.SubcategoryID, m.Amount))
	}
	return "[" + strings.Join(out, " | ") + "]"
}

func TestConversation_NoOpCorrectionWritesNothing(t *testing.T) {
	h := newConversationHarness(t)
	super := h.subID("Alimentación", "Supermercado")
	id := h.SeedMovementOn("Carrefour", "-12700", super, agent.StartOfTodayArgentina())

	h.ScriptToolCalls(correctMovementCall(`{
		"change":"ponelo en supermercado",
		"changes":[{"field":"category","op":"set","value":"Supermercado"}]}`))

	h.SendText("el carrefour ponelo en supermercado")

	movs := h.Movements()
	if len(movs) != 1 || movs[0].ID != id {
		t.Fatalf("la fila se reemplazó por una idéntica: %s", describeRows(movs))
	}
	if !containsAny(h.Messages(), "ya estaba así") {
		t.Errorf("no se le avisó al usuario que no cambiaba nada: %v", h.Messages())
	}
}

func TestConversation_ManageSettingsCategoryManage_OpensThePickFlow(t *testing.T) {
	h := newConversationHarness(t)
	h.SeedOwnCategory("Mascotas", "Paseador", "El que saca al perro", "🐕")

	h.ScriptToolCalls(manageSettingsCall(agent.SettingsAreaCategoryManage))
	h.SendText("eliminá subcategorías")

	if fl, _ := h.FlowState(); fl != flow.CategoryManagePickFlowName {
		t.Fatalf("flow abierto = %q, want %q. Copia: %v", fl, flow.CategoryManagePickFlowName, h.Messages())
	}
}

func TestConversation_GuardsSayWhatHappened(t *testing.T) {
	casos := []struct {
		nombre  string
		monto   string
		cambio  string
		mensaje string
		espera  string
	}{
		{
			nombre: "reintegro mayor que el gasto",
			monto:  "-2500", mensaje: "del peaje me devolvieron 99999",
			cambio: `{"field":"amount","op":"subtract","value":"99999"}`,
			espera: "más de lo que salió",
		},
		{
			nombre: "un reintegro que haría crecer el gasto",
			monto:  "-2500", mensaje: "del peaje me devolvieron algo",
			cambio: `{"field":"amount","op":"add","value":"500"}`,
			espera: "lo dejaría más caro",
		},
	}
	for _, tc := range casos {
		t.Run(tc.nombre, func(t *testing.T) {
			h := newConversationHarness(t)
			id := h.SeedMovementOn("Peaje", tc.monto, h.subID("Transporte", "Peaje"), agent.StartOfTodayArgentina())

			h.ScriptToolCalls(correctMovementCall(
				`{"change":"` + tc.mensaje + `","changes":[` + tc.cambio + `]}`))
			h.SendText(tc.mensaje)

			movs := h.Movements()
			if len(movs) != 1 || movs[0].ID != id || !movs[0].Amount.Equal(mustDec(t, tc.monto)) {
				t.Fatalf("la guarda tenía que dejar la fila intacta: %s", describeRows(movs))
			}
			if !containsAny(h.Messages(), tc.espera) {
				t.Errorf("la copy no dice qué pasó, salió: %v", h.Messages())
			}
		})
	}
}

func TestRecentEntities_ReachTheWholeDayNotTenMinutes(t *testing.T) {
	h := newConversationHarness(t)
	h.SeedMovementOn("Peaje", "-2500", h.subID("Transporte", "Peaje"), agent.StartOfTodayArgentina())
	h.conn.DB.Exec(
		"UPDATE movements SET created_at = now() - interval '30 minutes' WHERE user_id = ? AND description = 'Peaje'",
		h.userID)

	bloque := agent.BuildRecentEntities(h.c, h.userID)

	if !strings.Contains(bloque, "Peaje") {
		t.Errorf("el peaje de hace 30 min tiene que estar en el bloque:\n%q", bloque)
	}
}
