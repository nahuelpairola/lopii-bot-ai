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
	"lopiibot.com/internal/metric"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingjob"
	"lopiibot.com/internal/subcategory"
	"lopiibot.com/internal/user"
)

// El drenaje de la cola de 429, de punta a punta contra el Postgres local:
// pendingjob.Run → drainUser → replayJob → handleFreeText → StartLoop →
// record_movements → INSERT. Es el único test del repo que prueba que un
// mensaje perdido por falta de cupo vuelve y la plata TERMINA EN LA BASE; los
// 15 unitarios de internal/pendingjob cubren encolar, diferir, rendirse y el
// flag de replay, pero todos con fakes y ninguno toca Postgres.
//
// El orchestrator va fakeado A PROPÓSITO. Hasta 2026-08-18 esto corría contra
// Groq real bajo el tag queue_eval, y esa versión mentía en las dos
// direcciones: sin GROQ_APIKEY se salteaba en silencio (verde que no probaba
// nada) y con la key se caía por el techo de 8.000 TPM o porque el modelo
// elegía parkear en vez de registrar (rojo que no era un bug). Peor: estuvo
// roto y mudo desde el 2026-08-12, cuando a419524 borró el router y mandó el
// replay por el loop unificado sin actualizar la config del eval, que siguió
// pasando el modelo del camino viejo.
//
// Con el tool call dictado, lo que se prueba es el CABLEADO —que es lo que el
// test siempre quiso probar— y se prueba en milisegundos, sin cupo y sin key.
// Qué tool elige el modelo es trabajo de los evals de internal/orchestrator.
//
// OJO: pendingjob.Run drena los jobs de TODOS los usuarios de la base que
// apunte, no sólo los de este test. Correr contra local/dev, nunca contra prod.

func queueDrainConn(t *testing.T) *database.Connection {
	conn, err := database.Initialize(database.Creds{
		Host: "localhost", Port: 5432, Name: "lopiibot", User: "lopiibot", Password: "lopiibot",
	}, false)
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	return conn
}

// queueDrainUser siembra un usuario descartable con un channel_user_id
// numérico: drainUser lo parsea con strconv.ParseInt, así que no puede ser el
// "qeval-<n>" que usa query_eval_test.go (ese test nunca llega a drainUser).
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
		// chat_turns tiene FK a users y el loop escribe una fila por turno. El
		// test viejo no la limpiaba: el DELETE del usuario fallaba con
		// query_turns_user_id_fkey y dejaba usuarios colgados en la base de dev,
		// uno por corrida. GORM no lo gritaba porque el error del Delete no se
		// chequea acá — el cleanup es best-effort a propósito, así que la única
		// defensa es borrar en el orden correcto.
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&chathistory.ChatTurn{})
		// conversation_states va por el repositorio, no por SQL suelto: la regla
		// del repo es que a esa tabla se entra sólo por conversation.
		_ = conversation.NewRepository(conn).Clear(uid)
		// intent_events tiene FK a users. Este controller va con metrics=nil, así
		// que no escribe ahí; se limpia igual por si otra corrida concurrente
		// contra la misma base dejó una fila y el delete del usuario se traba.
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&metric.IntentEvent{})
		// user_channels tiene FK a users: hay que borrarla antes que la fila de
		// usuario o el DELETE final falla igual que las otras tablas de arriba.
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

	banco := &account.Account{UserID: uid, Name: "Banco QueueDrain", Type: account.StandardType, Currency: currency.ARS, IsDefault: true}
	if err := accRepo.Insert(banco); err != nil {
		t.Fatalf("insert banco: %v", err)
	}
	bancoID := uint64(banco.ID)

	// Saldo de apertura: con saldo 0 el gasto deja la cuenta en negativo y
	// dispara el gate movement_negative_confirm en vez de insertar derecho.
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

	// El engine y el flujo de alta son los REALES, como en server.go: si el loop
	// terminara parkeando, el parkeo tiene dónde caer en vez de explotar.
	engine := conversation.NewEngine(conversation.NewRepository(conn), FlowResumeLabel)
	engine.Register(flow.NewMovementCreateFlow(cache, accRepo))

	// El fake dicta el tool call. classifyPairs es obligatorio: record_movements
	// ya no recibe la categoría del loop —la asigna e.classify()— y sin un par
	// el movimiento cae en PENDING_REVIEW, abre gap y el turno parkea en vez de
	// insertar.
	orch := &fakeFullOrchestrator{
		classifyPairs: []orchestrator.Pair{{Category: "Alimentación", Subcategory: "Supermercado"}},
		runFn: func(execute func(string, json.RawMessage) (string, error)) (string, error) {
			args := `{"movements":[{"type":"expense","amount":"500","currency":"ARS",` +
				`"date":"` + time.Now().Format("2006-01-02") + `",` +
				`"description":"super","payment_method":"transfer"}]}`
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
	go pendingjob.Run(ctx, c.PendingJobServices(), jobsRepo, nil, 50*time.Millisecond)

	// Sin Groq de por medio el drenaje es inmediato; el deadline corto está para
	// que un cuelgue falle rápido en vez de comerse el timeout del paquete.
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
	// El signo ES la contabilidad: un gasto guardado en positivo infla el saldo.
	if !movs[0].Amount.IsNegative() {
		t.Errorf("el gasto quedó en %s: tiene que guardarse negativo", movs[0].Amount)
	}
	if movs[0].AccountID == nil || *movs[0].AccountID != bancoID {
		t.Errorf("account_id = %v, want %d: un gasto sin cuenta no le baja el saldo a nadie", movs[0].AccountID, bancoID)
	}
}

// Un job viejo se descarta SIN llamar al modelo: reintentarlo gastaría cupo por
// un mensaje que el usuario ya dio por perdido. El orchestrator va nil a
// propósito — si el drenaje lo tocara, el test panichea en vez de pasar.
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
	go pendingjob.Run(ctx, c.PendingJobServices(), jobsRepo, nil, 50*time.Millisecond)

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
