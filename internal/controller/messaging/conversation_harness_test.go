//go:build conv_test

package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/database"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
	"lopiibot.com/internal/user"
)

// El nivel 2 del sistema de tests: conversación MULTI-TURNO con el modelo
// scripteado y todo lo demás real — engine, cola de parking, flows, Postgres.
//
// Es el nivel que faltaba, y no es un detalle: la tesis de la etapa 5 es que la
// corrección funciona porque el loop puede recordar, parkear una pregunta y
// retomar la respuesta. Todo lo que había era de un solo turno o función pura,
// y el arnés de eval usa un executor falso que NO puede llegar a
// park → callback → resume, que es exactamente donde murieron los dos intentos
// del 2026-08-10.
//
// LA REGLA DE ORO DE ESTE ARCHIVO: un escenario pasa cuando la BASE cambió,
// nunca cuando se ofreció una confirmación. Asertar el ofrecimiento es
// precisamente el error que escondió aquel bug durante dos días.
//
// Corre con:
//
//	go test -tags conv_test ./internal/controller/messaging/ -v
//
// Necesita el Postgres local (docker compose up -d) y NO necesita API key: para
// eso el modelo está scripteado.

type convHarness struct {
	t   *testing.T
	c   *controller
	b   *bot.Bot
	rt  *recordingTransport
	orc *fakeFullOrchestrator

	conn *database.Connection
	// userID es users.id — el que toma handleFreeText.
	userID uint64
	// telegramID es users.telegram_id — el que viaja en el Update de un callback,
	// que lo resuelve por FindByTelegramID. Son DOS espacios de ids distintos y
	// confundirlos hace que el botón opere sobre otro usuario que el texto.
	telegramID  string
	telegramNum int64
	chatID      int64

	accounts map[string]uint64
}

func newConversationHarness(t *testing.T) *convHarness {
	t.Helper()

	conn, err := database.Initialize(database.Creds{
		Host: "localhost", Port: 5432, Name: "lopiibot", User: "lopiibot", Password: "lopiibot",
	}, false)
	if err != nil {
		t.Skipf("el nivel 2 necesita Postgres local (docker compose up -d): %v", err)
	}

	userRepo := user.NewRepository(conn)
	accRepo := account.NewRepository(conn)
	movRepo := movement.InitRepository(conn)
	subRepo := subcategory.NewRepository(conn)
	cache, err := subcategory.NewCache(subRepo)
	if err != nil {
		t.Fatalf("subcategory cache: %v", err)
	}

	// El telegram id tiene que ser NUMÉRICO: viaja como int64 en el Update del
	// callback y el handler lo pasa a string para FindByTelegramID. Con un prefijo
	// de texto el round-trip no cierra y el botón no encuentra al usuario.
	tgNum := time.Now().UnixNano() % 1_000_000_000
	tgID := fmt.Sprintf("%d", tgNum)
	u := &user.User{TelegramID: tgID}
	if err := userRepo.Insert(u); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	uid := u.ID
	t.Cleanup(func() {
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&movement.Movement{})
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&account.Account{})
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&pendingaction.PendingAction{})
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&reminder.Reminder{})
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&subcategory.Subcategory{})
		conn.DB.Unscoped().Exec("DELETE FROM conversation_states WHERE user_id = ?", uid)
		conn.DB.Unscoped().Where("id = ?", uid).Delete(&user.User{})
	})

	// Con apertura: una cuenta en cero hace que CUALQUIER gasto dispare el gate
	// de saldo insuficiente, y todos los escenarios medirían eso en vez de lo suyo.
	banco := &account.Account{UserID: uid, Name: "Banco Test", Type: account.StandardType, Currency: currency.ARS, IsDefault: true}
	if err := movRepo.InsertAccountsWithOpenings([]movement.AccountOpening{{
		Account: banco,
		Movement: movement.Movement{
			UserID: uid, SubcategoryID: 1, Date: time.Now(),
			Type: movement.Transfer, Amount: decimal.NewFromInt(1000000), Currency: currency.ARS,
		},
	}}); err != nil {
		t.Fatalf("insert banco con apertura: %v", err)
	}

	rt := &recordingTransport{}
	b, err := bot.New("123:ABC", bot.WithSkipGetMe(), bot.WithHTTPClient(time.Second, &http.Client{Transport: rt}))
	if err != nil {
		t.Fatalf("bot.New: %v", err)
	}

	// Store REAL: el estado de la conversación tiene que sobrevivir entre turnos,
	// que es todo el punto del nivel.
	engine := conversation.NewEngine(conversation.NewRepository(conn), FlowResumeLabel)
	// Los mismos que registra server.go: parkear a un flow no registrado es un
	// error de arranque, y un escenario que lo toque muere con un mensaje que no
	// habla de lo que el escenario prueba.
	engine.Register(NewMovementCreateFlow(cache, accRepo))
	engine.Register(NewMovementUpdatePickFlow())
	engine.Register(NewMovementUpdateConfirmFlow())
	engine.Register(NewMovementDeleteFlow())
	engine.Register(NewAccountCreateFlow())
	engine.Register(NewAccountManageFlow(movRepo))
	engine.Register(NewAccountMoveOfferFlow())
	engine.Register(NewSubcategorySetupFlow(cache))
	engine.Register(NewCategoryMatchOfferFlow())
	engine.Register(NewCategoryProposalConfirmFlow())
	engine.Register(NewCategoryManagePickFlow(cache))
	engine.Register(NewCategoryManageTargetFlow(cache))
	engine.Register(NewMovementNegativeConfirmFlow())
	engine.Register(NewReminderSetupFlow())
	engine.Register(NewAskUserFlow())

	// El clasificador va scripteado con un par válido por default: desde que la
	// clasificación salió del loop, sin par TODA fila cae en PENDING_REVIEW y
	// abre el picker — y cada escenario mediría el gap-fill en vez de lo suyo.
	// ScriptCategory lo pisa donde el escenario necesite otra cosa.
	orc := &fakeFullOrchestrator{
		classifyPairs: []orchestrator.Pair{{Category: "Alimentación", Subcategory: "Supermercado"}},
	}
	c := &controller{
		engine:        engine,
		orchestrator:  orc,
		users:         userRepo,
		accounts:      accRepo,
		movements:     movRepo,
		subcategories: cache,
		actions:       pendingaction.NewRepository(conn),
		reminders:     reminder.NewRepository(conn),
		metrics:       &fakeMetricRepo{},
		chatHistory:   stubChatHistory{},
	}

	return &convHarness{
		t: t, c: c, b: b, rt: rt, orc: orc, conn: conn,
		userID: uid, telegramID: tgID, telegramNum: tgNum, chatID: 4242,
		accounts: map[string]uint64{"Banco Test": uint64(banco.ID)},
	}
}

// ScriptIntent quedó sin efecto: la etapa 5 borró el router, así que ya no hay
// intent que fijar. Se conserva como no-op para no reescribir cada escenario, y
// como recordatorio de que el mensaje ya no se clasifica antes del loop.
func (h *convHarness) ScriptIntent(orchestrator.Intent) {}

// ScriptCategory fija el par que devuelve el clasificador.
func (h *convHarness) ScriptCategory(pairs ...orchestrator.Pair) { h.orc.classifyPairs = pairs }

// ScriptToolCalls encola lo que el loop "emite": una ronda por llamada. El
// executor es el REAL, así que lo que pasa después de la tool call es
// producción.
func (h *convHarness) ScriptToolCalls(calls ...scriptedCall) {
	remaining := calls
	h.orc.runFn = func(execute func(string, json.RawMessage) (string, error)) (string, error) {
		for _, call := range remaining {
			if _, err := execute(call.name, json.RawMessage(call.args)); err != nil {
				return "", err
			}
		}
		remaining = nil
		return "", nil
	}
}

type scriptedCall struct {
	name string
	args string
}

func recordMovementsCall(args string) scriptedCall {
	return scriptedCall{orchestrator.ToolRecordMovements, args}
}
func correctMovementCall(args string) scriptedCall {
	return scriptedCall{orchestrator.ToolCorrectMovement, args}
}
func deleteMovementsCall(args string) scriptedCall {
	return scriptedCall{orchestrator.ToolDeleteMovements, args}
}

// SendText simula un mensaje de texto del usuario. Toma users.id.
func (h *convHarness) SendText(text string) {
	h.t.Helper()
	if err := h.c.handleFreeText(context.Background(), h.b, h.chatID, h.userID, text); err != nil {
		h.t.Fatalf("SendText(%q): %v", text, err)
	}
}

// TapButton simula tocar un botón. handleConversationInput toma el *models.Update
// crudo del webhook y resuelve el usuario por TELEGRAM id, no por users.id.
func (h *convHarness) TapButton(data string) {
	h.t.Helper()
	h.c.handleConversationInput(context.Background(), h.b, &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:      "cb",
			Data:    data,
			From:    models.User{ID: h.telegramNum},
			Message: models.MaybeInaccessibleMessage{Message: &models.Message{Chat: models.Chat{ID: h.chatID}}},
		},
	})
}

// Movements es la superficie de aserción: lo que quedó EN LA BASE, sin la
// apertura de la cuenta.
//
// La apertura se filtra por la misma razón por la que MovementQuery filtra las
// categorías reservadas: es plomería del modelo, no plata que el usuario movió,
// y si apareciera cada escenario tendría que descontarla a mano. AllMovements
// la trae para el escenario al que le importe.
func (h *convHarness) Movements() []movement.Movement {
	h.t.Helper()
	var out []movement.Movement
	for _, m := range h.AllMovements() {
		if m.Type == movement.Transfer && m.TransactionID == nil {
			continue // apertura: pata suelta tipada transfer, sin contraparte
		}
		out = append(out, m)
	}
	return out
}

// AllMovements trae todo, apertura incluida.
func (h *convHarness) AllMovements() []movement.Movement {
	h.t.Helper()
	var out []movement.Movement
	if err := h.conn.DB.Where("user_id = ?", h.userID).Order("id").Find(&out).Error; err != nil {
		h.t.Fatalf("AllMovements: %v", err)
	}
	return out
}

// Messages es la copia que salió, en orden.
func (h *convHarness) Messages() []string { return h.rt.texts }

// LastMessage es la última copia que salió.
func (h *convHarness) LastMessage() string {
	if len(h.rt.texts) == 0 {
		return ""
	}
	return h.rt.texts[len(h.rt.texts)-1]
}

// SeedMovement inserta un movimiento ya existente, para los escenarios que
// arrancan con historia.
func (h *convHarness) SeedMovement(description, amount string, subcategoryID uint64) uint {
	h.t.Helper()
	accID := h.accounts["Banco Test"]
	movs := []movement.Movement{{
		UserID: h.userID, AccountID: &accID, SubcategoryID: subcategoryID,
		Date: time.Now(), Type: movement.Expense,
		Amount: mustDec(h.t, amount), Currency: currency.ARS,
		Description: &description,
	}}
	if err := movement.InitRepository(h.conn).InsertBatch(movs); err != nil {
		h.t.Fatalf("SeedMovement: %v", err)
	}
	return movs[0].ID
}

func mustDec(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatalf("decimal %q: %v", s, err)
	}
	return d
}

// El arnés tiene que probarse a sí mismo antes de que alguien lo use para
// probar otra cosa: un turno, una tool call, una fila en la base con el signo
// contable correcto.
func TestHarness_CreateWritesToTheDatabase(t *testing.T) {
	h := newConversationHarness(t)
	h.ScriptIntent(orchestrator.IntentCreate)
	h.ScriptToolCalls(recordMovementsCall(`{"movements":[
		{"type":"expense","amount":"12700","currency":"ARS","category":"Alimentación",
		 "subcategory":"Supermercado","description":"Café","date":"2026-08-12"}]}`))

	h.SendText("Cafe 12700")

	movs := h.Movements()
	if len(movs) != 1 {
		t.Fatalf("movimientos = %d, want 1. Copia: %v", len(movs), h.Messages())
	}
	if got := movs[0].Amount.String(); got != "-12700" {
		t.Errorf("amount = %s, want -12700 (el gasto se guarda negativo)", got)
	}
}

// El primer escenario multi-turno de verdad: texto → parkeo → BOTÓN → escritura.
// Es el camino que ningún test tocaba y donde murieron los dos intentos del
// 2026-08-10, y la aserción es sobre la BASE, no sobre la copia ofrecida.
func TestConversation_DeleteConfirmedActuallyDeletes(t *testing.T) {
	h := newConversationHarness(t)
	id := h.SeedMovement("Café", "-12700", 1)

	h.ScriptIntent(orchestrator.IntentDelete)
	h.ScriptToolCalls(deleteMovementsCall(`{"reference":"el café de hoy"}`))
	h.SendText("borrá el café de hoy")

	if len(h.Movements()) != 1 {
		t.Fatalf("todavía no se confirmó nada: el movimiento tiene que seguir. Copia: %v", h.Messages())
	}

	// Con UN candidato el loop siembra resolved_index y el picker se saltea, así
	// que el primer botón que ve el usuario ya es el de confirmar. (Cuando hay
	// varios, el picker manda ÍNDICES y no etiquetas: callback_data son 64 bytes.)
	h.TapButton(optionConfirm)

	movs := h.Movements()
	if len(movs) != 0 {
		t.Fatalf("el movimiento %d sigue vivo después de confirmar el borrado: %+v", id, movs)
	}
}

// La otra dirección: cancelar NO borra. Sin este caso, un flujo que borrara
// siempre pasaría el test de arriba.
func TestConversation_DeleteCancelledKeepsTheMovement(t *testing.T) {
	h := newConversationHarness(t)
	h.SeedMovement("Café", "-12700", 1)

	h.ScriptIntent(orchestrator.IntentDelete)
	h.ScriptToolCalls(deleteMovementsCall(`{"reference":"el café de hoy"}`))
	h.SendText("borrá el café de hoy")
	h.TapButton("cancel")

	if len(h.Movements()) != 1 {
		t.Errorf("cancelar borró igual: %+v", h.Movements())
	}
}

// ScriptUpdateResult fija lo que devuelve la Call 2 de UPDATE (ResolveUpdate).
func (h *convHarness) ScriptUpdateResult(r orchestrator.UpdateResult) { h.orc.updateResult = r }

// El bug vivo, probado de punta a punta: una corrección que nombra una
// categoría que NO existe en la taxonomía del usuario.
//
// Hasta el 2026-08-12, seedAndStartUpdateConfirm escribía los gaps en nil
// hardcodeado, así que el par inventado no marcaba nada, el flujo insertaba
// derecho, FindByCategoryAndSubcategory fallaba y EL MOVIMIENTO SE PERDÍA con
// un error genérico. Lo que importa acá no es que aparezca el picker: es que el
// movimiento SIGA EXISTIENDO.
func TestConversation_CorrectionWithUnknownCategoryKeepsTheMovement(t *testing.T) {
	h := newConversationHarness(t)
	id := h.SeedMovement("Café", "-12700", 1)

	h.ScriptIntent(orchestrator.IntentUpdate)
	// El par que nombra el usuario NO existe en la taxonomía. Va por el camino
	// estructurado, que es el único desde que `changes` es requerido.
	h.ScriptToolCalls(correctMovementCall(`{
		"change":"ponelo en proyecto hogar",
		"changes":[{"field":"category","op":"set","value":"proyecto hogar"}]}`))

	h.SendText("el café ponelo en proyecto hogar")
	h.TapButton(optionConfirm) // resuelve el picker de candidatos

	if got := h.Movements(); len(got) != 1 || got[0].ID != id {
		t.Fatalf("el movimiento %d se perdió — este es EL bug. Quedó: %+v. Copia: %v", id, got, h.Messages())
	}
	if !containsAny(h.Messages(), "categoría") {
		t.Errorf("esperaba que pregunte por la categoría, salió: %v", h.Messages())
	}
}

func containsAny(msgs []string, want string) bool {
	for _, m := range msgs {
		if strings.Contains(strings.ToLower(m), strings.ToLower(want)) {
			return true
		}
	}
	return false
}

// EL caso guía del 2026-08-10, modelado como pasó de verdad: "Editá los
// movimientos de lote de hoy" nombra el destino y NO el cambio, y el modelo
// devolvió filas con el monto en CERO. correctionIsDeletion leyó eso como
// "regalo/gratis total", armó un BORRADO, y el usuario lo confirmó — sólo no se
// borró porque la escritura falló (dos de dos, intent_events 386 y 388).
//
// Ese accidente ya no está: SoftDeleteByIDs dejó de fallar. Así que la guarda
// tiene que sostenerlo sola.
func TestConversation_ZeroAmountsWithoutTheUserNamingMoney_NeverOffersDeletion(t *testing.T) {
	h := newConversationHarness(t)
	id := h.SeedMovement("lote", "-80000", 1)

	h.ScriptIntent(orchestrator.IntentUpdate)
	h.ScriptToolCalls(correctMovementCall(`{"change":"editá los movimientos de lote de hoy"}`))
	h.ScriptUpdateResult(orchestrator.UpdateResult{Resolved: true, Movements: []orchestrator.MovementDraft{{
		Type: "expense", Amount: "0", Currency: "ARS",
		Category: "Alimentación", Subcategory: "Supermercado",
		Description: "lote", Date: "2026-08-12",
	}}})

	h.SendText("Editá los movimientos de lote de hoy")

	// Nada de "borrar" en la copia: el usuario pidió EDITAR.
	for _, m := range h.Messages() {
		low := strings.ToLower(m)
		if strings.Contains(low, "borr") || strings.Contains(low, "elimin") {
			t.Errorf("se ofreció un borrado ante un pedido de edición: %q", m)
		}
	}

	// Y aunque confirme lo que sea que se le ofreció, el movimiento sigue.
	h.TapButton(optionConfirm)
	movs := h.Movements()
	if len(movs) != 1 || movs[0].ID != id {
		t.Fatalf("el movimiento se perdió: %+v. Copia: %v", movs, h.Messages())
	}
	if movs[0].Amount.IsZero() {
		t.Error("quedó un movimiento en 0: el guard lo rechaza y no es un estado válido")
	}
}

// La capa 2 de punta a punta: el caso de las 22:27 del 2026-08-11. El usuario
// escribió "Al café de hoy sumale 1070" cuatro veces y se llevó cuatro
// movimientos nuevos. El gate no arregla al modelo — caza la FORMA del daño.
//
// Un helper por botón, porque lo que hay que probar es el estado final de la
// BASE en cada rama, no que el botón exista.
func (h *convHarness) insertCoffeeThenCorrection(t *testing.T) {
	t.Helper()
	h.ScriptIntent(orchestrator.IntentCreate)
	h.ScriptToolCalls(recordMovementsCall(`{"movements":[
		{"type":"expense","amount":"12700","currency":"ARS","category":"Alimentación",
		 "subcategory":"Supermercado","description":"Café","date":"2026-08-12"}]}`))
	h.SendText("Cafe 12700")

	h.ScriptToolCalls(recordMovementsCall(`{"movements":[
		{"type":"expense","amount":"1070","currency":"ARS","category":"Alimentación",
		 "subcategory":"Supermercado","description":"Café","date":"2026-08-12"}]}`))
	h.SendText("Al café de hoy sumale 1070")

	// Sin esto los tests de abajo serían vacíos: arman el callback a mano, así
	// que pasarían aunque el gate no hubiera marcado nada. Lo que se exige acá
	// es que el RECIBO haya salido con los botones puestos.
	if !h.lastMarkupHas(nearDupPrefix) {
		t.Fatalf("el recibo salió sin los botones del gate. Markups: %v", h.rt.markups)
	}
}

// lastMarkupHas dice si algún reply_markup emitido contiene el fragmento.
func (h *convHarness) lastMarkupHas(want string) bool {
	for _, m := range h.rt.markups {
		if strings.Contains(m, want) {
			return true
		}
	}
	return false
}

// nearDupButton arma el callback del botón tal como lo emitió el recibo.
func (h *convHarness) nearDupButton(t *testing.T, action string) string {
	t.Helper()
	movs := h.Movements()
	if len(movs) != 2 {
		t.Fatalf("esperaba dos filas antes de tocar el botón, hay %d", len(movs))
	}
	return nearDupPrefix + action + ":" +
		strconv.FormatUint(uint64(movs[1].ID), 10) + ":" +
		strconv.FormatUint(uint64(movs[0].ID), 10)
}

func TestNearDuplicate_SumaloAEse_MergesAndDeletes(t *testing.T) {
	h := newConversationHarness(t)
	h.insertCoffeeThenCorrection(t)

	h.TapButton(h.nearDupButton(t, nearDupMerge))

	movs := h.Movements()
	if len(movs) != 1 {
		t.Fatalf("movimientos = %d, want 1: %+v", len(movs), movs)
	}
	if !movs[0].Amount.Equal(mustDec(t, "-13770")) {
		t.Errorf("amount = %s, want -13770 (12.700 + 1.070)", movs[0].Amount)
	}
}

func TestNearDuplicate_Reemplazalo_KeepsOnlyTheNewAmount(t *testing.T) {
	h := newConversationHarness(t)
	h.insertCoffeeThenCorrection(t)

	h.TapButton(h.nearDupButton(t, nearDupReplace))

	movs := h.Movements()
	if len(movs) != 1 {
		t.Fatalf("movimientos = %d, want 1: %+v", len(movs), movs)
	}
	if !movs[0].Amount.Equal(mustDec(t, "-1070")) {
		t.Errorf("amount = %s, want -1070", movs[0].Amount)
	}
}

// Ignorar el gate es SEGURO, y es la propiedad que lo hace shippeable: el
// resultado por default es exactamente el de hoy — dos filas, totales
// correctos. Sin esto, el gate sería un paso bloqueante disfrazado.
func TestNearDuplicate_IgnoringItLeavesTodaysBehaviour(t *testing.T) {
	h := newConversationHarness(t)
	h.insertCoffeeThenCorrection(t)

	// El usuario no toca nada y sigue hablando.
	h.ScriptToolCalls(recordMovementsCall(`{"movements":[
		{"type":"expense","amount":"500","currency":"ARS","category":"Alimentación",
		 "subcategory":"Supermercado","description":"Kiosco","date":"2026-08-12"}]}`))
	h.SendText("kiosco 500")

	movs := h.Movements()
	if len(movs) != 3 {
		t.Fatalf("movimientos = %d, want 3: ignorar el gate no puede cambiar nada", len(movs))
	}
	total := movs[0].Amount.Add(movs[1].Amount)
	if !total.Equal(mustDec(t, "-13770")) {
		t.Errorf("los dos cafés suman %s, want -13770: los totales quedan bien igual", total)
	}
}

func TestNearDuplicate_VaAparte_ChangesNothing(t *testing.T) {
	h := newConversationHarness(t)
	h.insertCoffeeThenCorrection(t)

	h.TapButton(h.nearDupButton(t, nearDupSeparte))

	if got := len(h.Movements()); got != 2 {
		t.Errorf("movimientos = %d, want 2", got)
	}
}
