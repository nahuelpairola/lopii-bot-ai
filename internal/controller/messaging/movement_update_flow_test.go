package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
)

type fakeOrchestrator struct {
	updateResult      orchestrator.UpdateResult
	updateErr         error
	gotUpdateAccounts []orchestrator.AccountOption
	// runFn deja que un test maneje el loop unificado. Sin setear, Run falla
	// fuerte: un camino que llegue ahí sin quererlo migró antes de su etapa.
	runFn        func(execute func(string, json.RawMessage) (string, error)) (string, error)
	gotRunPrompt string
	gotRunTools  []orchestrator.AgentTool
}

func (o *fakeOrchestrator) ClassifyIntent(ctx context.Context, text string) (orchestrator.IntentResult, error) {
	return orchestrator.IntentResult{}, nil
}
func (o *fakeOrchestrator) ClassifyCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry, accounts []orchestrator.AccountOption, today string) (orchestrator.CreateResult, error) {
	return orchestrator.CreateResult{}, nil
}
func (o *fakeOrchestrator) ResolveUpdate(ctx context.Context, text string, candidate orchestrator.MovementCandidate, accounts []orchestrator.AccountOption) (orchestrator.UpdateResult, error) {
	o.gotUpdateAccounts = accounts
	return o.updateResult, o.updateErr
}
func (o *fakeOrchestrator) ResolveDelete(ctx context.Context, text string, candidate orchestrator.MovementCandidate) (orchestrator.DeleteResult, error) {
	return orchestrator.DeleteResult{}, nil
}
func (o *fakeOrchestrator) ClassifyOnboarding(ctx context.Context, text string) (orchestrator.OnboardingResult, error) {
	return orchestrator.OnboardingResult{}, nil
}
func (o *fakeOrchestrator) AnswerQuery(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error) {
	return "", nil
}

// errRunNotWired es lo que devuelven los fakes cuando el test no programó el
// loop. Un camino que llegue ahí sin querer migró antes de su etapa, y tiene que
// fallar fuerte en vez de recibir una respuesta vacía plausible.
var errRunNotWired = errors.New("Run is not wired in this test")

// swallowTurnDone imita lo que el Run de verdad hace con ErrAgentTurnDone: no es
// un error, es el executor avisando que la app se queda con el turno. Sin esto
// cada fake lo propagaría como fallo y el test vería rojo donde el código real
// ve un turno normal — de una sola vuelta, que es justo el punto.
func swallowTurnDone(execute func(string, json.RawMessage) (string, error)) func(string, json.RawMessage) (string, error) {
	return func(name string, args json.RawMessage) (string, error) {
		result, err := execute(name, args)
		if errors.Is(err, orchestrator.ErrAgentTurnDone) {
			return result, nil
		}
		return result, err
	}
}

func (o *fakeOrchestrator) Run(_ context.Context, systemPrompt, _ string, _ []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(string, json.RawMessage) (string, error)) (string, error) {
	o.gotRunPrompt = systemPrompt
	o.gotRunTools = tools
	if o.runFn == nil {
		return "", errRunNotWired
	}
	return o.runFn(swallowTurnDone(execute))
}
func (o *fakeOrchestrator) ClassifyCategoryCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry) (orchestrator.CategoryCreateResult, error) {
	return orchestrator.CategoryCreateResult{}, nil
}
func (o *fakeOrchestrator) ResolveAccountManage(ctx context.Context, text string, accounts []orchestrator.AccountOption) (orchestrator.AccountManageResult, error) {
	return orchestrator.AccountManageResult{}, nil
}

type fakeStoreForController struct {
	flowName, stepName string
	data               conversation.Data
	updatedAt          time.Time
	found              bool
}

func (s *fakeStoreForController) Get(userID uint64) (string, string, conversation.Data, time.Time, bool, error) {
	return s.flowName, s.stepName, s.data, s.updatedAt, s.found, nil
}
func (s *fakeStoreForController) Set(userID uint64, flowName, stepName string, data conversation.Data) error {
	s.flowName, s.stepName, s.data, s.found = flowName, stepName, data, true
	s.updatedAt = time.Now()
	return nil
}
func (s *fakeStoreForController) Clear(userID uint64) error {
	s.found = false
	return nil
}

func TestMovementToRow_ResolvesCategoryFromSubcategory(t *testing.T) {
	sub := newSubForTest(3, "Transporte", "Nafta")
	m := movement.Movement{SubcategoryID: 3, Subcategory: sub, Type: movement.Expense, Amount: mustDecimal(t, "15000"), Currency: "ARS"}

	row := movementToRow(m)
	if row.Category != "Transporte" || row.Subcategory != "Nafta" {
		t.Errorf("row category/subcategory = %q/%q, want Transporte/Nafta", row.Category, row.Subcategory)
	}
	if row.Amount != "15000" {
		t.Errorf("row amount = %q, want 15000", row.Amount)
	}
}

func TestMovementToRow_EmitsAbsAmount(t *testing.T) {
	id := uint64(1)
	m := movement.Movement{Type: movement.Expense, Amount: decimal.NewFromInt(-100000), Currency: currency.ARS, AccountID: &id, Date: time.Now()}
	row := movementToRow(m)
	if row.Amount != "100000" {
		t.Fatalf("row.Amount = %q, want positive 100000 (abs boundary — the LLM must never see the stored sign)", row.Amount)
	}
}

func TestRowToDraft_RoundTripsAccountID(t *testing.T) {
	row := movementRow{Type: "transfer", Amount: "100", Currency: "USD", AccountID: "5"}
	draft := rowToDraft(row)
	if draft.AccountID == nil || *draft.AccountID != 5 {
		t.Errorf("draft.AccountID = %v, want 5", draft.AccountID)
	}
}

func TestDraftToRow_RoundTripsAccountID(t *testing.T) {
	id := uint64(9)
	row := draftToRow(orchestrator.MovementDraft{Type: "transfer", Amount: "100", Currency: "USD", AccountID: &id})
	if row.AccountID != "9" {
		t.Errorf("row.AccountID = %q, want 9", row.AccountID)
	}
}

func TestEncodeDecodeCandidateGroups_RoundTrip(t *testing.T) {
	sub := newSubForTest(1, "Alimentación", "Nafta")
	groups := []transactionGroup{
		{TransactionID: "", Movements: []movement.Movement{{SubcategoryID: 1, Subcategory: sub, Amount: mustDecimal(t, "3000"), Currency: "ARS"}}},
	}

	encoded := encodeCandidateGroups(groups)
	decoded := decodeCandidateGroups(conversation.Data{"candidate_groups": encoded})

	if len(decoded) != 1 {
		t.Fatalf("got %d candidates, want 1", len(decoded))
	}
	if len(decoded[0].Rows) != 1 || decoded[0].Rows[0].Amount != "3000" {
		t.Errorf("decoded rows = %+v, want amount 3000", decoded[0].Rows)
	}
}

func TestProceedToUpdateConfirm_SeedsConfirmFlowOnResolved(t *testing.T) {
	orch := &fakeOrchestrator{updateResult: orchestrator.UpdateResult{
		Resolved: true,
		Movements: []orchestrator.MovementDraft{
			{Type: "expense", Amount: "3500", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", Description: "Café", Date: "2026-07-02"},
		},
	}}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewMovementUpdateConfirmFlow())

	accRepo := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true), acct(7, currency.ARS, false)}}
	c := &controller{orchestrator: orch, engine: engine, subcategories: &fakeSubcategoryRepoFull{}, accounts: accRepo}

	beforeRows := []movementRow{{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café"}}
	if err := c.proceedToUpdateConfirm(context.Background(), nil, 0, 1, "en realidad fue 3500", "", []string{"42"}, beforeRows, changeAsk{}); err != nil {
		t.Fatalf("proceedToUpdateConfirm: %v", err)
	}
	if store.flowName != movementUpdateConfirmFlowName {
		t.Errorf("started flow = %q, want %q", store.flowName, movementUpdateConfirmFlowName)
	}
	if len(orch.gotUpdateAccounts) == 0 {
		t.Error("ResolveUpdate should receive the user's accounts so it can re-target by name, got none")
	}
}

func TestProceedToUpdateConfirm_UnresolvedSendsNoDBCall(t *testing.T) {
	orch := &fakeOrchestrator{updateResult: orchestrator.UpdateResult{Resolved: false}}
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewMovementUpdateConfirmFlow())
	c := &controller{orchestrator: orch, engine: engine, accounts: &fakeAccountRepoFull{}}

	if err := c.proceedToUpdateConfirm(context.Background(), nil, 0, 1, "che no sé", "", nil, nil, changeAsk{}); err != nil {
		t.Fatalf("proceedToUpdateConfirm: %v", err)
	}
	if store.found {
		t.Error("an unresolved result should never start the confirm flow")
	}
}

// TestUpdate_UnresolvedChangeAsksWhatToChange: "el café estaba mal" nombra bien
// el movimiento y no dice qué cambiarle. Antes moría acá con un "no me quedó
// claro"; ahora pregunta, que es la máquina de preguntas que la etapa 2 ya
// construyó.
func TestUpdate_UnresolvedChangeAsksWhatToChange(t *testing.T) {
	actions := &fakeActionsRepo{}
	engine := conversation.NewEngine(&fakeConvStore{}, func(string) string { return "algo" })
	engine.Register(NewAskUserFlow())
	c := &controller{
		engine: engine, actions: actions,
		orchestrator: &fakeOrchestrator{updateResult: orchestrator.UpdateResult{Resolved: false}},
		movements:    &fakeMovementRepoFull{}, accounts: &fakeAccountRepoFull{},
	}

	err := c.proceedToUpdateConfirm(context.Background(), nil, 0, 1,
		"estaba mal", "", []string{"10"}, []movementRow{{Amount: "3000", Currency: "ARS", Description: "café"}}, changeAsk{})
	if err != nil {
		t.Fatal(err)
	}
	if len(actions.rows) != 1 {
		t.Fatalf("tenía que parkear la pregunta, parkeó %d", len(actions.rows))
	}
	var qs []pendingaction.OpenQuestion
	if err := json.Unmarshal(actions.rows[0].Questions, &qs); err != nil {
		t.Fatal(err)
	}
	if len(qs) != 1 || qs[0].Key != questionKeyChange {
		t.Fatalf("la pregunta abierta tiene que ser qué cambiar: %+v", qs)
	}
	// Botones para los campos que NO son el monto. El monto se escribe derecho,
	// así que el caso común queda en un paso; los otros encadenan la pregunta
	// del valor. Y ask_user acepta texto libre igual: los botones aceleran,
	// nunca encierran.
	if len(qs[0].Options) != len(changeFieldOptions()) {
		t.Errorf("faltan los botones de campo: %+v", qs[0].Options)
	}
	for _, opt := range qs[0].Options {
		if strings.Contains(opt, "monto") {
			t.Errorf("el monto no va como botón, se escribe: %q", opt)
		}
	}
	// El candidato ya está elegido: encontrarlo fue la mitad cara y no se repite.
	var payload agentPayload
	if err := json.Unmarshal(actions.rows[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Chosen != 0 || len(payload.Candidates) != 1 || payload.Candidates[0].OldIDs[0] != "10" {
		t.Errorf("el candidato resuelto no viajó: %+v", payload)
	}
}

// TestMsgAskWhatToChange_AsksForTheValueNotTheField: la primera versión listaba
// "(el monto, la categoría, la fecha…)" y se leía como un menú. En la prueba
// real el usuario contestó "El monto" — el campo, que es justo lo que no sirve:
// ResolveUpdate necesita con qué reemplazar, y la corrección murió ahí.
func TestMsgAskWhatToChange_AsksForTheValueNotTheField(t *testing.T) {
	got := msgAskWhatToChange([]movementRow{{Amount: "1800", Description: "Cafe"}})

	if !strings.Contains(got, "Cuánto era") {
		t.Errorf("no pide el valor nuevo: %q", got)
	}
	// Y avisa que hay botones para lo que no sea el monto: sin eso el usuario no
	// sabe que puede corregir la categoría o la fecha.
	if !strings.Contains(got, "tocá abajo") {
		t.Errorf("no ofrece los botones para los otros campos: %q", got)
	}
	if strings.Contains(got, "el monto, la categoría") {
		t.Errorf("volvió la lista de campos en el texto, que se lee como menú: %q", got)
	}
	// Y nombra el movimiento, para que se sepa cuál se está tocando.
	if !strings.Contains(got, "1800") {
		t.Errorf("no nombra el movimiento: %q", got)
	}
}

// TestUpdate_NoOpCorrectionAsksInsteadOfConfirming reproduce la traza 317df846:
// "el café estaba mal" contra un movimiento de $1.800 y ResolveUpdate devolvió
// Resolved=TRUE con el mismo $1.800. Confirmarlo haría un DELETE+INSERT para
// dejar todo igual y contaría como update_confirmed.
//
// El chequeo no puede depender de que el modelo se declare incapaz.
func TestUpdate_NoOpCorrectionAsksInsteadOfConfirming(t *testing.T) {
	before := []movementRow{{Type: "expense", Amount: "1800", Currency: "ARS",
		Category: "Ocio y salidas", Subcategory: "Salir a comer", Date: "2026-08-01", Description: "Cafe"}}
	actions := &fakeActionsRepo{}
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewAskUserFlow())
	engine.Register(NewMovementUpdateConfirmFlow())
	c := &controller{
		engine: engine, actions: actions, movements: &fakeMovementRepoFull{},
		accounts:      &fakeAccountRepoFull{},
		subcategories: &fakeSubcategoryRepoFull{},
		orchestrator: &fakeOrchestrator{updateResult: orchestrator.UpdateResult{
			Resolved:  true,                                                // el modelo dice que sí...
			Movements: []orchestrator.MovementDraft{rowToDraft(before[0])}, // ...y no cambió nada
		}},
	}

	if err := c.proceedToUpdateConfirm(context.Background(), nil, 0, 1,
		"El café estaba mal", "", []string{"127"}, before, changeAsk{}); err != nil {
		t.Fatal(err)
	}

	if store.flowName == movementUpdateConfirmFlowName {
		t.Fatal("una corrección que no cambia nada no puede llegar al gate de confirmación")
	}
	if len(actions.rows) != 1 {
		t.Fatalf("tenía que preguntar qué cambiar, parkeó %d", len(actions.rows))
	}
	var qs []pendingaction.OpenQuestion
	if err := json.Unmarshal(actions.rows[0].Questions, &qs); err != nil {
		t.Fatal(err)
	}
	if len(qs) != 1 || qs[0].Key != questionKeyChange {
		t.Fatalf("la pregunta tiene que ser qué cambiar: %+v", qs)
	}
}

// TestUpdate_PickedFieldAsksForTheValueWithoutCallingTheModel: tocar "La
// categoría" nombra el CAMPO y nada más. Preguntar el valor antes de llamar al
// modelo ahorra la llamada entera — iba a volver sin cambiar nada.
func TestUpdate_PickedFieldAsksForTheValueWithoutCallingTheModel(t *testing.T) {
	actions := &fakeActionsRepo{}
	engine := conversation.NewEngine(&fakeConvStore{}, func(string) string { return "algo" })
	engine.Register(NewAskUserFlow())
	// orchestrator nil: si llamara a ResolveUpdate, panichearía. Ésa ES la prueba.
	c := &controller{engine: engine, actions: actions, accounts: &fakeAccountRepoFull{}}

	err := c.proceedToUpdateConfirm(context.Background(), nil, 0, 1,
		"El café estaba mal La categoría", "", []string{"127"},
		[]movementRow{{Amount: "1800", Description: "Cafe"}},
		changeAsk{pickedField: true})
	if err != nil {
		t.Fatal(err)
	}

	if len(actions.rows) != 1 {
		t.Fatalf("tenía que parkear la pregunta del valor, parkeó %d", len(actions.rows))
	}
	var qs []pendingaction.OpenQuestion
	if err := json.Unmarshal(actions.rows[0].Questions, &qs); err != nil {
		t.Fatal(err)
	}
	if qs[0].Prompt != msgAskChangeValue {
		t.Errorf("la segunda vuelta pregunta el valor, no el campo: %q", qs[0].Prompt)
	}
	// Y sin botones: el campo ya se eligió, ofrecerlos de nuevo confunde.
	if len(qs[0].Options) != 0 {
		t.Errorf("la segunda vuelta no lleva botones: %+v", qs[0].Options)
	}
}

// TestUpdate_AmountAnswerSkipsTheModel: preguntamos "¿Cuánto era?" y contestó
// un número. No queda nada que interpretar — parseARAmount ya lo sabe leer— así
// que la corrección se arma del lado de la app. Mandárselo al modelo costaba
// ~1.500 tokens para que copiara el número.
func TestUpdate_AmountAnswerSkipsTheModel(t *testing.T) {
	before := []movementRow{{Type: "expense", Amount: "1800", Currency: "ARS", AccountID: "46",
		Category: "Ocio y salidas", Subcategory: "Salir a comer", Description: "Cafe"}}
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewMovementUpdateConfirmFlow())
	// orchestrator nil: si llamara a ResolveUpdate, panichearía. Ésa ES la prueba.
	c := &controller{engine: engine, accounts: &fakeAccountRepoFull{}, subcategories: &fakeSubcategoryRepoFull{}}

	err := c.proceedToUpdateConfirm(context.Background(), nil, 0, 1,
		"el café estaba mal 2000", "", []string{"127"}, before,
		changeAsk{gaveValue: true, answer: "2000"})
	if err != nil {
		t.Fatal(err)
	}

	// El gate NO se saltea: el usuario tiene que ver el antes/después igual.
	if store.flowName != movementUpdateConfirmFlowName {
		t.Fatalf("tenía que abrir el confirm, abrió %q", store.flowName)
	}
	after := decodeMovementRows(store.data)
	if len(after) != 1 || after[0].Amount != "2000" {
		t.Fatalf("el monto nuevo no llegó: %+v", after)
	}
	// Y el resto del movimiento queda intacto — sobre todo la cuenta, que es
	// de donde sale la plata.
	if after[0].AccountID != "46" || after[0].Subcategory != "Salir a comer" {
		t.Errorf("el atajo tocó algo que no era el monto: %+v", after[0])
	}
}

// TestAmountOnlyCorrection_FallsBackWhenItIsNotJustTheAmount: las condiciones
// del atajo son todas necesarias. Cualquiera que falte vuelve al camino con
// modelo, que es el que sabe interpretar.
func TestAmountOnlyCorrection_FallsBackWhenItIsNotJustTheAmount(t *testing.T) {
	one := []movementRow{{Amount: "1800", Currency: "ARS"}}
	two := []movementRow{{Amount: "1800"}, {Amount: "1800"}}

	for name, tc := range map[string]struct {
		rows []movementRow
		ask  changeAsk
	}{
		"tocó un botón, el campo no es el monto": {one, changeAsk{gaveValue: true, pickedField: true, answer: "2000"}},
		"no es un número":                        {one, changeAsk{gaveValue: true, answer: "era en Delivery"}},
		"transferencia de dos piernas":           {two, changeAsk{gaveValue: true, answer: "2000"}},
		"monto cero (es un borrado)":             {one, changeAsk{gaveValue: true, answer: "0"}},
		"todavía no contestó nada":               {one, changeAsk{}},
	} {
		if _, ok := amountOnlyCorrection(tc.rows, tc.ask); ok {
			t.Errorf("%s: no puede tomar el atajo", name)
		}
	}
}

// TestUpdate_NoOpAfterAskingGivesUp: si ya preguntamos y con la respuesta
// TAMPOCO sale una corrección, se corta. Sin esto cada vuelta parkea una acción
// nueva con presupuesto entero y el usuario gira para siempre.
func TestUpdate_NoOpAfterAskingGivesUp(t *testing.T) {
	before := []movementRow{{Type: "expense", Amount: "1800", Currency: "ARS", Description: "Cafe"}}
	actions := &fakeActionsRepo{}
	metrics := &fakeMetricRepo{}
	c := &controller{
		engine:  conversation.NewEngine(&fakeStoreForController{}, func(string) string { return "algo" }),
		actions: actions, metrics: metrics, movements: &fakeMovementRepoFull{},
		accounts:      &fakeAccountRepoFull{},
		subcategories: &fakeSubcategoryRepoFull{},
		orchestrator: &fakeOrchestrator{updateResult: orchestrator.UpdateResult{
			Resolved: true, Movements: []orchestrator.MovementDraft{rowToDraft(before[0])},
		}},
	}

	if err := c.proceedToUpdateConfirm(context.Background(), nil, 0, 1,
		"El café estaba mal no sé", "", []string{"127"}, before, changeAsk{gaveValue: true}); err != nil {
		t.Fatal(err)
	}

	if len(actions.rows) != 0 {
		t.Errorf("ya se preguntó una vez: no puede volver a parkear la misma pregunta")
	}
	if len(metrics.resolved) != 1 || metrics.resolved[0] != outcomeLoopDidNothing {
		t.Errorf("el evento tiene que cerrar como fallo, got %v", metrics.resolved)
	}
}

// TestCorrectionIsNoOp_DetectsARealChange: la contracara. Un campo omitido por
// el modelo es "no lo tocó", pero uno distinto es un cambio de verdad y tiene
// que pasar derecho al gate.
func TestCorrectionIsNoOp_DetectsARealChange(t *testing.T) {
	before := []movementRow{{Type: "expense", Amount: "1800", Currency: "ARS",
		Category: "Ocio y salidas", Subcategory: "Salir a comer", Description: "Cafe"}}

	same := []orchestrator.MovementDraft{{Amount: "1800.00"}} // mismo monto, otro formato
	if !correctionIsNoOp(before, same) {
		t.Error("1800 y 1800.00 son el mismo monto: tiene que dar no-op")
	}

	changed := []orchestrator.MovementDraft{{Amount: "2000"}}
	if correctionIsNoOp(before, changed) {
		t.Error("cambió el monto: NO es no-op")
	}

	recat := []orchestrator.MovementDraft{{Amount: "1800", Subcategory: "Delivery"}}
	if correctionIsNoOp(before, recat) {
		t.Error("cambió la subcategoría: NO es no-op")
	}

	if correctionIsNoOp(before, nil) {
		t.Error("sin filas nuevas no hay con qué comparar: no puede dar no-op")
	}
}

// TestApplyAnswers_ChangeAnswerIsAppended: la respuesta se suma al texto
// original en vez de reemplazarlo. El original dice a cuál ("el café"), la
// respuesta dice el valor nuevo ("2000"): con uno solo ResolveUpdate no cierra.
func TestApplyAnswers_ChangeAnswerIsAppended(t *testing.T) {
	action := &pendingaction.PendingAction{Payload: mustJSON(t, agentPayload{
		Change: "el café estaba mal", Candidates: []candidateGroup{{OldIDs: []string{"10"}}}, Chosen: 0,
	})}
	answers := []pendingaction.OpenQuestion{{Key: questionKeyChange, Answer: "eran 2000"}}

	payload, resolved := applyAnswers(action, answers)
	if !resolved {
		t.Fatal("contestar qué cambiar tiene que resolver la acción")
	}
	if payload.Change != "el café estaba mal eran 2000" {
		t.Errorf("change = %q; se perdió una de las dos mitades", payload.Change)
	}
}

func TestSeedAndStartUpdateConfirm_NeverCallsOrchestrator(t *testing.T) {
	// orchestrator is deliberately nil — this function must not call it.
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewMovementUpdateConfirmFlow())
	c := &controller{engine: engine, subcategories: &fakeSubcategoryRepoFull{}, accounts: &fakeAccountRepoFull{}}

	result := orchestrator.UpdateResult{Resolved: true, Movements: []orchestrator.MovementDraft{
		{Type: "expense", Amount: "3500", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", Date: "2026-07-02"},
	}}
	if err := c.seedAndStartUpdateConfirm(context.Background(), nil, 0, 1, "eran 3500", []string{"7"}, nil, result); err != nil {
		t.Fatalf("seedAndStartUpdateConfirm: %v", err)
	}
	if store.flowName != movementUpdateConfirmFlowName {
		t.Errorf("started flow = %q, want %q", store.flowName, movementUpdateConfirmFlowName)
	}
}

func TestCorrectionIsDeletion(t *testing.T) {
	cases := []struct {
		name    string
		rows    []movementRow
		message string
		want    bool
	}{
		{"empty set is not a deletion", nil, "salió 0", false},
		{"single zero row deletes", []movementRow{{Amount: "0"}}, "en realidad fue 0", true},
		{"zero with decimals deletes", []movementRow{{Amount: "0.00"}}, "0 pesos", true},
		{"non-zero is a real correction", []movementRow{{Amount: "600"}}, "eran 600", false},
		{"mixed zero and non-zero is not a deletion", []movementRow{{Amount: "0"}, {Amount: "500"}}, "poné 0 y 500", false},
		{"unparseable amount is not a deletion", []movementRow{{Amount: ""}}, "gratis", false},

		// El caso que casi borra datos: el 2026-08-10 "Editá los movimientos de
		// lote de hoy" no dice ningún cambio, el modelo devolvió montos en 0, y
		// esto armó un borrado que el usuario confirmó. Sólo no borró porque la
		// escritura falló — y ese accidente ya no está.
		{"todo cero SIN monto en el mensaje NO borra", []movementRow{{Amount: "0"}, {Amount: "0"}},
			"Editá los movimientos de lote de hoy", false},
		{"tampoco con un solo movimiento", []movementRow{{Amount: "0"}},
			"editá el café", false},

		// Las formas de decir "no salió nada" que no traen ningún dígito.
		{"me lo regalaron", []movementRow{{Amount: "0"}}, "me regalaron el helado", true},
		{"al final fue gratis", []movementRow{{Amount: "0"}}, "al final fue gratis", true},
		{"no me cobraron nada", []movementRow{{Amount: "0"}}, "no me cobraron nada", true},
		{"me invitaron, con acento de por medio", []movementRow{{Amount: "0"}}, "me invitó él", true},
	}
	for _, tc := range cases {
		if got := correctionIsDeletion(tc.rows, tc.message); got != tc.want {
			t.Errorf("%s: correctionIsDeletion(%q) = %v, want %v", tc.name, tc.message, got, tc.want)
		}
	}
}
