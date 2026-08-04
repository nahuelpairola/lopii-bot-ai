package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
)

type fakeActionsRepo struct {
	rows    []*pendingaction.PendingAction
	nextID  uint64
	deleted []uint64
}

func (r *fakeActionsRepo) Insert(a *pendingaction.PendingAction) error {
	r.nextID++
	a.ID = r.nextID
	r.rows = append(r.rows, a)
	return nil
}

func (r *fakeActionsRepo) NextForUser(userID uint64) (*pendingaction.PendingAction, error) {
	var best *pendingaction.PendingAction
	for _, a := range r.rows {
		if a.UserID != userID {
			continue
		}
		if best == nil || a.Position < best.Position || (a.Position == best.Position && a.ID < best.ID) {
			best = a
		}
	}
	if best == nil {
		return nil, pendingaction.ErrNoPendingAction
	}
	return best, nil
}

func (r *fakeActionsRepo) Delete(id uint64) error {
	r.deleted = append(r.deleted, id)
	kept := r.rows[:0]
	for _, a := range r.rows {
		if a.ID != id {
			kept = append(kept, a)
		}
	}
	r.rows = kept
	return nil
}

func (r *fakeActionsRepo) CountForUser(userID uint64) (int64, error) {
	var n int64
	for _, a := range r.rows {
		if a.UserID == userID {
			n++
		}
	}
	return n, nil
}

func newDispatchController(t *testing.T, repo *fakeActionsRepo) *controller {
	t.Helper()
	engine := conversation.NewEngine(&fakeConvStore{}, func(string) string { return "algo" })
	engine.Register(NewAskUserFlow())
	engine.Register(NewMovementDeleteFlow())
	return &controller{engine: engine, actions: repo, movements: &fakeMovementRepoFull{}}
}

func twoCandidateAction(t *testing.T) parkedAction {
	t.Helper()
	return parkedAction{
		Tool: orchestrator.ToolCorrectMovement,
		Payload: agentPayload{
			Change: "eran 2000",
			Candidates: []candidateGroup{
				{TransactionID: "", OldIDs: []string{"10"}, Rows: []movementRow{{Amount: "3000", Currency: "ARS"}}},
				{TransactionID: "", OldIDs: []string{"11"}, Rows: []movementRow{{Amount: "5000", Currency: "ARS"}}},
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
	c := newDispatchController(t, repo)

	err := c.parkAgentActions(context.Background(), 1, []parkedAction{
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
	c := newDispatchController(t, repo)
	if err := c.parkAgentActions(context.Background(), 1, []parkedAction{twoCandidateAction(t), twoCandidateAction(t)}); err != nil {
		t.Fatal(err)
	}

	if err := c.drainNextAgentAction(context.Background(), nil, 0, 1); err != nil {
		t.Fatal(err)
	}
	inProgress, err := c.engine.InProgress(1)
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
		keyMovements:           encodeMovementRows([]movementRow{{Type: "expense", Amount: "5000", Currency: "ARS"}}),
		keyPendingCategoryGaps: encodeStringSlice([]string{"0"}),
		keyPendingAccountGaps:  encodeStringSlice(nil),
	}
	repo := &fakeActionsRepo{}
	c := newDispatchController(t, repo)
	c.engine.Register(NewMovementCreateFlow(&fakeSubcategoryRepoFull{}, &fakeAccountRepoFull{}))
	c.accounts, c.subcategories = &fakeAccountRepoFull{}, &fakeSubcategoryRepoFull{}

	if err := c.parkAgentActions(context.Background(), 1, []parkedAction{{
		Tool:    orchestrator.ToolRecordMovements,
		Payload: agentPayload{Seed: seed, Chosen: 0},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := c.drainNextAgentAction(context.Background(), nil, 0, 1); err != nil {
		t.Fatal(err)
	}

	if len(repo.deleted) != 1 {
		t.Errorf("la acción tiene que salir de la cola antes de abrir el flujo, deleted=%v", repo.deleted)
	}
	if inProgress, _ := c.engine.InProgress(1); !inProgress {
		t.Error("tenía que quedar abierto movement_create para preguntar el gap")
	}
}

func TestDrain_NothingParkedIsANoOp(t *testing.T) {
	c := newDispatchController(t, &fakeActionsRepo{})
	if err := c.drainNextAgentAction(context.Background(), nil, 0, 1); err != nil {
		t.Fatalf("sin nada parkeado el drenaje no puede fallar: %v", err)
	}
	if inProgress, _ := c.engine.InProgress(1); inProgress {
		t.Error("no tenía que abrir ningún flujo")
	}
}

// TestAnswerResolvesTheCandidate: la etiqueta contestada es el índice.
func TestApplyAnswers_LabelPositionIsTheCandidateIndex(t *testing.T) {
	action := &pendingaction.PendingAction{Payload: mustJSON(t, agentPayload{
		Candidates: []candidateGroup{{OldIDs: []string{"10"}}, {OldIDs: []string{"11"}}},
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
		Candidates: []candidateGroup{{OldIDs: []string{"10"}}, {OldIDs: []string{"11"}}},
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
	c := newDispatchController(t, repo)
	if err := c.parkAgentActions(context.Background(), 1, []parkedAction{twoCandidateAction(t)}); err != nil {
		t.Fatal(err)
	}
	action := repo.rows[0]

	if err := c.discardAgentAction(context.Background(), nil, 0, 1, action); err != nil {
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
	c := newDispatchController(t, repo)
	if err := c.parkAgentActions(context.Background(), 1, []parkedAction{twoCandidateAction(t)}); err != nil {
		t.Fatal(err)
	}

	if err := c.openAskUser(context.Background(), nil, 0, 1, repo.rows[0], 0); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.CountForUser(1); n != 0 {
		t.Errorf("con el presupuesto agotado la acción se descarta, quedan %d", n)
	}
	if inProgress, _ := c.engine.InProgress(1); inProgress {
		t.Error("no puede quedar preguntando con el presupuesto en cero")
	}
}

// TestResume_DeleteOpensTheExistingGate: el confirm de borrado no se toca, sólo
// cambia cómo se llega hasta él.
func TestResume_DeleteOpensTheExistingGate(t *testing.T) {
	repo := &fakeActionsRepo{}
	c := newDispatchController(t, repo)
	action := &pendingaction.PendingAction{
		UserID: 1, Tool: orchestrator.ToolDeleteMovements,
		Payload: mustJSON(t, agentPayload{
			Candidates: []candidateGroup{{OldIDs: []string{"10"}, Rows: []movementRow{{Amount: "3000", Currency: "ARS", Type: "expense"}}}},
			Chosen:     0,
		}),
		Questions: []byte(`[]`),
	}
	if err := repo.Insert(action); err != nil {
		t.Fatal(err)
	}

	if err := c.resumeAgentAction(context.Background(), nil, 0, 1, action); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.CountForUser(1); n != 0 {
		t.Errorf("retomar saca la acción de la cola, quedan %d", n)
	}
	if inProgress, _ := c.engine.InProgress(1); !inProgress {
		t.Error("tenía que quedar abierto el confirm de borrado")
	}
}

// TestResume_RefusesACandidateOutOfRange: nunca actuar sobre un índice que no
// existe, aunque el payload venga corrupto.
func TestResume_RefusesACandidateOutOfRange(t *testing.T) {
	repo := &fakeActionsRepo{}
	c := newDispatchController(t, repo)
	action := &pendingaction.PendingAction{
		UserID: 1, Tool: orchestrator.ToolDeleteMovements,
		Payload: mustJSON(t, agentPayload{Candidates: []candidateGroup{{OldIDs: []string{"10"}}}, Chosen: 7}),
	}
	if err := repo.Insert(action); err != nil {
		t.Fatal(err)
	}
	if err := c.resumeAgentAction(context.Background(), nil, 0, 1, action); err == nil {
		t.Fatal("un índice fuera de rango tiene que fallar, no elegir cualquiera")
	}
	if n, _ := repo.CountForUser(1); n != 1 {
		t.Errorf("si no se retomó, la acción no se borra: quedan %d", n)
	}
}

func TestOpenAction_RefusesAMismatchedID(t *testing.T) {
	repo := &fakeActionsRepo{}
	c := newDispatchController(t, repo)
	if err := c.parkAgentActions(context.Background(), 1, []parkedAction{twoCandidateAction(t)}); err != nil {
		t.Fatal(err)
	}
	data := conversation.Data{conversation.UserIDKey: uint64(1), keyActionID: "999"}
	if _, err := c.openAction(1, data); err == nil {
		t.Fatal("un id que no coincide con la cabeza de la cola no puede resolverse")
	}
}

func TestOpenAction_MatchesTheQueueHead(t *testing.T) {
	repo := &fakeActionsRepo{}
	c := newDispatchController(t, repo)
	if err := c.parkAgentActions(context.Background(), 1, []parkedAction{twoCandidateAction(t)}); err != nil {
		t.Fatal(err)
	}
	data := conversation.Data{
		conversation.UserIDKey: uint64(1),
		keyActionID:            strconv.FormatUint(repo.rows[0].ID, 10),
	}
	got, err := c.openAction(1, data)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != repo.rows[0].ID {
		t.Errorf("want action %d, got %d", repo.rows[0].ID, got.ID)
	}
}

func TestNextForUser_IsPerUser(t *testing.T) {
	repo := &fakeActionsRepo{}
	c := newDispatchController(t, repo)
	if err := c.parkAgentActions(context.Background(), 1, []parkedAction{twoCandidateAction(t)}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.NextForUser(2); !errors.Is(err, pendingaction.ErrNoPendingAction) {
		t.Errorf("la cola de otro usuario no puede verse: %v", err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
