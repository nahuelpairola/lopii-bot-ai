//go:build queue_eval

package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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

// End-to-end pending-jobs-queue test against REAL Groq + the local Docker
// Postgres. Forcing a genuine 429 from Groq on demand isn't reliable (would
// mean deliberately hammering the API or an invalid key, which gives 401, not
// 429) — that enqueue-on-429 branch is already covered by the mocked unit
// tests (pending_jobs_test.go). What those mocks CAN'T cover is whether a
// queued job, once drained, actually reaches a working real Groq call and a
// real DB insert through the production code path (drainTick -> drainUser ->
// replayJob -> handleFreeText). This test seeds a job directly and proves
// that path end to end.
//
// drainTick processes every pending job for every user in the target
// Postgres instance, not just this test's — run this against a local/dev DB
// you don't mind draining, never prod.
//
// Run:
//   set -a; . ./.env; set +a
//   go test -tags queue_eval ./internal/controller/messaging/ -run TestQueueEval -v
// Excluded from the default `go test ./...` (no tag) — needs a live DB + key.

// queueEvalOrchestrator skips the test (t.Skip) if no key is set; otherwise
// returns a ready-to-use real Groq config.
func queueEvalOrchestrator(t *testing.T) orchestrator.Config {
	key := os.Getenv("GROQ_APIKEY")
	if key == "" {
		t.Skip("GROQ_APIKEY unset — real-LLM eval skipped")
	}
	baseURL := os.Getenv("GROQ_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.groq.com/openai/v1"
	}
	model := os.Getenv("GROQ_CREATE_MODEL")
	if model == "" {
		model = "openai/gpt-oss-20b"
	}
	// AgentModel va sí o sí: el replay de un free_text entra por el loop
	// unificado (orchestrator.Run), no por el camino de CREATE. Sin esto el
	// request sale con model:"" y Groq contesta 404 «The model `` does not
	// exist», o sea el eval fallaba sin llegar a probar el drenaje. Es el mismo
	// descuido que fc300ab: la config del eval quedó atada al modelo que usaba
	// el camino viejo.
	return orchestrator.Config{
		APIKey: key, BaseURL: baseURL, CreateModel: model, AgentModel: model, TimeoutSeconds: 30,
	}
}

func queueEvalConn(t *testing.T) *database.Connection {
	conn, err := database.Initialize(database.Creds{
		Host: "localhost", Port: 5432, Name: "lopiibot", User: "lopiibot", Password: "lopiibot",
	}, false)
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	return conn
}

// queueEvalUser seeds a throwaway user with a numeric TelegramID — drainUser
// parses it with strconv.ParseInt, so it can't be the "qeval-<n>" style
// prefix query_eval_test.go uses (that test never reaches drainUser).
func queueEvalUser(t *testing.T, conn *database.Connection) (uid uint64, cleanup func()) {
	u := &user.User{TelegramID: fmt.Sprintf("%d", time.Now().UnixNano())}
	if err := user.NewRepository(conn).Insert(u); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	uid = u.ID
	return uid, func() {
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&pendingjob.PendingJob{})
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&movement.Movement{})
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&account.Account{})
		// intent_events FKs to users — this test's controller has metrics=nil so it
		// never writes here itself, but a concurrent run against the same DB
		// (another controller with metrics wired) could; clean up defensively so
		// the final user delete never fails on a stray FK.
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&metric.IntentEvent{})
		conn.DB.Unscoped().Where("id = ?", uid).Delete(&user.User{})
	}
}

func TestQueueEval_DrainReplaysFreeText(t *testing.T) {
	cfg := queueEvalOrchestrator(t)
	conn := queueEvalConn(t)

	userRepo := user.NewRepository(conn)
	accRepo := account.NewRepository(conn)
	movRepo := movement.InitRepository(conn)
	subRepo := subcategory.NewRepository(conn)
	cache, err := subcategory.NewCache(subRepo)
	if err != nil {
		t.Fatalf("subcategory cache: %v", err)
	}
	jobsRepo := pendingjob.NewRepository(conn)

	uid, cleanup := queueEvalUser(t, conn)
	t.Cleanup(cleanup)

	banco := &account.Account{UserID: uid, Name: "Banco QueueEval", Type: account.StandardType, Currency: currency.ARS, IsDefault: true}
	if err := accRepo.Insert(banco); err != nil {
		t.Fatalf("insert banco: %v", err)
	}
	bancoID := uint64(banco.ID)

	// Give the account an opening balance — with balance 0, any expense drives
	// it negative and trips the movement_negative_confirm gate instead of
	// inserting directly, which this test isn't wired to click through.
	saldoInicial, err := cache.FindByCategoryAndSubcategory(uid, "Sistema", "Saldo inicial")
	if err != nil {
		t.Fatalf("find Sistema|Saldo inicial subcategory: %v", err)
	}
	opening := movement.Movement{
		UserID: uid, AccountID: &bancoID, SubcategoryID: uint64(saldoInicial.ID),
		Date: time.Now(), Type: movement.Transfer, Amount: decimal.RequireFromString("10000"), Currency: currency.ARS,
	}
	if err := movRepo.InsertBatch([]movement.Movement{opening}); err != nil {
		t.Fatalf("insert opening balance: %v", err)
	}

	// Real classification can land on the gap-fill flow instead of the
	// frictionless zero-gap insert (account/category ambiguity is a real LLM
	// call, not deterministic) — wire the real engine + movement_create flow,
	// same as server.go, so either branch works instead of nil-panicking.
	engine := conversation.NewEngine(conversation.NewRepository(conn), FlowResumeLabel)
	engine.Register(flow.NewMovementCreateFlow(cache, accRepo))

	orch := orchestrator.New(cfg)
	chatHist := chathistory.InitRepository(conn, 24*time.Hour, 20)
	c := &controller{users: userRepo, accounts: accRepo, movements: movRepo, subcategories: cache, engine: engine, orchestrator: orch, jobs: jobsRepo, chatHistory: chatHist}

	payload, _ := json.Marshal(pendingjob.FreeTextPayload{Text: "gasté 500 en el super"})
	job := &pendingjob.PendingJob{UserID: uid, Kind: pendingjob.KindFreeText, Payload: payload}
	if err := jobsRepo.Insert(job); err != nil {
		t.Fatalf("insert pending job: %v", err)
	}

	// A real 429 against a rate-limited/shared key defers the job instead of
	// draining it (drainUser: "drain: deferred (429)") — correct behavior, not
	// a bug, but it means one drainTick isn't guaranteed to finish under real
	// conditions. Retry for up to 90s, same tolerance query_eval_test.go uses
	// for real-Groq flakiness.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pendingjob.Run(ctx, c, jobsRepo, nil, 100*time.Millisecond)

	deadline := time.Now().Add(90 * time.Second)
	for {
		n, err := jobsRepo.CountByUser(uid)
		if err != nil {
			t.Fatalf("count pending jobs: %v", err)
		}
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job still pending after 90s (persistent real 429/rate limit?), count=%d", n)
		}
		t.Logf("job still pending (real 429 deferred it) — retrying in 5s")
		time.Sleep(5 * time.Second)
	}

	var movs []movement.Movement
	if err := conn.DB.Where("user_id = ? AND type = ?", uid, movement.Expense).Find(&movs).Error; err != nil {
		t.Fatalf("find movements: %v", err)
	}
	if len(movs) != 1 {
		t.Fatalf("want exactly 1 expense inserted by the replayed job, got %d: %+v", len(movs), movs)
	}
	want := decimal.RequireFromString("500")
	if !movs[0].Amount.Abs().Equal(want) {
		t.Errorf("want abs(amount)=%s, got %s", want, movs[0].Amount)
	}
}

func TestQueueEval_GiveUp_DoesNotCallOrchestrator(t *testing.T) {
	queueEvalOrchestrator(t) // gate on the same key requirement; config itself unused (orchestrator stays nil below)
	conn := queueEvalConn(t)
	userRepo := user.NewRepository(conn)
	jobsRepo := pendingjob.NewRepository(conn)

	uid, cleanup := queueEvalUser(t, conn)
	t.Cleanup(cleanup)

	payload, _ := json.Marshal(pendingjob.FreeTextPayload{Text: "gasté 999 en el super"})
	job := &pendingjob.PendingJob{UserID: uid, Kind: pendingjob.KindFreeText, Payload: payload}
	if err := jobsRepo.Insert(job); err != nil {
		t.Fatalf("insert pending job: %v", err)
	}
	// Backdate past maxJobAge so drainUser gives up instead of replaying —
	// orchestrator is left nil below, so a real call here would panic.
	old := time.Now().Add(-3 * time.Hour)
	if err := conn.DB.Model(&pendingjob.PendingJob{}).Where("id = ?", job.ID).Update("created_at", old).Error; err != nil {
		t.Fatalf("backdate job: %v", err)
	}

	c := &controller{users: userRepo, jobs: jobsRepo}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pendingjob.Run(ctx, c, jobsRepo, nil, 100*time.Millisecond)
	time.Sleep(500 * time.Millisecond) // one tick
	cancel()

	if n, err := jobsRepo.CountByUser(uid); err != nil || n != 0 {
		t.Fatalf("want gave-up job deleted, got count=%d err=%v", n, err)
	}
}
