package agent

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
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
	// 1 pregunta + budgetSlack; la que no pregunta nada arranca en el slack.
	if repo.rows[0].Budget != 1+budgetSlack {
		t.Errorf("want budget %d, got %d", 1+budgetSlack, repo.rows[0].Budget)
	}
	if repo.rows[0].Position != 0 || repo.rows[1].Position != 1 {
		t.Errorf("el orden de dependencia no se guardó: %d, %d", repo.rows[0].Position, repo.rows[1].Position)
	}
}

// TestDrain_WIP1: con dos acciones parkeadas se abre UNA sola.
func TestDrain_OpensExactlyOneAtATime(t *testing.T) {
	repo := &fakeActionsRepo{}
	svc := newDispatchServices(t, repo)
	if err := parkAgentActions(context.Background(), svc, 1, []parkedAction{twoCandidateAction(t), twoCandidateAction(t)}); err != nil {
		t.Fatal(err)
	}

	if err := drainNextAgentAction(context.Background(), svc, nil, 0, 1); err != nil {
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

// TestResume_CreateWithGapsOpensMovementCreate: un CREATE incompleto no inserta
// nada y retoma el flujo que ya existe. La etapa 3 cambia cómo se llega al
// gap-fill, no el gap-fill.
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
	if err := drainNextAgentAction(context.Background(), svc, nil, 0, 1); err != nil {
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
	if err := drainNextAgentAction(context.Background(), svc, nil, 0, 1); err != nil {
		t.Fatalf("sin nada parkeado el drenaje no puede fallar: %v", err)
	}
	if inProgress, _ := svc.engine.InProgress(1); inProgress {
		t.Error("no tenía que abrir ningún flujo")
	}
}

// TestAnswerResolvesTheCandidate: la etiqueta contestada es el índice.
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

// TestDiscard_NamesWhatWasDropped es el tope de presupuesto: se tira entera y se
// dice qué se cayó. Tirar algo en silencio es la falla que esto viene a evitar.
func TestDiscard_NamesWhatWasDropped(t *testing.T) {
	repo := &fakeActionsRepo{}
	svc := newDispatchServices(t, repo)
	if err := parkAgentActions(context.Background(), svc, 1, []parkedAction{twoCandidateAction(t)}); err != nil {
		t.Fatal(err)
	}
	action := repo.rows[0]

	if err := discardAgentAction(context.Background(), svc, nil, 0, 1, action); err != nil {
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

// TestBudgetExhausted_DiscardsInsteadOfAskingAgain cierra el lazo: openAskUser
// con presupuesto agotado descarta, no vuelve a preguntar para siempre.
func TestBudgetExhausted_DiscardsInsteadOfAskingAgain(t *testing.T) {
	repo := &fakeActionsRepo{}
	svc := newDispatchServices(t, repo)
	if err := parkAgentActions(context.Background(), svc, 1, []parkedAction{twoCandidateAction(t)}); err != nil {
		t.Fatal(err)
	}

	if err := openAskUser(context.Background(), svc, nil, 0, 1, repo.rows[0], 0); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.CountForUser(1); n != 0 {
		t.Errorf("con el presupuesto agotado la acción se descarta, quedan %d", n)
	}
	if inProgress, _ := svc.engine.InProgress(1); inProgress {
		t.Error("no puede quedar preguntando con el presupuesto en cero")
	}
}

// TestResume_DeleteOpensTheExistingGate: el confirm de borrado no se toca, sólo
// cambia cómo se llega hasta él.
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

	if err := resumeAgentAction(context.Background(), svc, nil, 0, 1, action); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.CountForUser(1); n != 0 {
		t.Errorf("retomar saca la acción de la cola, quedan %d", n)
	}
	if inProgress, _ := svc.engine.InProgress(1); !inProgress {
		t.Error("tenía que quedar abierto el confirm de borrado")
	}
}

// TestResume_RefusesACandidateOutOfRange: nunca actuar sobre un índice que no
// existe, aunque el payload venga corrupto.
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
	if err := resumeAgentAction(context.Background(), svc, nil, 0, 1, action); err == nil {
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

// Cancelar es un final del usuario, no un flujo muerto: sin el resolve la fila
// queda en pending y el mensaje siguiente la cierra como abandoned.
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

	finishAskUserFlow(context.Background(), svc, nil, 0, data)

	if len(svc.metrics.resolved) != 1 || svc.metrics.resolved[0] != flow.OutcomeUpdateCancelled {
		t.Fatalf("want %q, got %v", flow.OutcomeUpdateCancelled, svc.metrics.resolved)
	}
}
