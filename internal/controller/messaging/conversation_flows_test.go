//go:build conv_test

package messaging

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messages"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
)

// Los flujos que después de la etapa 5 SÓLO se alcanzan por el loop, probados
// con datos sembrados y aserciones contra la base.
//
// Antes de la etapa 5 a cada uno de estos lo abría una rama del router, y lo que
// los tests cubrían era esa rama. El router ya no está: si el loop no elige la
// tool correcta, o el delegado no arranca el wizard, el usuario queda sin
// camino — y nada más en la suite lo notaría.

// subID busca el id de un par en la taxonomía VIVA.
//
// Los ids NO van literales, y no es purismo: la primera versión de estos tests
// sembraba con 1/13/18/20, sacados de un SELECT que no filtraba deleted_at. La
// taxonomía se repodó en algún momento —"Proyecto hogar" pasó a ser
// "Mantenimiento hogar"— así que los cuatro apuntaban a filas borradas y los
// escenarios medían mi consulta, no el código.
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

// SeedMovementOn es SeedMovement con fecha y subcategoría a elección: sin
// controlar la fecha no se puede probar un rango, que es de lo que vive QUERY.
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

// SeedCorpus siembra el mes en curso con gastos de tres categorías. Los montos
// son distintos entre sí a propósito: con montos repetidos un total correcto y
// uno cruzado dan lo mismo, y el test pasaría con la agrupación rota.
func (h *convHarness) SeedCorpus() {
	h.t.Helper()
	today := startOfTodayArgentina()
	super := h.subID("Alimentación", "Supermercado")
	h.SeedMovementOn("Carrefour", "-12700", super, today)
	h.SeedMovementOn("Coto", "-3000", super, today.AddDate(0, 0, -2))
	h.SeedMovementOn("Edesur", "-45000", h.subID("Vivienda", "Luz"), today.AddDate(0, 0, -5))
	h.SeedMovementOn("YPF", "-30000", h.subID("Transporte", "Combustible"), today.AddDate(0, 0, -7))
}

// ScriptQuery maneja el loop de QUERY: recibe el executor real, así que las
// tools de lectura pegan contra Postgres.
func (h *convHarness) ScriptQuery(fn func(execute func(string, json.RawMessage) (string, error)) (string, error)) {
	h.orc.queryRunFn = fn
}

// FlowState devuelve el flow y el step guardados para el usuario, o ("","")
// si no hay conversación abierta. Es la aserción de "el wizard arrancó": el
// wizard vive en conversation_states, no en la copia.
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

// QUERY de punta a punta con datos reales: el loop elige answer_query, el
// delegado entra al loop de consulta, y sum_movements suma lo que hay en la
// base. Lo que se asierta es el número que devolvió la TOOL, no la prosa del
// modelo — el modelo está scripteado y su prosa la escribí yo.
func TestConversation_AnswerQuerySumsRealData(t *testing.T) {
	h := newConversationHarness(t)
	h.SeedCorpus()

	var toolOut string
	h.ScriptQuery(func(execute func(string, json.RawMessage) (string, error)) (string, error) {
		today := startOfTodayArgentina()
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

	// Alimentación 12.700 + 3.000; Vivienda 45.000; Transporte 30.000.
	for _, want := range []string{"15700.00", "45000.00", "30000.00"} {
		if !strings.Contains(toolOut, want) {
			t.Errorf("falta %s en el total por categoría:\n%s", want, toolOut)
		}
	}
	// La apertura de la cuenta es un transfer y sum_movements las excluye: si
	// aparece, el filtro de transferencias se rompió y todo total queda inflado.
	if strings.Contains(toolOut, "1000000") {
		t.Errorf("la apertura de la cuenta entró en el total:\n%s", toolOut)
	}
	if !strings.Contains(h.LastMessage(), "Gastaste esto") {
		t.Errorf("la respuesta no llegó al usuario, salió: %q", h.LastMessage())
	}
}

// El loop puede contestar sobre una cuenta sin que exista un intent de cuentas:
// account_balance suma los movimientos, apertura incluida.
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

	// 1.000.000 de apertura − 90.700 de gastos.
	if !strings.Contains(toolOut, "909300") {
		t.Errorf("saldo mal calculado, la tool devolvió:\n%s", toolOut)
	}
}

// manage_settings, área recordatorio. Es el área más barata de probar y la
// única sin una segunda llamada al modelo: si el wizard no queda abierto en la
// base, el usuario tocó un botón que no existe.
func TestConversation_ManageSettingsReminder_OpensTheWizard(t *testing.T) {
	h := newConversationHarness(t)
	h.ScriptToolCalls(manageSettingsCall(settingsAreaReminder))

	h.SendText("quiero que me recuerdes cargar los gastos")

	if fl, _ := h.FlowState(); fl != flow.ReminderSetupFlowName {
		t.Fatalf("flow abierto = %q, want %q. Copia: %v", fl, flow.ReminderSetupFlowName, h.Messages())
	}
}

// Área cuenta: el delegado corre ResolveAccountManage y abre el menú sobre la
// cuenta que matcheó.
func TestConversation_ManageSettingsAccount_OpensTheManageFlow(t *testing.T) {
	h := newConversationHarness(t)
	matched := h.accounts["Banco Test"]
	h.orc.accountManageResult = orchestrator.AccountManageResult{MatchedAccountID: &matched}
	h.ScriptToolCalls(manageSettingsCall(settingsAreaAccount))

	h.SendText("renombrá la cuenta del banco")

	if fl, _ := h.FlowState(); fl != flow.AccountManageFlowName {
		t.Fatalf("flow abierto = %q, want %q. Copia: %v", fl, flow.AccountManageFlowName, h.Messages())
	}
}

// Área categoría: con una propuesta que NO existe, el wizard de 7 pasos colapsa
// en una confirmación.
//
// El par tiene que ser inexistente de verdad. La primera versión de este test
// propuso "Mascotas | Veterinaria", que YA está en la taxonomía global, y el
// código hizo lo correcto — ofrecer la que existe — así que el test medía mi
// fixture, no el camino que decía probar.
func TestConversation_ManageSettingsCategory_OpensTheProposalConfirm(t *testing.T) {
	h := newConversationHarness(t)
	h.orc.categoryResult = orchestrator.CategoryCreateResult{Proposal: &orchestrator.CategoryProposal{
		Category: "Ocio y salidas", Subcategory: "Escalada indoor", Icon: "🧗",
		Description: "Entradas y alquiler de equipo en el rocódromo",
	}}
	h.ScriptToolCalls(manageSettingsCall(settingsAreaCategory))

	h.SendText("quiero una categoría para la escalada")

	if fl, _ := h.FlowState(); fl != flow.CategoryProposalConfirmFlowName {
		t.Fatalf("flow abierto = %q, want %q. Copia: %v", fl, flow.CategoryProposalConfirmFlowName, h.Messages())
	}
}

// Y la otra rama, que salió gratis del error de arriba: una propuesta que ya
// existe no se re-crea, se ofrece. Sin este caso, un código que SIEMPRE fuera al
// match-offer pasaría igual, y la taxonomía se llenaría de duplicados.
func TestConversation_ManageSettingsCategory_DuplicateOffersTheExistingOne(t *testing.T) {
	h := newConversationHarness(t)
	h.orc.categoryResult = orchestrator.CategoryCreateResult{Proposal: &orchestrator.CategoryProposal{
		Category: "Mascotas", Subcategory: "Veterinaria", Icon: "🐶",
		Description: "Consultas y vacunas del perro",
	}}
	h.ScriptToolCalls(manageSettingsCall(settingsAreaCategory))

	h.SendText("quiero una categoría para el veterinario")

	if fl, _ := h.FlowState(); fl != flow.CategoryMatchOfferFlowName {
		t.Fatalf("flow abierto = %q, want %q. Copia: %v", fl, flow.CategoryMatchOfferFlowName, h.Messages())
	}
}

// Un área que el switch no conoce no puede quedar en silencio: el pedido muere
// en ask_rewrite, y el usuario tiene que enterarse.
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

// EL caso guía del 2026-08-10, con los datos que lo generaron: tres gastos de
// un lote de materiales cargados en Supermercado, y el pedido de moverlos todos
// a Vivienda | Mantenimiento hogar — que EXISTE en la taxonomía viva, así que no
// hay ningún gap que llenar.
//
// La condición de aceptación de la etapa entera, y se juzga por la BASE: las
// tres filas tienen que terminar en la subcategoría nueva. Que se ofrezca una
// corrección no cuenta — leer el ofrecimiento como éxito es exactamente lo que
// escondió el bug durante dos días.
func TestConversation_RecategorizeABatch(t *testing.T) {
	h := newConversationHarness(t)
	today := startOfTodayArgentina()
	super := h.subID("Alimentación", "Supermercado")
	destino := h.subID("Vivienda", "Mantenimiento hogar")
	ids := []uint{
		h.SeedMovementOn("lote materiales", "-80000", super, today),
		h.SeedMovementOn("lote cemento", "-45000", super, today),
		h.SeedMovementOn("lote arena", "-25000", super, today),
	}

	// El modelo emite un DIFF sobre TODOS los candidatos, no las filas corregidas
	// una por una. El destino viaja tal como lo diría el usuario, en minúsculas y
	// sin la categoría: resolver el par contra la taxonomía es trabajo de la app.
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
	// Ninguna se pierde por el camino: recategorizar mueve, no borra.
	if got := len(h.Movements()); got != len(ids) {
		t.Errorf("quedaron %d movimientos, want %d", got, len(ids))
	}
}

// describeRows arma una línea por movimiento para que un fallo diga QUÉ quedó
// en la base, no sólo que la cuenta no dio.
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

// Una corrección que deja todo igual no se escribe. Confirmarla haría un
// DELETE+INSERT para no cambiar nada: quema un id y cuenta como
// update_confirmed. Visto en vivo el 2026-08-12 al contestar la categoría que el
// movimiento YA tenía.
func TestConversation_NoOpCorrectionWritesNothing(t *testing.T) {
	h := newConversationHarness(t)
	super := h.subID("Alimentación", "Supermercado")
	id := h.SeedMovementOn("Carrefour", "-12700", super, startOfTodayArgentina())

	// El "cambio" nombra la categoría que la fila ya tiene.
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

// Sacar una categoría propia es OTRO wizard que el de crearla, y hasta el
// 2026-08-12 era inalcanzable: las dos cosas compartían el área "categoria" y el
// área entera iba al wizard de ALTA. Al morir el router, category_manage_pick se
// quedó sin ningún camino que lo abriera — una feature huérfana, registrada en
// server.go y sin llamador.
func TestConversation_ManageSettingsCategoryManage_OpensThePickFlow(t *testing.T) {
	h := newConversationHarness(t)
	// El flow de administrar exige al menos una categoría PROPIA: sin eso corta
	// antes con "no creaste ninguna", que es una respuesta correcta pero de otro
	// caso.
	h.SeedOwnCategory("Mascotas", "Paseador", "El que saca al perro", "🐕")

	h.ScriptToolCalls(manageSettingsCall(settingsAreaCategoryManage))
	h.SendText("eliminá subcategorías")

	if fl, _ := h.FlowState(); fl != flow.CategoryManagePickFlowName {
		t.Fatalf("flow abierto = %q, want %q. Copia: %v", fl, flow.CategoryManagePickFlowName, h.Messages())
	}
}

// Las tres guardas que un usuario puede disparar hablando normal tienen que
// decir QUÉ pasó, no la genérica de "no te entendí" — en los tres casos el bot
// entendió y se niega, y mandarlo a reformular algo que dijo bien lo hace girar.
//
// Medido en vivo el 2026-08-12: "Del peaje me devolvieron 99999" sobre un gasto
// de $2.500 cortaba con la copy genérica.
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
			id := h.SeedMovementOn("Peaje", tc.monto, h.subID("Transporte", "Peaje"), startOfTodayArgentina())

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

// El bloque de entidades recientes tiene que alcanzar TODO EL DÍA, no los
// últimos diez minutos.
//
// Medido en vivo el 2026-08-12: "El peaje ponelo en banco galicia" 17 minutos
// después de cargarlo. La ventana era justCreatedWindow (10 min, compartida con
// el gate de casi-duplicado), así que el peaje no estaba en el prompt: el modelo
// no tenía a qué referirse y pidió el monto para registrarlo de nuevo. La
// inferencia era correcta — le faltaba el ancla.
func TestRecentEntities_ReachTheWholeDayNotTenMinutes(t *testing.T) {
	h := newConversationHarness(t)
	// Cargado hace media hora: fuera de justCreatedWindow, dentro de hoy.
	h.SeedMovementOn("Peaje", "-2500", h.subID("Transporte", "Peaje"), startOfTodayArgentina())
	h.conn.DB.Exec(
		"UPDATE movements SET created_at = now() - interval '30 minutes' WHERE user_id = ? AND description = 'Peaje'",
		h.userID)

	bloque := h.c.buildRecentEntities(h.userID)

	if !strings.Contains(bloque, "Peaje") {
		t.Errorf("el peaje de hace 30 min tiene que estar en el bloque:\n%q", bloque)
	}
}
