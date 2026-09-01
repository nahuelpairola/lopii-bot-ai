package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
)

func twoCandidateAction(t *testing.T) parkedAction {
	t.Helper()
	return parkedAction{
		Tool: orchestrator.ToolCorrectMovement,
		Payload: agentPayload{
			Change: "eran 2000",
			Candidates: []flow.CandidateGroup{
				{TransactionID: "", OldIDs: []string{"10"}, Rows: []movement.MovementRow{{Amount: "3000", Currency: "ARS"}}},
				{TransactionID: "", OldIDs: []string{"11"}, Rows: []movement.MovementRow{{Amount: "5000", Currency: "ARS"}}},
			},
			Chosen: -1,
		},
		Questions: []pendingaction.OpenQuestion{{
			Key: questionKeyCandidate, Prompt: "¿Cuál es?", Options: []string{"la de 3000", "la de 5000"},
		}},
	}
}

func TestParkAgentActions_FreezesTheBudgetAndKeepsOrder(t *testing.T) {
	repo := &fakeActionsRepo{}
	svc := newDispatchServices(t, repo)

	err := parkAgentActions(context.Background(), svc, 1, []parkedAction{
		twoCandidateAction(t),
		{Tool: orchestrator.ToolDeleteMovements, Payload: agentPayload{Chosen: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.rows) != 2 {
		t.Fatalf("want 2 parked rows, got %d", len(repo.rows))
	}
	if repo.rows[0].Budget != 1+budgetSlack {
		t.Errorf("want budget %d, got %d", 1+budgetSlack, repo.rows[0].Budget)
	}
	if repo.rows[0].Position != 0 || repo.rows[1].Position != 1 {
		t.Errorf("el orden de dependencia no se guardó: %d, %d", repo.rows[0].Position, repo.rows[1].Position)
	}
}

func TestDrain_OpensExactlyOneAtATime(t *testing.T) {
	repo := &fakeActionsRepo{}
	svc := newDispatchServices(t, repo)
	if err := parkAgentActions(context.Background(), svc, 1, []parkedAction{twoCandidateAction(t), twoCandidateAction(t)}); err != nil {
		t.Fatal(err)
	}

	if err := drainNextAgentAction(context.Background(), svc, &messenger.FakeChat{}, 1); err != nil {
		t.Fatal(err)
	}
	inProgress, err := svc.engine.InProgress(1)
	if err != nil || !inProgress {
		t.Fatalf("el drenaje tiene que dejar un flujo abierto: %v %v", inProgress, err)
	}
	if n, _ := repo.CountForUser(1); n != 2 {
		t.Errorf("drenar no borra: la acción se saca al resolverse, got %d", n)
	}
}

func TestResume_CreateWithGapsOpensMovementCreate(t *testing.T) {
	seed := conversation.Data{
		conversation.KeyMovements:           movement.EncodeMovementRows([]movement.MovementRow{{Type: "expense", Amount: "5000", Currency: "ARS"}}),
		conversation.KeyPendingCategoryGaps: conversation.EncodeStringSlice([]string{"0"}),
		conversation.KeyPendingAccountGaps:  conversation.EncodeStringSlice(nil),
	}
	repo := &fakeActionsRepo{}
	svc := newDispatchServices(t, repo)
	svc.engine.Register(flow.NewMovementCreateFlow(&fakeSubcategoryRepoFull{}, &fakeAccountRepoFull{}))
	svc.accounts, svc.subcategories = &fakeAccountRepoFull{}, &fakeSubcategoryRepoFull{}

	if err := parkAgentActions(context.Background(), svc, 1, []parkedAction{{
		Tool:    orchestrator.ToolRecordMovements,
		Payload: agentPayload{Seed: seed, Chosen: 0},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := drainNextAgentAction(context.Background(), svc, &messenger.FakeChat{}, 1); err != nil {
		t.Fatal(err)
	}

	if len(repo.deleted) != 1 {
		t.Errorf("la acción tiene que salir de la cola antes de abrir el flujo, deleted=%v", repo.deleted)
	}
	if inProgress, _ := svc.engine.InProgress(1); !inProgress {
		t.Error("tenía que quedar abierto movement_create para preguntar el gap")
	}
}

func TestDrain_NothingParkedIsANoOp(t *testing.T) {
	svc := newDispatchServices(t, &fakeActionsRepo{})
	if err := drainNextAgentAction(context.Background(), svc, &messenger.FakeChat{}, 1); err != nil {
		t.Fatalf("sin nada parkeado el drenaje no puede fallar: %v", err)
	}
	if inProgress, _ := svc.engine.InProgress(1); inProgress {
		t.Error("no tenía que abrir ningún flujo")
	}
}

func TestApplyAnswers_LabelPositionIsTheCandidateIndex(t *testing.T) {
	action := &pendingaction.PendingAction{Payload: mustJSON(t, agentPayload{
		Candidates: []flow.CandidateGroup{{OldIDs: []string{"10"}}, {OldIDs: []string{"11"}}},
		Chosen:     -1,
	})}
	answers := []pendingaction.OpenQuestion{{
		Key: questionKeyCandidate, Options: []string{"la de 3000", "la de 5000"}, Answer: "la de 5000",
	}}

	payload, resolved := applyAnswers(action, answers)
	if !resolved || payload.Chosen != 1 {
		t.Fatalf("want chosen=1 resolved, got chosen=%d resolved=%v", payload.Chosen, resolved)
	}
}

func TestApplyAnswers_FreeTextThatNamesNoCandidateStaysUnresolved(t *testing.T) {
	action := &pendingaction.PendingAction{Payload: mustJSON(t, agentPayload{
		Candidates: []flow.CandidateGroup{{OldIDs: []string{"10"}}, {OldIDs: []string{"11"}}},
		Chosen:     -1,
	})}
	answers := []pendingaction.OpenQuestion{{
		Key: questionKeyCandidate, Options: []string{"la de 3000", "la de 5000"}, Answer: "ninguna de esas",
	}}

	if _, resolved := applyAnswers(action, answers); resolved {
		t.Error("una respuesta que no nombra ningún candidato NO puede resolver la acción")
	}
}

func TestDiscard_NamesWhatWasDropped(t *testing.T) {
	repo := &fakeActionsRepo{}
	svc := newDispatchServices(t, repo)
	if err := parkAgentActions(context.Background(), svc, 1, []parkedAction{twoCandidateAction(t)}); err != nil {
		t.Fatal(err)
	}
	action := repo.rows[0]

	if err := discardAgentAction(context.Background(), svc, &messenger.FakeChat{}, 1, action); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.CountForUser(1); n != 0 {
		t.Errorf("la acción tiene que salir de la cola, quedan %d", n)
	}
	msg := msgAgentActionDiscarded(describeAction(action))
	if !strings.Contains(msg, "eran 2000") {
		t.Errorf("el aviso no nombra lo que se cayó:\n%s", msg)
	}
}

func TestBudgetExhausted_DiscardsInsteadOfAskingAgain(t *testing.T) {
	repo := &fakeActionsRepo{}
	svc := newDispatchServices(t, repo)
	if err := parkAgentActions(context.Background(), svc, 1, []parkedAction{twoCandidateAction(t)}); err != nil {
		t.Fatal(err)
	}

	if err := openAskUser(context.Background(), svc, &messenger.FakeChat{}, 1, repo.rows[0], 0); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.CountForUser(1); n != 0 {
		t.Errorf("con el presupuesto agotado la acción se descarta, quedan %d", n)
	}
	if inProgress, _ := svc.engine.InProgress(1); inProgress {
		t.Error("no puede quedar preguntando con el presupuesto en cero")
	}
}

func TestResume_DeleteOpensTheExistingGate(t *testing.T) {
	repo := &fakeActionsRepo{}
	svc := newDispatchServices(t, repo)
	action := &pendingaction.PendingAction{
		UserID: 1, Tool: orchestrator.ToolDeleteMovements,
		Payload: mustJSON(t, agentPayload{
			Candidates: []flow.CandidateGroup{{OldIDs: []string{"10"}, Rows: []movement.MovementRow{{Amount: "3000", Currency: "ARS", Type: "expense"}}}},
			Chosen:     0,
		}),
		Questions: []byte(`[]`),
	}
	if err := repo.Insert(action); err != nil {
		t.Fatal(err)
	}

	if err := resumeAgentAction(context.Background(), svc, &messenger.FakeChat{}, 1, action); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.CountForUser(1); n != 0 {
		t.Errorf("retomar saca la acción de la cola, quedan %d", n)
	}
	if inProgress, _ := svc.engine.InProgress(1); !inProgress {
		t.Error("tenía que quedar abierto el confirm de borrado")
	}
}

func TestResume_RefusesACandidateOutOfRange(t *testing.T) {
	repo := &fakeActionsRepo{}
	svc := newDispatchServices(t, repo)
	action := &pendingaction.PendingAction{
		UserID: 1, Tool: orchestrator.ToolDeleteMovements,
		Payload: mustJSON(t, agentPayload{Candidates: []flow.CandidateGroup{{OldIDs: []string{"10"}}}, Chosen: 7}),
	}
	if err := repo.Insert(action); err != nil {
		t.Fatal(err)
	}
	if err := resumeAgentAction(context.Background(), svc, &messenger.FakeChat{}, 1, action); err == nil {
		t.Fatal("un índice fuera de rango tiene que fallar, no elegir cualquiera")
	}
	if n, _ := repo.CountForUser(1); n != 1 {
		t.Errorf("si no se retomó, la acción no se borra: quedan %d", n)
	}
}

func TestOpenAction_RefusesAMismatchedID(t *testing.T) {
	repo := &fakeActionsRepo{}
	svc := newDispatchServices(t, repo)
	if err := parkAgentActions(context.Background(), svc, 1, []parkedAction{twoCandidateAction(t)}); err != nil {
		t.Fatal(err)
	}
	data := conversation.Data{conversation.UserIDKey: uint64(1), conversation.KeyActionID: "999"}
	if _, err := openAction(svc, 1, data); err == nil {
		t.Fatal("un id que no coincide con la cabeza de la cola no puede resolverse")
	}
}

func TestOpenAction_MatchesTheQueueHead(t *testing.T) {
	repo := &fakeActionsRepo{}
	svc := newDispatchServices(t, repo)
	if err := parkAgentActions(context.Background(), svc, 1, []parkedAction{twoCandidateAction(t)}); err != nil {
		t.Fatal(err)
	}
	data := conversation.Data{
		conversation.UserIDKey:   uint64(1),
		conversation.KeyActionID: strconv.FormatUint(repo.rows[0].ID, 10),
	}
	got, err := openAction(svc, 1, data)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != repo.rows[0].ID {
		t.Errorf("want action %d, got %d", repo.rows[0].ID, got.ID)
	}
}

func TestNextForUser_IsPerUser(t *testing.T) {
	repo := &fakeActionsRepo{}
	svc := newDispatchServices(t, repo)
	if err := parkAgentActions(context.Background(), svc, 1, []parkedAction{twoCandidateAction(t)}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.NextForUser(2); !errors.Is(err, pendingaction.ErrNoPendingAction) {
		t.Errorf("la cola de otro usuario no puede verse: %v", err)
	}
}

func TestFinishAskUser_CancelResuelveLaMetrica(t *testing.T) {
	repo := &fakeActionsRepo{}
	svc := newDispatchServices(t, repo)
	if err := parkAgentActions(context.Background(), svc, 1, []parkedAction{twoCandidateAction(t)}); err != nil {
		t.Fatal(err)
	}
	data := conversation.Data{
		conversation.UserIDKey:   uint64(1),
		conversation.KeyActionID: strconv.FormatUint(repo.rows[0].ID, 10),
	}
	conversation.SetFlag(data, conversation.KeyCancelled)

	finishAskUserFlow(context.Background(), svc, &messenger.FakeChat{}, data)

	if len(svc.metrics.resolved) != 1 || svc.metrics.resolved[0] != flow.OutcomeUpdateCancelled {
		t.Fatalf("want %q, got %v", flow.OutcomeUpdateCancelled, svc.metrics.resolved)
	}
}

func TestFinishAskUser_TextoLibreVuelveABuscar(t *testing.T) {
	repo := &fakeActionsRepo{}
	svc := newDispatchServices(t, repo)

	svc.movements = &fakeMovementRepoForResolve{result: []movement.Movement{
		{Model: gorm.Model{ID: 40, CreatedAt: time.Now().Add(-200 * time.Hour)},
			Description: strPtr("Compra en carnicería")},
	}}

	action := twoCandidateAction(t)
	action.Payload.SearchText = "eran 2000"
	if err := parkAgentActions(context.Background(), svc, 1, []parkedAction{action}); err != nil {
		t.Fatal(err)
	}

	data := conversation.Data{
		conversation.UserIDKey:   uint64(1),
		conversation.KeyActionID: strconv.FormatUint(repo.rows[0].ID, 10),
		conversation.KeyOpenQuestions: flow.EncodeOpenQuestions([]pendingaction.OpenQuestion{{
			Key:     questionKeyCandidate,
			Prompt:  "¿Cuál es?",
			Options: []string{"la de 3000", "la de 5000"},
			Answer:  "no, el de la carnicería",
		}}),
		conversation.KeyAskBudget: "2",
	}

	finishAskUserFlow(context.Background(), svc, &messenger.FakeChat{}, data)

	var payload agentPayload
	if err := json.Unmarshal(repo.rows[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Candidates) != 1 {
		t.Fatalf("candidatos = %d, want 1: el texto libre tenía que re-buscar", len(payload.Candidates))
	}
	if payload.Candidates[0].OldIDs[0] != "40" {
		t.Errorf("candidato = %v, want 40 (la carnicería, que salió de la re-búsqueda)", payload.Candidates[0].OldIDs)
	}
}

func TestFinishAskUser_ReBusquedaSinMatchAvisaFallback(t *testing.T) {
	repo := &fakeActionsRepo{}
	svc := newDispatchServices(t, repo)

	svc.movements = &fakeMovementRepoForResolve{result: []movement.Movement{
		{Model: gorm.Model{ID: 40, CreatedAt: time.Now().Add(-200 * time.Hour)},
			Description: strPtr("Nafta YPF")},
	}}

	action := twoCandidateAction(t)
	action.Payload.SearchText = "eran 2000"
	if err := parkAgentActions(context.Background(), svc, 1, []parkedAction{action}); err != nil {
		t.Fatal(err)
	}

	data := conversation.Data{
		conversation.UserIDKey:   uint64(1),
		conversation.KeyActionID: strconv.FormatUint(repo.rows[0].ID, 10),
		conversation.KeyOpenQuestions: flow.EncodeOpenQuestions([]pendingaction.OpenQuestion{{
			Key:     questionKeyCandidate,
			Prompt:  "¿Cuál es?",
			Options: []string{"la de 3000", "la de 5000"},
			Answer:  "no se cual es",
		}}),
		conversation.KeyAskBudget: "2",
	}

	finishAskUserFlow(context.Background(), svc, &messenger.FakeChat{}, data)

	var questions []pendingaction.OpenQuestion
	if err := json.Unmarshal(repo.rows[0].Questions, &questions); err != nil {
		t.Fatal(err)
	}
	if len(questions) != 1 {
		t.Fatalf("questions = %d, want 1", len(questions))
	}
	if !strings.HasPrefix(questions[0].Prompt, flow.MsgPickRecentFallback) {
		t.Errorf("prompt = %q, want que empiece con %q", questions[0].Prompt, flow.MsgPickRecentFallback)
	}
}

func TestFinishAskUser_LaReBusquedaRespetaLaVentana(t *testing.T) {
	repo := &fakeActionsRepo{}
	svc := newDispatchServices(t, repo)
	fake := &fakeMovementRepoForResolve{}
	svc.movements = fake

	action := twoCandidateAction(t)
	action.Payload.SearchText = "el débito del 4 de agosto"
	action.Payload.DateFrom = "2026-08-04"
	if err := parkAgentActions(context.Background(), svc, 1, []parkedAction{action}); err != nil {
		t.Fatal(err)
	}

	data := conversation.Data{
		conversation.UserIDKey:   uint64(1),
		conversation.KeyActionID: strconv.FormatUint(repo.rows[0].ID, 10),
		conversation.KeyOpenQuestions: flow.EncodeOpenQuestions([]pendingaction.OpenQuestion{{
			Key:     questionKeyCandidate,
			Prompt:  "¿Cuál es?",
			Options: []string{"la de 3000", "la de 5000"},
			Answer:  "el de mercado pago",
		}}),
		conversation.KeyAskBudget: "2",
	}

	finishAskUserFlow(context.Background(), svc, &messenger.FakeChat{}, data)

	if fake.recencyCalled {
		t.Fatal("la re-búsqueda cayó en la ventana por created_at: perdió el localizador de fecha")
	}
	if want := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC); !fake.capturedSince.Equal(want) {
		t.Errorf("since = %v, want %v", fake.capturedSince, want)
	}
}
