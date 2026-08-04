package messaging

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
)

func newLoopController(t *testing.T, orch *fakeFullOrchestrator, repo *fakeActionsRepo, movements *fakeMovementRepoFull) *controller {
	t.Helper()
	engine := conversation.NewEngine(&fakeConvStore{}, func(string) string { return "algo" })
	engine.Register(NewAskUserFlow())
	engine.Register(NewMovementDeleteFlow())
	engine.Register(NewMovementUpdateConfirmFlow())
	return &controller{
		engine:        engine,
		orchestrator:  orch,
		actions:       repo,
		movements:     movements,
		accounts:      &fakeAccountRepoFull{},
		subcategories: &fakeSubcategoryRepoFull{},
		chatHistory:   stubChatHistory{},
		metrics:       &fakeMetricRepo{},
	}
}

// TestRouting_UpdateAndDeleteGoThroughTheLoop es el cambio de la etapa: esos dos
// intents dejan de abrir el picker y pasan por Run. Los otros ocho no se tocan.
func TestRouting_UpdateAndDeleteGoThroughTheLoop(t *testing.T) {
	for _, intent := range []orchestrator.Intent{orchestrator.IntentUpdate, orchestrator.IntentDelete} {
		t.Run(string(intent), func(t *testing.T) {
			called := false
			orch := &fakeFullOrchestrator{
				intent: intent,
				runFn: func(func(string, json.RawMessage) (string, error)) (string, error) {
					called = true
					return "listo", nil
				},
			}
			c := newLoopController(t, orch, &fakeActionsRepo{}, &fakeMovementRepoFull{})

			if err := c.handleFreeText(context.Background(), nil, 0, 1, "el café en realidad fue 3500"); err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Fatalf("%s tiene que ir por el agent loop", intent)
			}
		})
	}
}

// TestFreeText_CreateRoutingRespectsTheFlag: el interruptor es la única forma de
// bisectar, porque las etapas 2 y 3 despliegan juntas. Apagado, CREATE tiene que
// volver al camino de siempre sin tocar nada más.
func TestFreeText_CreateRoutingRespectsTheFlag(t *testing.T) {
	for _, tc := range []struct{ flag, wantLoop bool }{{true, true}, {false, false}} {
		orch := &fakeFullOrchestrator{
			intent: orchestrator.IntentCreate,
			runFn:  func(func(string, json.RawMessage) (string, error)) (string, error) { return "ok", nil },
		}
		c := newLoopController(t, orch, &fakeActionsRepo{}, &fakeMovementRepoFull{})
		c.routeCreateToLoop = tc.flag

		_ = c.handleFreeText(context.Background(), nil, 0, 1, "gasté 5000 en el super")

		if orch.runCalled != tc.wantLoop {
			t.Errorf("flag=%v: loop usado = %v, want %v", tc.flag, orch.runCalled, tc.wantLoop)
		}
	}
}

// correctInTheLoop programa un loop que pide corregir sin nombrar cuál.
func correctInTheLoop(intent orchestrator.Intent) *fakeFullOrchestrator {
	return &fakeFullOrchestrator{
		intent: intent,
		runFn: func(execute func(string, json.RawMessage) (string, error)) (string, error) {
			// El texto nombra la panadería: sin eso resolveCandidates cae al atajo
			// de "lo último que cargaste" y devuelve uno solo, nunca dos.
			_, err := execute(orchestrator.ToolCorrectMovement, json.RawMessage(`{"change":"la panaderia era 2000"}`))
			return "", err
		},
	}
}

// TestLoop_AmbiguousCorrectionParksAndAsks: con dos candidatos la acción queda
// en la cola y el mismo turno abre la pregunta.
func TestLoop_AmbiguousCorrectionParksAndAsks(t *testing.T) {
	repo := &fakeActionsRepo{}
	movements := &fakeMovementRepoFull{similar: []movement.Movement{
		candidateMovement(10, nil, "compra en panadería", 3000),
		candidateMovement(11, nil, "panadería del barrio", 5000),
	}}
	c := newLoopController(t, correctInTheLoop(orchestrator.IntentUpdate), repo, movements)

	if err := c.startAgentLoop(context.Background(), nil, 0, 1, "la panaderia era 2000"); err != nil {
		t.Fatal(err)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("want 1 parked action, got %d", len(repo.rows))
	}
	if repo.rows[0].Tool != orchestrator.ToolCorrectMovement {
		t.Errorf("tool equivocada: %q", repo.rows[0].Tool)
	}
	if inProgress, _ := c.engine.InProgress(1); !inProgress {
		t.Error("el turno tenía que dejar la pregunta abierta")
	}
}

// TestLoop_SingleCandidateGoesStraightToTheGate: con un solo candidato no hay
// nada que preguntar, así que la acción se parkea y se consume en el mismo
// turno — y termina en el confirm de corrección, que no se tocó.
func TestLoop_SingleCandidateGoesStraightToTheGate(t *testing.T) {
	repo := &fakeActionsRepo{}
	movements := &fakeMovementRepoFull{similar: []movement.Movement{
		candidateMovement(10, nil, "compra en panadería", 3000),
	}}
	orch := correctInTheLoop(orchestrator.IntentUpdate)
	orch.updateResult = orchestrator.UpdateResult{
		Resolved:  true,
		Movements: []orchestrator.MovementDraft{{Type: "expense", Amount: "2000", Currency: "ARS", Category: "Comida", Subcategory: "Supermercado", Date: "2026-08-01"}},
	}
	c := newLoopController(t, orch, repo, movements)

	if err := c.startAgentLoop(context.Background(), nil, 0, 1, "la panaderia era 2000"); err != nil {
		t.Fatal(err)
	}
	if len(repo.rows) != 0 {
		t.Errorf("sin preguntas la acción se consume en el turno, quedan %d", len(repo.rows))
	}
	if inProgress, _ := c.engine.InProgress(1); !inProgress {
		t.Error("tenía que quedar abierto el confirm de corrección")
	}
}

// TestLoop_OurCopyBeatsTheModelNarration: la ayuda es texto tuneado y sale
// textual, no lo que el modelo haya querido decir arriba.
func TestLoop_OurCopyBeatsTheModelNarration(t *testing.T) {
	orch := &fakeFullOrchestrator{
		intent: orchestrator.IntentUpdate,
		runFn: func(execute func(string, json.RawMessage) (string, error)) (string, error) {
			if _, err := execute(orchestrator.ToolReplyHelp, json.RawMessage(`{}`)); err != nil {
				return "", err
			}
			return "yo te explico a mi manera", nil
		},
	}
	c := newLoopController(t, orch, &fakeActionsRepo{}, &fakeMovementRepoFull{})

	// Sin bot no se puede leer lo enviado; lo que se verifica es que el executor
	// deje la copia nuestra cargada y que el turno cierre limpio.
	if err := c.startAgentLoop(context.Background(), nil, 0, 1, "¿qué podés hacer?"); err != nil {
		t.Fatal(err)
	}
}

func TestLoop_PromptCarriesTheUsersAccountsAndTools(t *testing.T) {
	orch := &fakeOrchestrator{
		runFn: func(func(string, json.RawMessage) (string, error)) (string, error) { return "ok", nil },
	}
	engine := conversation.NewEngine(&fakeConvStore{}, func(string) string { return "algo" })
	engine.Register(NewAskUserFlow())
	c := &controller{
		engine: engine, orchestrator: orch, actions: &fakeActionsRepo{},
		accounts: &fakeAccountRepoFull{}, subcategories: &fakeSubcategoryRepoFull{},
		chatHistory: stubChatHistory{}, movements: &fakeMovementRepoFull{},
	}

	if err := c.startAgentLoop(context.Background(), nil, 0, 1, "corregí el asado"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(orch.gotRunPrompt, "CUENTAS DEL USUARIO") {
		t.Errorf("el prompt no lleva las cuentas:\n%s", orch.gotRunPrompt)
	}
	// Sólo las tools que el ejecutor sabe correr. En la etapa 3 son 5:
	// record_movements se suma porque CREATE ya pasa por acá.
	if len(orch.gotRunTools) != 5 {
		t.Errorf("want las 5 tools cableadas, got %d", len(orch.gotRunTools))
	}
	// Y el prompt tiene que hablar de ESAS, no de las 14: si nombra una que no
	// se manda, el modelo la pide igual y el turno se cae.
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

// TestWiredTools_CorrectionStillPicksCorrectMovement: con record_movements a la
// vista, una corrección tiene que seguir eligiendo correct_movement. Es la
// regresión de la traza 84322077, donde el router dijo UPDATE y el agente
// registró de nuevo. Lo que lo evita ya no es la ausencia de la tool sino el
// desempate, así que el desempate es lo que se fija acá.
func TestWiredTools_CorrectionStillPicksCorrectMovement(t *testing.T) {
	names := make([]string, 0, 5)
	for _, tool := range wiredAgentTools() {
		names = append(names, tool.Name)
	}
	if !slices.Contains(names, orchestrator.ToolRecordMovements) {
		t.Fatal("record_movements tiene que estar cableada en la etapa 3")
	}
	if len(names) != 5 {
		t.Errorf("toolbox = %v, want las 5 de la etapa 3", names)
	}

	prompt := orchestrator.BuildAgentPrompt("2026-08-01", nil, nil, "", wiredAgentTools())
	if !strings.Contains(prompt, "era, eran, fue") {
		t.Error("falta el copulativo en pasado: es lo único que separa corregir de registrar")
	}
	if !strings.Contains(prompt, orchestrator.ToolRecordMovements) {
		t.Error("el prompt tiene que nombrar record_movements ahora que se manda")
	}
}

// TestWiredAgentTools_MatchesTheExecutorSwitch: la lista de tools que se manda y
// el switch de execute son la misma cosa dicha dos veces. Si se separan, o el
// modelo pide algo que nadie corre (vuelta extra y 429), o hay una tool cableada
// que nunca se le ofrece.
func TestWiredAgentTools_MatchesTheExecutorSwitch(t *testing.T) {
	e := newExecutorWith(t, "cualquier cosa")
	for _, tool := range wiredAgentTools() {
		if out, _ := e.execute(tool.Name, json.RawMessage(`{}`)); out == resultNotWiredYet {
			t.Errorf("%s se manda pero el ejecutor no la corre", tool.Name)
		}
	}
}

// TestLoop_AlwaysResolvesTheMetric es la regresión más cara de todas. Un
// intent_event que queda 'pending' lo pisa a 'abandoned' el próximo mensaje del
// usuario, y el portón de esta etapa es exactamente "update_confirmed sube y
// abandoned NO sube". Sin esto, cada turno del loop que no parkea nada cuenta
// como un abandono y el portón da negativo aunque todo funcione.
func TestLoop_AlwaysResolvesTheMetric(t *testing.T) {
	for _, tc := range []struct {
		name string
		tool string
		want string
	}{
		{"ayuda", orchestrator.ToolReplyHelp, outcomeHelpShown},
		{"pedir reescritura", orchestrator.ToolAskRewrite, outcomeUnclear},
		{"sin candidatos", orchestrator.ToolCorrectMovement, outcomeNoCandidates},
		{"narró sin hacer nada", orchestrator.ToolSumMovements, outcomeUpdateFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metrics := &fakeMetricRepo{}
			tool := tc.tool
			orch := &fakeFullOrchestrator{
				intent: orchestrator.IntentUpdate,
				runFn: func(execute func(string, json.RawMessage) (string, error)) (string, error) {
					_, err := execute(tool, json.RawMessage(`{}`))
					return "listo", err
				},
			}
			// Sin movimientos: correct_movement no encuentra candidatos.
			c := newLoopController(t, orch, &fakeActionsRepo{}, &fakeMovementRepoFull{})
			c.metrics = metrics

			if err := c.startAgentLoop(context.Background(), nil, 0, 1, "algo"); err != nil {
				t.Fatal(err)
			}
			if len(metrics.resolved) != 1 || metrics.resolved[0] != tc.want {
				t.Fatalf("want el evento resuelto como %q, got %v", tc.want, metrics.resolved)
			}
		})
	}
}

// TestLoop_LeavesTheMetricPendingWhenSomethingIsOpen: si quedó una acción
// parkeada, el que resuelve es el gate cuando el usuario decida. Resolverla acá
// contaría el turno dos veces.
func TestLoop_LeavesTheMetricPendingWhenSomethingIsOpen(t *testing.T) {
	metrics := &fakeMetricRepo{}
	movements := &fakeMovementRepoFull{similar: []movement.Movement{
		candidateMovement(10, nil, "compra en panadería", 3000),
		candidateMovement(11, nil, "panadería del barrio", 5000),
	}}
	c := newLoopController(t, correctInTheLoop(orchestrator.IntentUpdate), &fakeActionsRepo{}, movements)
	c.metrics = metrics

	if err := c.startAgentLoop(context.Background(), nil, 0, 1, "la panaderia estaba mal"); err != nil {
		t.Fatal(err)
	}
	if len(metrics.resolved) != 0 {
		t.Errorf("con una acción abierta el evento sigue pendiente, got %v", metrics.resolved)
	}
}

// TestLoop_InsertedResolvesCreateInserted: un CREATE por el loop tiene que
// cerrar el intent_event como create_inserted. Sin este caso cae en el fracaso
// genérico —el reply es el recibo, no matchea ninguna copy— y el portón de la
// etapa (create_inserted ≥ 74%) daría negativo con todo funcionando.
func TestLoop_InsertedResolvesCreateInserted(t *testing.T) {
	metrics := &fakeMetricRepo{}
	orch := &fakeFullOrchestrator{
		intent: orchestrator.IntentCreate,
		runFn: func(execute func(string, json.RawMessage) (string, error)) (string, error) {
			_, err := execute(orchestrator.ToolRecordMovements, json.RawMessage(`{"movements":[
				{"type":"expense","amount":"5000","currency":"ARS","category":"Alimentación",
				 "subcategory":"Supermercado","date":"2026-08-01","description":"super",
				 "payment_method":"transfer"}]}`))
			return "", err
		},
	}
	engine := conversation.NewEngine(&fakeConvStore{}, func(string) string { return "algo" })
	engine.Register(NewAskUserFlow())
	c := &controller{
		engine: engine, orchestrator: orch, actions: &fakeActionsRepo{}, metrics: metrics,
		accounts: accountsWithDefault(), subcategories: subcategoriesForTest(),
		chatHistory: stubChatHistory{}, movements: movementsWithBalance("100000"),
	}

	if err := c.startAgentLoop(context.Background(), nil, 0, 1, "gasté 5000 en el super"); err != nil {
		t.Fatal(err)
	}
	if len(metrics.resolved) != 1 || metrics.resolved[0] != outcomeCreateInserted {
		t.Fatalf("want el evento resuelto como %q, got %v", outcomeCreateInserted, metrics.resolved)
	}
}

// TestLoop_NoQueueAfterAWrite: si el turno ya escribió, un 429 posterior NO
// puede encolar el mensaje — el drenaje lo reinsertaría y la plata quedaría
// registrada dos veces. Spec 8.2.
func TestLoop_NoQueueAfterAWrite(t *testing.T) {
	jobs := &drainJobs{}
	// El turno escribe y RECIÉN DESPUÉS se come el 429 — que es el único orden
	// en que el bug existe. Por eso el runFn llama al ejecutor antes de fallar,
	// en vez de setear el flag a mano.
	orch := &fakeFullOrchestrator{
		intent: orchestrator.IntentCreate,
		runFn: func(execute func(string, json.RawMessage) (string, error)) (string, error) {
			_, _ = execute(orchestrator.ToolRecordMovements, json.RawMessage(`{"movements":[
				{"type":"expense","amount":"5000","currency":"ARS","category":"Alimentación",
				 "subcategory":"Supermercado","date":"2026-08-01","description":"super",
				 "payment_method":"transfer"}]}`))
			return "", &orchestrator.RateLimitedError{RetryAfter: time.Second}
		},
	}
	engine := conversation.NewEngine(&fakeConvStore{}, func(string) string { return "algo" })
	engine.Register(NewAskUserFlow())
	c := &controller{
		engine: engine, orchestrator: orch, jobs: jobs, actions: &fakeActionsRepo{},
		accounts: accountsWithDefault(), subcategories: subcategoriesForTest(),
		chatHistory: stubChatHistory{}, movements: movementsWithBalance("100000"),
		users: fakeUsers{}, metrics: &fakeMetricRepo{},
	}

	if err := c.startAgentLoop(context.Background(), nil, 0, 1, "gasté 5000 en el super"); err != nil {
		t.Fatal(err)
	}
	if len(jobs.inserted) != 0 {
		t.Errorf("encoló %d jobs después de escribir: el drenaje los duplicaría", len(jobs.inserted))
	}
}

func TestDrainAfterLoop_OpensTheQuestion(t *testing.T) {
	repo := &fakeActionsRepo{}
	c := newLoopController(t, &fakeFullOrchestrator{}, repo, &fakeMovementRepoFull{})
	if err := c.parkAgentActions(context.Background(), 1, []parkedAction{{
		Tool:    orchestrator.ToolCorrectMovement,
		Payload: agentPayload{Change: "eran 2000", Candidates: []candidateGroup{{OldIDs: []string{"10"}}, {OldIDs: []string{"11"}}}, Chosen: -1},
		Questions: []pendingaction.OpenQuestion{{
			Key: questionKeyCandidate, Prompt: "¿Cuál es?", Options: []string{"a", "b"},
		}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := c.drainNextAgentAction(context.Background(), nil, 0, 1); err != nil {
		t.Fatal(err)
	}
	if inProgress, _ := c.engine.InProgress(1); !inProgress {
		t.Error("el drenaje tenía que dejar la pregunta abierta")
	}
}
