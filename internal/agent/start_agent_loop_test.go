package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
)

func correctInTheLoop() *fakeOrchestrator {
	return &fakeOrchestrator{
		runFn: func(execute func(string, json.RawMessage) (string, error)) (string, error) {
			_, err := execute(orchestrator.ToolCorrectMovement, json.RawMessage(
				`{"change":"la panaderia era 2000","changes":[{"field":"amount","op":"set","value":"2000"}]}`))
			return "", err
		},
	}
}

func TestLoop_EmptyChangesAsksWhatToChange(t *testing.T) {
	repo := &fakeActionsRepo{}
	movements := &fakeMovementRepoFull{similar: []movement.Movement{
		candidateMovement(10, nil, "compra en panadería", 3000),
	}}
	orch := &fakeOrchestrator{
		runFn: func(execute func(string, json.RawMessage) (string, error)) (string, error) {
			_, err := execute(orchestrator.ToolCorrectMovement, json.RawMessage(
				`{"change":"editá la panaderia","changes":[]}`))
			return "", err
		},
	}
	svc := newLoopServices(t)
	svc.orch = orch
	svc.actions = repo
	svc.movements = movements

	if err := startAgentLoop(context.Background(), svc, &messenger.FakeChat{}, 1, "editá la panaderia"); err != nil {
		t.Fatal(err)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("tenía que quedar parkeada la pregunta de qué cambiar, quedan %d", len(repo.rows))
	}
	if orch.updateCalled {
		t.Error("se llamó a ResolveUpdate: con changes vacío se PREGUNTA, no se interpreta")
	}
}

func TestLoop_AmbiguousCorrectionParksAndAsks(t *testing.T) {
	repo := &fakeActionsRepo{}
	movements := &fakeMovementRepoFull{similar: []movement.Movement{
		candidateMovement(10, nil, "compra en panadería", 3000),
		candidateMovement(11, nil, "panadería del barrio", 5000),
	}}
	svc := newLoopServices(t)
	svc.orch = correctInTheLoop()
	svc.actions = repo
	svc.movements = movements

	if err := startAgentLoop(context.Background(), svc, &messenger.FakeChat{}, 1, "la panaderia era 2000"); err != nil {
		t.Fatal(err)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("want 1 parked action, got %d", len(repo.rows))
	}
	if repo.rows[0].Tool != orchestrator.ToolCorrectMovement {
		t.Errorf("tool equivocada: %q", repo.rows[0].Tool)
	}
	if inProgress, _ := svc.engine.InProgress(1); !inProgress {
		t.Error("el turno tenía que dejar la pregunta abierta")
	}
}

func TestLoop_SingleCandidateGoesStraightToTheGate(t *testing.T) {
	repo := &fakeActionsRepo{}
	movements := &fakeMovementRepoFull{similar: []movement.Movement{
		candidateMovement(10, nil, "compra en panadería", 3000),
	}}
	orch := correctInTheLoop()
	orch.updateResult = orchestrator.UpdateResult{
		Resolved:  true,
		Movements: []orchestrator.MovementDraft{{Type: "expense", Amount: "2000", Currency: "ARS", Category: "Comida", Subcategory: "Supermercado", Date: "2026-08-01"}},
	}
	svc := newLoopServices(t)
	svc.orch = orch
	svc.actions = repo
	svc.movements = movements

	if err := startAgentLoop(context.Background(), svc, &messenger.FakeChat{}, 1, "la panaderia era 2000"); err != nil {
		t.Fatal(err)
	}
	if len(repo.rows) != 0 {
		t.Errorf("sin preguntas la acción se consume en el turno, quedan %d", len(repo.rows))
	}
	if inProgress, _ := svc.engine.InProgress(1); !inProgress {
		t.Error("tenía que quedar abierto el confirm de corrección")
	}
}

func TestLoop_OurCopyBeatsTheModelNarration(t *testing.T) {
	orch := &fakeOrchestrator{
		runFn: func(execute func(string, json.RawMessage) (string, error)) (string, error) {
			if _, err := execute(orchestrator.ToolReplyHelp, json.RawMessage(`{}`)); err != nil {
				return "", err
			}
			return "yo te explico a mi manera", nil
		},
	}
	svc := newLoopServices(t)
	svc.orch = orch

	if err := startAgentLoop(context.Background(), svc, &messenger.FakeChat{}, 1, "¿qué podés hacer?"); err != nil {
		t.Fatal(err)
	}
}

func TestLoop_PromptCarriesTheUsersAccountsAndTools(t *testing.T) {
	orch := &fakeOrchestrator{
		runFn: func(func(string, json.RawMessage) (string, error)) (string, error) { return "ok", nil },
	}
	engine := conversation.NewEngine(&fakeConvStore{}, func(string) string { return "algo" })
	engine.Register(flow.NewAskUserFlow())
	svc := &fakeServices{
		engine: engine, orch: orch, actions: &fakeActionsRepo{},
		accounts: &fakeAccountRepoFull{}, subcategories: &fakeSubcategoryRepoFull{},
		chatHistory: &stubChatHistory{}, movements: &fakeMovementRepoFull{},
		metrics: &fakeMetricRepo{},
	}

	if err := startAgentLoop(context.Background(), svc, &messenger.FakeChat{}, 1, "corregí el asado"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(orch.gotRunPrompt, "CUENTAS DEL USUARIO") {
		t.Errorf("el prompt no lleva las cuentas:\n%s", orch.gotRunPrompt)
	}
	if len(orch.gotRunTools) != 7 {
		t.Errorf("want las 7 tools cableadas, got %d", len(orch.gotRunTools))
	}
	for _, t2 := range orch.gotRunTools {
		if !strings.Contains(orch.gotRunPrompt, t2.Name) {
			t.Errorf("el prompt no nombra %s", t2.Name)
		}
	}
	for _, absent := range []string{orchestrator.ToolManageAccount, orchestrator.ToolSumMovements} {
		if strings.Contains(orch.gotRunPrompt, absent) {
			t.Errorf("el prompt nombra %s, que no está en el toolbox de esta etapa", absent)
		}
	}
}

func TestWiredTools_CorrectionStillPicksCorrectMovement(t *testing.T) {
	names := make([]string, 0, 5)
	for _, tool := range wiredAgentTools() {
		names = append(names, tool.Name)
	}
	if !slices.Contains(names, orchestrator.ToolRecordMovements) {
		t.Fatal("record_movements tiene que estar cableada")
	}
	if len(names) != 7 {
		t.Errorf("toolbox = %v, want las 7 de la etapa 5", names)
	}

	prompt := orchestrator.BuildAgentPrompt("2026-08-01", nil, nil, "", wiredAgentTools(), "")
	if !strings.Contains(prompt, "era, eran, fue") {
		t.Error("falta el copulativo en pasado: es lo único que separa corregir de registrar")
	}
	if !strings.Contains(prompt, orchestrator.ToolRecordMovements) {
		t.Error("el prompt tiene que nombrar record_movements ahora que se manda")
	}
}

func TestWiredAgentTools_MatchesTheExecutorSwitch(t *testing.T) {
	e := newExecutorWith(t, "cualquier cosa")
	for _, tool := range wiredAgentTools() {
		if out, _ := e.execute(tool.Name, json.RawMessage(`{}`)); out == resultNotWiredYet {
			t.Errorf("%s se manda pero el ejecutor no la corre", tool.Name)
		}
	}
}

func TestLoop_AlwaysResolvesTheMetric(t *testing.T) {
	for _, tc := range []struct {
		name string
		tool string
		want string
	}{
		{"ayuda", orchestrator.ToolReplyHelp, outcomeHelpShown},
		{"pedir reescritura", orchestrator.ToolAskRewrite, outcomeUnclear},
		{"sin candidatos", orchestrator.ToolCorrectMovement, outcomeNoCandidates},
		{"narró sin hacer nada", orchestrator.ToolSumMovements, outcomeLoopDidNothing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metrics := &fakeMetricRepo{}
			tool := tc.tool
			orch := &fakeOrchestrator{
				runFn: func(execute func(string, json.RawMessage) (string, error)) (string, error) {
					_, err := execute(tool, json.RawMessage(`{}`))
					return "listo", err
				},
			}
			svc := newLoopServices(t)
			svc.orch = orch
			svc.metrics = metrics

			if err := startAgentLoop(context.Background(), svc, &messenger.FakeChat{}, 1, "algo"); err != nil {
				t.Fatal(err)
			}
			if len(metrics.resolved) != 1 || metrics.resolved[0] != tc.want {
				t.Fatalf("want el evento resuelto como %q, got %v", tc.want, metrics.resolved)
			}
		})
	}
}

func TestLoop_LeavesTheMetricPendingWhenSomethingIsOpen(t *testing.T) {
	metrics := &fakeMetricRepo{}
	movements := &fakeMovementRepoFull{similar: []movement.Movement{
		candidateMovement(10, nil, "compra en panadería", 3000),
		candidateMovement(11, nil, "panadería del barrio", 5000),
	}}
	svc := newLoopServices(t)
	svc.orch = correctInTheLoop()
	svc.metrics = metrics
	svc.movements = movements

	if err := startAgentLoop(context.Background(), svc, &messenger.FakeChat{}, 1, "la panaderia estaba mal"); err != nil {
		t.Fatal(err)
	}
	if len(metrics.resolved) != 0 {
		t.Errorf("con una acción abierta el evento sigue pendiente, got %v", metrics.resolved)
	}
}

func TestLoop_InsertedResolvesCreateInserted(t *testing.T) {
	metrics := &fakeMetricRepo{}
	orch := &fakeOrchestrator{
		classifyPairs: []orchestrator.Pair{{Category: "Alimentación", Subcategory: "Supermercado"}},
		runFn: func(execute func(string, json.RawMessage) (string, error)) (string, error) {
			_, err := execute(orchestrator.ToolRecordMovements, json.RawMessage(`{"movements":[
				{"type":"expense","amount":"5000","currency":"ARS","category":"Alimentación",
				 "subcategory":"Supermercado","date":"2026-08-01","description":"super",
				 "payment_method":"transfer"}]}`))
			return "", err
		},
	}
	svc := newLoopServices(t)
	svc.orch = orch
	svc.metrics = metrics
	svc.accounts = accountsWithDefault()
	svc.subcategories = subcategoriesForTest()
	svc.movements = movementsWithBalance("100000")

	if err := startAgentLoop(context.Background(), svc, &messenger.FakeChat{}, 1, "gasté 5000 en el super"); err != nil {
		t.Fatal(err)
	}
	if len(metrics.resolved) != 1 || metrics.resolved[0] != flow.OutcomeCreateInserted {
		t.Fatalf("want el evento resuelto como %q, got %v", flow.OutcomeCreateInserted, metrics.resolved)
	}
}

func TestLoop_NoQueueAfterAWrite(t *testing.T) {
	orch := &fakeOrchestrator{
		classifyPairs: []orchestrator.Pair{{Category: "Alimentación", Subcategory: "Supermercado"}},
		runFn: func(execute func(string, json.RawMessage) (string, error)) (string, error) {
			_, _ = execute(orchestrator.ToolRecordMovements, json.RawMessage(`{"movements":[
				{"type":"expense","amount":"5000","currency":"ARS","category":"Alimentación",
				 "subcategory":"Supermercado","date":"2026-08-01","description":"super",
				 "payment_method":"transfer"}]}`))
			return "", &orchestrator.RateLimitedError{RetryAfter: time.Second}
		},
	}
	svc := newLoopServices(t)
	svc.orch = orch
	svc.accounts = accountsWithDefault()
	svc.subcategories = subcategoriesForTest()
	svc.movements = movementsWithBalance("100000")

	if err := startAgentLoop(context.Background(), svc, &messenger.FakeChat{}, 1, "gasté 5000 en el super"); err != nil {
		t.Fatal(err)
	}
	if svc.enqueued != 0 {
		t.Errorf("encoló %d jobs después de escribir: el drenaje los duplicaría", svc.enqueued)
	}
}

func TestDrainAfterLoop_OpensTheQuestion(t *testing.T) {
	repo := &fakeActionsRepo{}
	svc := newLoopServices(t)
	svc.actions = repo
	if err := parkAgentActions(context.Background(), svc, 1, []parkedAction{{
		Tool:    orchestrator.ToolCorrectMovement,
		Payload: agentPayload{Change: "eran 2000", Candidates: []flow.CandidateGroup{{OldIDs: []string{"10"}}, {OldIDs: []string{"11"}}}, Chosen: -1},
		Questions: []pendingaction.OpenQuestion{{
			Key: questionKeyCandidate, Prompt: "¿Cuál es?", Options: []string{"a", "b"},
		}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := drainNextAgentAction(context.Background(), svc, &messenger.FakeChat{}, 1); err != nil {
		t.Fatal(err)
	}
	if inProgress, _ := svc.engine.InProgress(1); !inProgress {
		t.Error("el drenaje tenía que dejar la pregunta abierta")
	}
}

func TestLoop_ReplayDoesNotOpenANewIntentEvent(t *testing.T) {
	metrics := &fakeMetricRepo{}
	orch := &fakeOrchestrator{runFn: func(execute func(string, json.RawMessage) (string, error)) (string, error) {
		_, err := execute(orchestrator.ToolReplyHelp, json.RawMessage(`{}`))
		return "", err
	}}
	svc := newLoopServices(t)
	svc.orch = orch
	svc.metrics = metrics

	if err := startAgentLoop(context.Background(), svc, &messenger.FakeChat{}, 1, "que podés hacer"); err != nil {
		t.Fatal(err)
	}
	afterWebhook := len(metrics.logged)

	svc.replaying = true
	if err := startAgentLoop(context.Background(), svc, &messenger.FakeChat{}, 1, "que podés hacer"); err != nil {
		t.Fatal(err)
	}
	if len(metrics.logged) != afterWebhook {
		t.Errorf("el replay abrió %d eventos de más", len(metrics.logged)-afterWebhook)
	}
	if len(metrics.queuedIntents) != 1 || metrics.queuedIntents[0] != string(orchestrator.IntentHelp) {
		t.Errorf("el replay tenía que corregir el intent a HELP, corrigió %v", metrics.queuedIntents)
	}
}
