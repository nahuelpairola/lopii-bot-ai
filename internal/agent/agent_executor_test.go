package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)

func candidateMovement(id uint, txID *uuid.UUID, description string, amount int64) movement.Movement {
	now := time.Now()
	return movement.Movement{
		Model:         gorm.Model{ID: id, CreatedAt: now},
		TransactionID: txID,
		Date:          now,
		Type:          constants.Expense,
		Amount:        decimal.NewFromInt(-amount),
		Currency:      currency.ARS,
		Description:   strPtr(description),
		Subcategory:   &subcategory.Subcategory{Category: "Comida", Subcategory: "Supermercado"},
	}
}

func newExecutorWith(t *testing.T, userText string, movements ...movement.Movement) *agentExecutor {
	t.Helper()
	return newAgentExecutor(context.Background(), &fakeServices{
		movements:   &fakeMovementRepoFull{similar: movements},
		chatHistory: &stubChatHistory{},
	}, 1, userText, nil)
}

func taxonomyForTest() []orchestrator.TaxonomyEntry {
	return []orchestrator.TaxonomyEntry{{Category: "Alimentación", Subcategory: "Supermercado"}}
}

func accountsWithDefault() *fakeAccountRepoFull {
	acc := &account.Account{Model: gorm.Model{ID: 1}, Name: "Mercado Pago", Currency: currency.ARS, IsDefault: true}
	return &fakeAccountRepoFull{
		byCurrency: map[currency.Currency]*account.Account{currency.ARS: acc},
		byUserID:   []account.Account{*acc},
		byID:       map[uint64]*account.Account{1: acc},
	}
}

func movementsWithBalance(balance string) *fakeMovementRepoFull {
	return &fakeMovementRepoFull{balances: map[uint64]string{1: balance}}
}

func subcategoriesForTest() *fakeSubcategoryRepoFull {
	sub := subcategory.Subcategory{Model: gorm.Model{ID: 1}, Category: "Alimentación", Subcategory: "Supermercado"}
	return &fakeSubcategoryRepoFull{
		byCategoryAndSub: map[string]*subcategory.Subcategory{"Alimentación|Supermercado": &sub},
		all:              []subcategory.Subcategory{sub},
	}
}

func newCreateExecutor(t *testing.T, balance, userText string, pairs ...orchestrator.Pair) *agentExecutor {
	t.Helper()
	if pairs == nil {
		pairs = []orchestrator.Pair{{Category: "Alimentación", Subcategory: "Supermercado"}}
	}
	svc := &fakeServices{
		movements:     movementsWithBalance(balance),
		accounts:      accountsWithDefault(),
		subcategories: subcategoriesForTest(),
		orch:          &fakeOrchestrator{classifyPairs: pairs},
		chatHistory:   &stubChatHistory{},
	}
	return newAgentExecutor(context.Background(), svc, 1, userText, taxonomyForTest())
}

func TestAgentExecutor_CleanCreateInsertsAndOwnsTheTurn(t *testing.T) {
	e := newCreateExecutor(t, "100000", "gasté 5000 en el super")

	args := `{"movements":[{"type":"expense","amount":"5000","currency":"ARS",
		"category":"Alimentación","subcategory":"Supermercado","date":"2026-08-01",
		"description":"super en Coto","payment_method":"transfer"}]}`
	out := executeDone(t, e, orchestrator.ToolRecordMovements, args)

	if !e.wrote {
		t.Error("wrote quedó en false: un 429 posterior encolaría y duplicaría")
	}
	if len(e.inserted) != 1 {
		t.Fatalf("insertó %d movimientos, want 1", len(e.inserted))
	}
	if out != resultRecorded(1) {
		t.Errorf("out = %q, want %q", out, resultRecorded(1))
	}
	if len(e.parked) != 0 {
		t.Errorf("un CREATE limpio no parkea nada: %+v", e.parked)
	}
	if e.reply == "" {
		t.Error("falta el recibo; sin él el usuario no ve nada")
	}
}

func executeDone(t *testing.T, e *agentExecutor, tool, args string) string {
	t.Helper()
	out, err := e.execute(tool, json.RawMessage(args))
	if !errors.Is(err, orchestrator.ErrAgentTurnDone) {
		t.Fatalf("%s: want ErrAgentTurnDone, got %v", tool, err)
	}
	return out
}

func TestAgentExecutor_CorrectResolvesInOneRound(t *testing.T) {
	e := newExecutorWith(t, "la panaderia era 2000", candidateMovement(10, nil, "compra en panadería", 3000))

	out := executeDone(t, e, orchestrator.ToolCorrectMovement, `{"change":"eran 2000"}`)
	if out != resultParked {
		t.Fatalf("want %q, got %q", resultParked, out)
	}
	if len(e.parked) != 1 {
		t.Fatalf("want 1 parked action, got %d", len(e.parked))
	}
	a := e.parked[0]
	if a.Tool != orchestrator.ToolCorrectMovement {
		t.Errorf("tool equivocada: %q", a.Tool)
	}
	if len(a.Questions) != 0 {
		t.Errorf("con un solo candidato no se pregunta nada: %+v", a.Questions)
	}
	if a.Payload.Chosen != 0 {
		t.Errorf("el candidato tenía que quedar elegido: %+v", a.Payload)
	}
	if a.Payload.Change != "eran 2000" {
		t.Errorf("se perdió el cambio pedido: %q", a.Payload.Change)
	}
	if got := a.Payload.Candidates[0].OldIDs; len(got) != 1 || got[0] != "10" {
		t.Errorf("los oldIDs no salieron de resolveCandidates: %+v", got)
	}
}

func TestAgentExecutor_InsufficientFundsParksTheGate(t *testing.T) {
	e := newCreateExecutor(t, "100", "gasté 50000 en el super")

	args := `{"movements":[{"type":"expense","amount":"50000","currency":"ARS",
		"category":"Alimentación","subcategory":"Supermercado","date":"2026-08-01",
		"description":"super","payment_method":"transfer"}]}`
	executeDone(t, e, orchestrator.ToolRecordMovements, args)

	if e.wrote {
		t.Error("wrote quedó en true sin haber insertado: bloquearía la cola del 429 sin razón")
	}
	if len(e.parked) != 1 || e.parked[0].Payload.Seed[conversation.KeyGatePrompt] == nil {
		t.Fatalf("tenía que parkear el gate con su copy: %+v", e.parked)
	}
	if e.parked[0].Tool != orchestrator.ToolRecordMovements {
		t.Errorf("el gate retoma un CREATE, no otra cosa: %q", e.parked[0].Tool)
	}
}

func TestAgentExecutor_SearchesWithTheUsersTextNotTheModelParaphrase(t *testing.T) {
	e := newExecutorWith(t, "la panaderia era 2000", candidateMovement(10, nil, "compra en panadería", 3000))

	executeDone(t, e, orchestrator.ToolCorrectMovement, `{"change":"cambiar el monto a 2000"}`)
	if len(e.parked) != 1 {
		t.Fatalf("tenía que encontrarlo por el texto del usuario, got %d parked", len(e.parked))
	}
}

func TestAgentExecutor_TwoCandidatesAsksWhich(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	e := newExecutorWith(t, "la panaderia estaba mal",
		candidateMovement(10, &a, "compra en panadería", 3000),
		candidateMovement(11, &b, "panadería del barrio", 5000),
	)

	out := executeDone(t, e, orchestrator.ToolCorrectMovement, `{"change":"estaba mal"}`)
	if out != resultParked {
		t.Fatalf("want %q, got %q", resultParked, out)
	}
	action := e.parked[0]
	if action.Payload.Chosen != -1 {
		t.Errorf("con dos candidatos no se puede elegir solo: %+v", action.Payload)
	}
	if len(action.Questions) != 1 || action.Questions[0].Key != questionKeyCandidate {
		t.Fatalf("tiene que preguntar cuál: %+v", action.Questions)
	}
	if len(action.Questions[0].Options) != 2 {
		t.Errorf("las opciones son los candidatos: %+v", action.Questions[0].Options)
	}
}

func TestAgentExecutor_NothingToCorrectDoesNotPark(t *testing.T) {
	e := newExecutorWith(t, "corregí el asado")
	out := executeDone(t, e, orchestrator.ToolCorrectMovement, `{"change":"corregí el asado"}`)
	if out != resultNoCandidates {
		t.Errorf("want %q, got %q", resultNoCandidates, out)
	}
	if len(e.parked) != 0 {
		t.Errorf("sin candidatos no se parkea nada: %+v", e.parked)
	}
	if !e.noCandidates {
		t.Error("falta la marca de que no había candidatos")
	}
	const wantReply = "No tengo movimientos de ese día para tocar. ¿De qué fecha era?"
	if e.reply != wantReply {
		t.Errorf("want %q, got %q", wantReply, e.reply)
	}
}

func TestAgentExecutor_BadArgsStillPark(t *testing.T) {
	e := newExecutorWith(t, "la panaderia era 2000", candidateMovement(10, nil, "compra en panadería", 3000))
	executeDone(t, e, orchestrator.ToolCorrectMovement, `no soy json`)
	if len(e.parked) != 1 {
		t.Fatalf("tenía que parkear igual, got %d", len(e.parked))
	}
}

func TestAgentExecutor_DeleteParksToo(t *testing.T) {
	e := newExecutorWith(t, "borrá lo de la panaderia", candidateMovement(10, nil, "compra en panadería", 3000))
	executeDone(t, e, orchestrator.ToolDeleteMovements, `{}`)
	if len(e.parked) != 1 || e.parked[0].Tool != orchestrator.ToolDeleteMovements {
		t.Fatalf("delete no parkeó: %+v", e.parked)
	}
	if e.parked[0].Payload.Chosen != 0 {
		t.Errorf("con un solo candidato tenía que quedar elegido: %+v", e.parked[0].Payload)
	}
}

func TestAgentExecutor_HelpAndRewriteResolveInTurn(t *testing.T) {
	for _, tc := range []struct {
		tool string
		want string
	}{
		{orchestrator.ToolReplyHelp, msgHelp},
		{orchestrator.ToolAskRewrite, MsgAskRewrite},
	} {
		e := newExecutorWith(t, "¿qué podés hacer?")
		executeDone(t, e, tc.tool, `{}`)
		if e.reply != tc.want {
			t.Errorf("%s: want reply %q, got %q", tc.tool, tc.want, e.reply)
		}
		if len(e.parked) != 0 {
			t.Errorf("%s: se resuelve en el turno, no se parkea: %+v", tc.tool, e.parked)
		}
	}
}

func TestAgentExecutor_UnwiredToolsSayNotAvailable(t *testing.T) {
	e := newExecutorWith(t, "cualquier cosa")
	for _, tool := range []string{
		orchestrator.ToolManageAccount,
		orchestrator.ToolSumMovements,
	} {
		out, err := e.execute(tool, json.RawMessage(`{}`))
		if err != nil {
			t.Fatalf("%s: %v", tool, err)
		}
		if out != resultNotWiredYet {
			t.Errorf("%s: want %q, got %q", tool, resultNotWiredYet, out)
		}
	}
}

func TestClassify_TheClassifierBeatsTheStructuralDefault(t *testing.T) {
	movs := []orchestrator.MovementDraft{
		{Type: "transfer", Amount: "-3100000", Currency: "ARS"},
		{Type: "transfer", Amount: "3100000", Currency: "ARS"},
	}
	ex := executorWithPairs(t, "Suscribi 3100000 a FCI", []orchestrator.Pair{
		{Category: "Inversiones", Subcategory: "FCI"},
		{Category: "Inversiones", Subcategory: "FCI"},
	})
	ex.classify(movs)

	for i, m := range movs {
		if m.Category != "Inversiones" || m.Subcategory != "FCI" {
			t.Errorf("fila %d = %s | %s, want Inversiones | FCI", i, m.Category, m.Subcategory)
		}
	}
}

func TestClassify_StructuralDefaultFillsWhatTheClassifierLeavesEmpty(t *testing.T) {
	movs := []orchestrator.MovementDraft{
		{Type: "transfer", Amount: "-50000", Currency: "ARS"},
		{Type: "transfer", Amount: "50000", Currency: "ARS"},
	}
	ex := executorWithPairs(t, "pasé 50 mil al banco", []orchestrator.Pair{
		{Category: constants.PendingReview, Subcategory: constants.PendingReview},
		{Category: constants.PendingReview, Subcategory: constants.PendingReview},
	})
	ex.classify(movs)

	for i, m := range movs {
		if m.Category != "Sistema" || m.Subcategory != "Transferencia" {
			t.Errorf("fila %d = %s | %s, want el default estructural", i, m.Category, m.Subcategory)
		}
	}
}

func executorWithPairs(t *testing.T, userText string, pairs []orchestrator.Pair) *agentExecutor {
	t.Helper()
	svc := &fakeServices{
		movements:     movementsWithBalance("1000000"),
		accounts:      accountsWithDefault(),
		subcategories: subcategoriesForTest(),
		orch:          &fakeOrchestrator{classifyPairs: pairs},
		chatHistory:   &stubChatHistory{},
	}
	return newAgentExecutor(context.Background(), svc, 1, userText, taxonomyForTest())
}

func TestAgentExecutor_CorrectPassesTheDateLocatorToTheSearch(t *testing.T) {
	fake := &fakeMovementRepoForResolve{}
	e := newAgentExecutor(context.Background(), &fakeServices{movements: fake}, 3,
		"el débito de tarjeta del 04 de agosto eran 131306,49", nil)

	executeDone(t, e, orchestrator.ToolCorrectMovement,
		`{"change":"eran 131306,49","date_from":"2026-08-04"}`)

	if fake.recencyCalled {
		t.Fatal("con date_from no se busca por created_at: la fecha nunca llegó")
	}
	if want := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC); !fake.capturedSince.Equal(want) {
		t.Errorf("since = %v, want %v", fake.capturedSince, want)
	}
	if fake.capturedUntil == nil {
		t.Fatal("until = nil: la ventana quedó abierta hasta hoy")
	}
}
