//go:build integration

package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/chathistory"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/database"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/metric"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
	"lopiibot.com/internal/pendingjob"
	"lopiibot.com/internal/subcategory"
	"lopiibot.com/internal/user"
)

type fakeChats struct{ chat *messenger.FakeChat }

func (f fakeChats) ChatFor(uint64) (messenger.Chat, error) { return f.chat, nil }

func queueDrainConn(t *testing.T) *database.Connection {
	conn, err := database.Initialize(database.Creds{
		Host: "localhost", Port: 5432, Name: "lopiibot", User: "lopiibot", Password: "lopiibot",
	}, false)
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	return conn
}

func queueDrainUser(t *testing.T, conn *database.Connection) (uid uint64, cleanup func()) {
	userRepo := user.NewRepository(conn)
	telegramID := fmt.Sprintf("%d", time.Now().UnixNano())
	u := &user.User{}
	if err := userRepo.Insert(u); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := userRepo.LinkChannel(u.ID, user.ChannelTelegram, telegramID); err != nil {
		t.Fatalf("link channel: %v", err)
	}
	uid = u.ID
	return uid, func() {
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&pendingjob.PendingJob{})
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&movement.Movement{})
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&account.Account{})
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&chathistory.ChatTurn{})
		_ = conversation.NewRepository(conn).Clear(uid)
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&metric.IntentEvent{})
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&user.UserChannel{})
		conn.DB.Unscoped().Where("id = ?", uid).Delete(&user.User{})
	}
}

func TestQueueDrain_ReplayInsertsTheMovement(t *testing.T) {
	conn := queueDrainConn(t)

	accRepo := account.NewRepository(conn)
	movRepo := movement.InitRepository(conn)
	subRepo := subcategory.NewRepository(conn)
	cache, err := subcategory.NewCache(subRepo)
	if err != nil {
		t.Fatalf("subcategory cache: %v", err)
	}
	jobsRepo := pendingjob.NewRepository(conn)

	uid, cleanup := queueDrainUser(t, conn)
	t.Cleanup(cleanup)

	banco := &account.Account{UserID: uid, Name: "Banco QueueDrain", Currency: currency.ARS, IsDefault: true}
	if err := accRepo.Insert(banco); err != nil {
		t.Fatalf("insert banco: %v", err)
	}
	bancoID := uint64(banco.ID)

	saldoInicial, err := cache.FindByCategoryAndSubcategory(uid, "Sistema", "Saldo inicial")
	if err != nil {
		t.Fatalf("find Sistema|Saldo inicial: %v", err)
	}
	opening := movement.Movement{
		UserID: uid, AccountID: &bancoID, SubcategoryID: uint64(saldoInicial.ID),
		Date: time.Now(), Type: movement.Transfer, Amount: decimal.RequireFromString("10000"), Currency: currency.ARS,
	}
	if err := movRepo.InsertBatch([]movement.Movement{opening}); err != nil {
		t.Fatalf("insert saldo de apertura: %v", err)
	}

	engine := conversation.NewEngine(conversation.NewRepository(conn), FlowResumeLabel)
	engine.Register(flow.NewMovementCreateFlow(cache, accRepo))

	orch := &fakeFullOrchestrator{
		classifyPairs: []orchestrator.Pair{{Category: "Alimentación", Subcategory: "Supermercado"}},
		runFn: func(execute func(string, json.RawMessage) (string, error)) (string, error) {
			args := `{"movements":[{"type":"expense","amount":"500","currency":"ARS",` +
				`"date":"` + time.Now().Format("2006-01-02") + `",` +
				`"description":"super"}]}`
			return execute(orchestrator.ToolRecordMovements, json.RawMessage(args))
		},
	}

	chatHist := chathistory.InitRepository(conn, 24*time.Hour, 20)
	c := &controller{
		users: user.NewRepository(conn), accounts: accRepo, movements: movRepo,
		subcategories: cache, engine: engine, orchestrator: orch, jobs: jobsRepo, chatHistory: chatHist,
	}

	payload, _ := json.Marshal(pendingjob.FreeTextPayload{Text: "gasté 500 en el super"})
	job := &pendingjob.PendingJob{UserID: uid, Kind: pendingjob.KindFreeText, Payload: payload}
	if err := jobsRepo.Insert(job); err != nil {
		t.Fatalf("insert pending job: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pendingjob.Run(ctx, c, jobsRepo, fakeChats{chat: &messenger.FakeChat{}}, 50*time.Millisecond)

	deadline := time.Now().Add(10 * time.Second)
	for {
		n, err := jobsRepo.CountByUser(uid)
		if err != nil {
			t.Fatalf("count pending jobs: %v", err)
		}
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("el job quedó pendiente después de 10s (count=%d): el drenaje no lo tomó", n)
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !orch.runCalled {
		t.Fatal("el replay no llegó al loop: se drenó el job sin ejecutar nada")
	}

	var movs []movement.Movement
	if err := conn.DB.Where("user_id = ? AND type = ?", uid, movement.Expense).Find(&movs).Error; err != nil {
		t.Fatalf("find movements: %v", err)
	}
	if len(movs) != 1 {
		t.Fatalf("want 1 gasto insertado por el job replayado, got %d: %+v", len(movs), movs)
	}
	if want := decimal.RequireFromString("500"); !movs[0].Amount.Abs().Equal(want) {
		t.Errorf("want abs(amount)=%s, got %s", want, movs[0].Amount)
	}
	if !movs[0].Amount.IsNegative() {
		t.Errorf("el gasto quedó en %s: tiene que guardarse negativo", movs[0].Amount)
	}
	if movs[0].AccountID == nil || *movs[0].AccountID != bancoID {
		t.Errorf("account_id = %v, want %d: un gasto sin cuenta no le baja el saldo a nadie", movs[0].AccountID, bancoID)
	}
}

func TestQueueDrain_ReplayAskUserGapSendsThePrompt(t *testing.T) {
	conn := queueDrainConn(t)

	accRepo := account.NewRepository(conn)
	movRepo := movement.InitRepository(conn)
	subRepo := subcategory.NewRepository(conn)
	cache, err := subcategory.NewCache(subRepo)
	if err != nil {
		t.Fatalf("subcategory cache: %v", err)
	}
	jobsRepo := pendingjob.NewRepository(conn)
	actionsRepo := pendingaction.NewRepository(conn)

	uid, cleanup := queueDrainUser(t, conn)
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&pendingaction.PendingAction{})
	})

	banco := &account.Account{UserID: uid, Name: "Banco AskUserGap", Currency: currency.ARS, IsDefault: true}
	if err := accRepo.Insert(banco); err != nil {
		t.Fatalf("insert banco: %v", err)
	}
	bancoID := uint64(banco.ID)

	saldoInicial, err := cache.FindByCategoryAndSubcategory(uid, "Sistema", "Saldo inicial")
	if err != nil {
		t.Fatalf("find Sistema|Saldo inicial: %v", err)
	}
	opening := movement.Movement{
		UserID: uid, AccountID: &bancoID, SubcategoryID: uint64(saldoInicial.ID),
		Date: time.Now(), Type: movement.Transfer, Amount: decimal.RequireFromString("10000"), Currency: currency.ARS,
	}
	if err := movRepo.InsertBatch([]movement.Movement{opening}); err != nil {
		t.Fatalf("insert saldo de apertura: %v", err)
	}

	comida, err := cache.FindByCategoryAndSubcategory(uid, "Alimentación", "Supermercado")
	if err != nil {
		t.Fatalf("find Alimentación|Supermercado: %v", err)
	}
	desc1, desc2 := "compra en el supermercado", "carga de nafta en la estación"
	rows := []movement.Movement{
		{UserID: uid, AccountID: &bancoID, SubcategoryID: uint64(comida.ID), Date: time.Now(),
			Type: movement.Expense, Amount: decimal.RequireFromString("-500"), Currency: currency.ARS, Description: &desc1},
		{UserID: uid, AccountID: &bancoID, SubcategoryID: uint64(comida.ID), Date: time.Now(),
			Type: movement.Expense, Amount: decimal.RequireFromString("-300"), Currency: currency.ARS, Description: &desc2},
	}
	if err := movRepo.InsertBatch(rows); err != nil {
		t.Fatalf("insert los dos gastos ambiguos: %v", err)
	}
	old := time.Now().Add(-20 * time.Minute)
	if err := conn.DB.Model(&movement.Movement{}).Where("id IN ?", []uint{rows[0].ID, rows[1].ID}).Update("created_at", old).Error; err != nil {
		t.Fatalf("age the two movements: %v", err)
	}

	engine := conversation.NewEngine(conversation.NewRepository(conn), FlowResumeLabel)
	engine.Register(flow.NewAskUserFlow())

	orch := &fakeFullOrchestrator{
		runFn: func(execute func(string, json.RawMessage) (string, error)) (string, error) {
			args := `{"change":"corregir el monto","changes":[{"field":"amount","op":"set","value":"700"}]}`
			return execute(orchestrator.ToolCorrectMovement, json.RawMessage(args))
		},
	}

	chatHist := chathistory.InitRepository(conn, 24*time.Hour, 20)
	c := &controller{
		users: user.NewRepository(conn), accounts: accRepo, movements: movRepo,
		subcategories: cache, engine: engine, orchestrator: orch, jobs: jobsRepo,
		chatHistory: chatHist, actions: actionsRepo,
	}

	payload, _ := json.Marshal(pendingjob.FreeTextPayload{Text: "che, corregime el monto a setecientos"})
	job := &pendingjob.PendingJob{UserID: uid, Kind: pendingjob.KindFreeText, Payload: payload}
	if err := jobsRepo.Insert(job); err != nil {
		t.Fatalf("insert pending job: %v", err)
	}

	chat := &messenger.FakeChat{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pendingjob.Run(ctx, c, jobsRepo, fakeChats{chat: chat}, 50*time.Millisecond)

	deadline := time.Now().Add(10 * time.Second)
	for {
		n, err := jobsRepo.CountByUser(uid)
		if err != nil {
			t.Fatalf("count pending jobs: %v", err)
		}
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("el job quedó pendiente después de 10s (count=%d): el drenaje no lo tomó", n)
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !orch.runCalled {
		t.Fatal("el replay no llegó al loop: se drenó el job sin ejecutar nada")
	}

	if len(chat.Sent) == 0 {
		t.Fatal("el usuario no recibió NADA: el gap-fill se perdió en silencio (el bug de la ronda 1)")
	}
	last := chat.Sent[len(chat.Sent)-1]
	if len(last.Buttons) == 0 {
		t.Fatalf("el último mensaje no trae el picker de candidatos: %+v", last)
	}

	pending, err := actionsRepo.NextForUser(uid)
	if err != nil {
		t.Fatalf("expected a pending action open, got err: %v", err)
	}
	if pending.Tool != orchestrator.ToolCorrectMovement {
		t.Errorf("pending action tool = %q, want %q", pending.Tool, orchestrator.ToolCorrectMovement)
	}
}

func TestQueueDrain_GiveUp_DoesNotCallOrchestrator(t *testing.T) {
	conn := queueDrainConn(t)
	jobsRepo := pendingjob.NewRepository(conn)

	uid, cleanup := queueDrainUser(t, conn)
	t.Cleanup(cleanup)

	payload, _ := json.Marshal(pendingjob.FreeTextPayload{Text: "gasté 999 en el super"})
	job := &pendingjob.PendingJob{UserID: uid, Kind: pendingjob.KindFreeText, Payload: payload}
	if err := jobsRepo.Insert(job); err != nil {
		t.Fatalf("insert pending job: %v", err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := conn.DB.Model(&pendingjob.PendingJob{}).Where("id = ?", job.ID).Update("created_at", old).Error; err != nil {
		t.Fatalf("age the job: %v", err)
	}

	c := &controller{users: user.NewRepository(conn), jobs: jobsRepo}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pendingjob.Run(ctx, c, jobsRepo, fakeChats{chat: &messenger.FakeChat{}}, 50*time.Millisecond)

	deadline := time.Now().Add(10 * time.Second)
	for {
		n, err := jobsRepo.CountByUser(uid)
		if err != nil {
			t.Fatalf("count pending jobs: %v", err)
		}
		if n == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("el job viejo sigue en la cola después de 10s (count=%d)", n)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
