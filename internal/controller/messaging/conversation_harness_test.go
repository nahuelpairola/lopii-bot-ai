//go:build conv_test

package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/database"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
	"lopiibot.com/internal/user"
)

type convHarness struct {
	t    *testing.T
	c    *controller
	chat *messenger.FakeChat
	orc  *fakeFullOrchestrator

	conn        *database.Connection
	userID      uint64
	telegramID  string
	telegramNum int64
	chatID      int64

	accounts map[string]uint64
	cache    *subcategory.Cache
}

func (h *convHarness) SeedAccount(name string, cur currency.Currency, opening string) uint64 {
	h.t.Helper()
	acc := &account.Account{UserID: h.userID, Name: name, Type: account.StandardType, Currency: cur}
	if err := movement.InitRepository(h.conn).InsertAccountsWithOpenings([]movement.AccountOpening{{
		Account: acc,
		Movement: movement.Movement{
			UserID: h.userID, SubcategoryID: 1, Date: time.Now(),
			Type: movement.Transfer, Amount: mustDec(h.t, opening), Currency: cur,
		},
	}}); err != nil {
		h.t.Fatalf("SeedAccount(%q): %v", name, err)
	}
	h.accounts[name] = uint64(acc.ID)
	return uint64(acc.ID)
}

func (h *convHarness) Balance(name string) decimal.Decimal {
	h.t.Helper()
	var total decimal.Decimal
	if err := h.conn.DB.Raw(
		`SELECT COALESCE(SUM(amount),0) FROM movements WHERE account_id = ? AND deleted_at IS NULL`,
		h.accounts[name],
	).Scan(&total).Error; err != nil {
		h.t.Fatalf("Balance(%q): %v", name, err)
	}
	return total
}

func (h *convHarness) SeedOwnCategory(category, sub, description, icon string) uint64 {
	h.t.Helper()
	s := &subcategory.Subcategory{
		UserID: &h.userID, Category: category, Subcategory: sub,
		Description: description, Icon: icon,
	}
	if err := h.cache.Insert(s); err != nil {
		h.t.Fatalf("SeedOwnCategory: %v", err)
	}
	if err := h.cache.Reload(); err != nil {
		h.t.Fatalf("SeedOwnCategory reload: %v", err)
	}
	return uint64(s.ID)
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

	tgNum := time.Now().UnixNano() % 1_000_000_000
	tgID := fmt.Sprintf("%d", tgNum)
	u := &user.User{}
	if err := userRepo.Insert(u); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := userRepo.LinkChannel(u.ID, user.ChannelTelegram, tgID); err != nil {
		t.Fatalf("link channel: %v", err)
	}
	uid := u.ID
	t.Cleanup(func() {
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&movement.Movement{})
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&account.Account{})
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&pendingaction.PendingAction{})
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&reminder.Reminder{})
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&subcategory.Subcategory{})
		conn.DB.Unscoped().Exec("DELETE FROM conversation_states WHERE user_id = ?", uid)
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&user.UserChannel{})
		conn.DB.Unscoped().Where("id = ?", uid).Delete(&user.User{})
	})

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

	chat := &messenger.FakeChat{}

	engine := conversation.NewEngine(conversation.NewRepository(conn), FlowResumeLabel)
	engine.Register(flow.NewMovementCreateFlow(cache, accRepo))
	engine.Register(flow.NewMovementUpdatePickFlow())
	engine.Register(flow.NewMovementUpdateConfirmFlow())
	engine.Register(flow.NewMovementDeleteFlow())
	engine.Register(flow.NewAccountCreateFlow())
	engine.Register(flow.NewAccountManageFlow(movRepo))
	engine.Register(flow.NewAccountMoveOfferFlow())
	engine.Register(flow.NewSubcategorySetupFlow(cache))
	engine.Register(flow.NewCategoryMatchOfferFlow())
	engine.Register(flow.NewCategoryProposalConfirmFlow())
	engine.Register(flow.NewCategoryManagePickFlow(cache))
	engine.Register(flow.NewCategoryManageTargetFlow(cache))
	engine.Register(flow.NewMovementNegativeConfirmFlow())
	engine.Register(flow.NewReminderSetupFlow())
	engine.Register(flow.NewAskUserFlow())

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
		t: t, c: c, chat: chat, orc: orc, conn: conn,
		userID: uid, telegramID: tgID, telegramNum: tgNum, chatID: 4242,
		accounts: map[string]uint64{"Banco Test": uint64(banco.ID)},
		cache:    cache,
	}
}

func (h *convHarness) ScriptIntent(orchestrator.Intent) {}

func (h *convHarness) ScriptCategory(pairs ...orchestrator.Pair) { h.orc.classifyPairs = pairs }

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

func (h *convHarness) SendText(text string) {
	h.t.Helper()
	if err := h.c.handleFreeText(context.Background(), h.chat, h.userID, text); err != nil {
		h.t.Fatalf("SendText(%q): %v", text, err)
	}
}

func (h *convHarness) TapButton(data string) {
	h.t.Helper()
	h.c.Handle(context.Background(), messenger.Incoming{
		Channel:       user.ChannelTelegram,
		ChannelUserID: h.telegramID,
		Chat:          h.chat,
		Input:         conversation.Input{CallbackData: data},
	})
}

func (h *convHarness) Movements() []movement.Movement {
	h.t.Helper()
	var out []movement.Movement
	for _, m := range h.AllMovements() {
		if m.Type == movement.Transfer && m.TransactionID == nil {
			continue
		}
		out = append(out, m)
	}
	return out
}

func (h *convHarness) AllMovements() []movement.Movement {
	h.t.Helper()
	var out []movement.Movement
	if err := h.conn.DB.Where("user_id = ?", h.userID).Order("id").Find(&out).Error; err != nil {
		h.t.Fatalf("AllMovements: %v", err)
	}
	return out
}

func (h *convHarness) Messages() []string {
	out := make([]string, len(h.chat.Sent))
	for i, p := range h.chat.Sent {
		out[i] = p.Text
	}
	return out
}

func (h *convHarness) LastMessage() string {
	return h.chat.LastText()
}

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

func TestConversation_DeleteConfirmedActuallyDeletes(t *testing.T) {
	h := newConversationHarness(t)
	id := h.SeedMovement("Café", "-12700", 1)

	h.ScriptIntent(orchestrator.IntentDelete)
	h.ScriptToolCalls(deleteMovementsCall(`{"reference":"el café de hoy"}`))
	h.SendText("borrá el café de hoy")

	if len(h.Movements()) != 1 {
		t.Fatalf("todavía no se confirmó nada: el movimiento tiene que seguir. Copia: %v", h.Messages())
	}

	h.TapButton(flow.OptionConfirm)

	movs := h.Movements()
	if len(movs) != 0 {
		t.Fatalf("el movimiento %d sigue vivo después de confirmar el borrado: %+v", id, movs)
	}
}

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

func (h *convHarness) ScriptUpdateResult(r orchestrator.UpdateResult) { h.orc.updateResult = r }

func TestConversation_CorrectionWithUnknownCategoryKeepsTheMovement(t *testing.T) {
	h := newConversationHarness(t)
	id := h.SeedMovement("Café", "-12700", 1)

	h.ScriptIntent(orchestrator.IntentUpdate)
	h.ScriptToolCalls(correctMovementCall(`{
		"change":"ponelo en proyecto hogar",
		"changes":[{"field":"category","op":"set","value":"proyecto hogar"}]}`))

	h.SendText("el café ponelo en proyecto hogar")
	h.TapButton(flow.OptionConfirm)

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

	for _, m := range h.Messages() {
		low := strings.ToLower(m)
		if strings.Contains(low, "borr") || strings.Contains(low, "elimin") {
			t.Errorf("se ofreció un borrado ante un pedido de edición: %q", m)
		}
	}

	h.TapButton(flow.OptionConfirm)
	movs := h.Movements()
	if len(movs) != 1 || movs[0].ID != id {
		t.Fatalf("el movimiento se perdió: %+v. Copia: %v", movs, h.Messages())
	}
	if movs[0].Amount.IsZero() {
		t.Error("quedó un movimiento en 0: el guard lo rechaza y no es un estado válido")
	}
}

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

	if !h.lastMarkupHas(flow.NearDupPrefix) {
		t.Fatalf("el recibo salió sin los botones del gate. Sent: %+v", h.chat.Sent)
	}
}

func (h *convHarness) lastMarkupHas(want string) bool {
	for _, p := range h.chat.Sent {
		for _, btn := range p.Buttons {
			if strings.Contains(btn.Data, want) {
				return true
			}
		}
	}
	return false
}

func (h *convHarness) nearDupButton(t *testing.T, action string) string {
	t.Helper()
	movs := h.Movements()
	if len(movs) != 2 {
		t.Fatalf("esperaba dos filas antes de tocar el botón, hay %d", len(movs))
	}
	return flow.NearDupPrefix + action + ":" +
		strconv.FormatUint(uint64(movs[1].ID), 10) + ":" +
		strconv.FormatUint(uint64(movs[0].ID), 10)
}

func TestNearDuplicate_SumaloAEse_MergesAndDeletes(t *testing.T) {
	h := newConversationHarness(t)
	h.insertCoffeeThenCorrection(t)

	h.TapButton(h.nearDupButton(t, flow.NearDupMerge))

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

	h.TapButton(h.nearDupButton(t, flow.NearDupReplace))

	movs := h.Movements()
	if len(movs) != 1 {
		t.Fatalf("movimientos = %d, want 1: %+v", len(movs), movs)
	}
	if !movs[0].Amount.Equal(mustDec(t, "-1070")) {
		t.Errorf("amount = %s, want -1070", movs[0].Amount)
	}
}

func TestNearDuplicate_IgnoringItLeavesTodaysBehaviour(t *testing.T) {
	h := newConversationHarness(t)
	h.insertCoffeeThenCorrection(t)

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

	h.TapButton(h.nearDupButton(t, flow.NearDupSeparte))

	if got := len(h.Movements()); got != 2 {
		t.Errorf("movimientos = %d, want 2", got)
	}
}
