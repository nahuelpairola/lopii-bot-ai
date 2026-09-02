package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
)

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
	row := movement.MovementRow{Type: "transfer", Amount: "100", Currency: "USD", AccountID: "5"}
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

	encoded := EncodeCandidateGroups(groups)
	decoded := flow.DecodeCandidateGroups(conversation.Data{"candidate_groups": encoded})

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

	store := &fakeConvStore{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow.NewMovementUpdateConfirmFlow())

	accRepo := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true), acct(7, currency.ARS, false)}}
	svc := &fakeServices{orch: orch, engine: engine, subcategories: &fakeSubcategoryRepoFull{}, accounts: accRepo}

	beforeRows := []movement.MovementRow{{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café"}}
	if err := proceedToUpdateConfirm(context.Background(), svc, &messenger.FakeChat{}, 1, "en realidad fue 3500", "", []string{"42"}, beforeRows, ChangeAsk{}); err != nil {
		t.Fatalf("proceedToUpdateConfirm: %v", err)
	}
	if store.flowName != flow.MovementUpdateConfirmFlowName {
		t.Errorf("started flow = %q, want %q", store.flowName, flow.MovementUpdateConfirmFlowName)
	}
	if len(orch.gotUpdateAccounts) == 0 {
		t.Error("ResolveUpdate should receive the user's accounts so it can re-target by name, got none")
	}
}

func TestProceedToUpdateConfirm_UnresolvedSendsNoDBCall(t *testing.T) {
	orch := &fakeOrchestrator{updateResult: orchestrator.UpdateResult{Resolved: false}}
	actions := &fakeActionsRepo{}
	store := &fakeConvStore{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow.NewAskUserFlow())
	svc := &fakeServices{orch: orch, engine: engine, actions: actions, accounts: &fakeAccountRepoFull{}}

	if err := proceedToUpdateConfirm(context.Background(), svc, &messenger.FakeChat{}, 1, "che no sé", "", nil, nil, ChangeAsk{}); err != nil {
		t.Fatalf("proceedToUpdateConfirm: %v", err)
	}
	if store.flowName == flow.MovementUpdateConfirmFlowName {
		t.Error("an unresolved result should never start the confirm flow")
	}
}

func TestUpdate_UnresolvedChangeAsksWhatToChange(t *testing.T) {
	actions := &fakeActionsRepo{}
	engine := conversation.NewEngine(&fakeConvStore{}, func(string) string { return "algo" })
	engine.Register(flow.NewAskUserFlow())
	svc := &fakeServices{
		engine: engine, actions: actions,
		orch:      &fakeOrchestrator{updateResult: orchestrator.UpdateResult{Resolved: false}},
		movements: &fakeMovementRepoFull{},
		accounts:  &fakeAccountRepoFull{},
	}

	err := proceedToUpdateConfirm(context.Background(), svc, &messenger.FakeChat{}, 1,
		"estaba mal", "", []string{"10"}, []movement.MovementRow{{Amount: "3000", Currency: "ARS", Description: "café"}}, ChangeAsk{})
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
	if len(qs[0].Options) != len(changeFieldOptions()) {
		t.Errorf("faltan los botones de campo: %+v", qs[0].Options)
	}
	for _, opt := range qs[0].Options {
		if strings.Contains(opt, "monto") {
			t.Errorf("el monto no va como botón, se escribe: %q", opt)
		}
	}
	var payload agentPayload
	if err := json.Unmarshal(actions.rows[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Chosen != 0 || len(payload.Candidates) != 1 || payload.Candidates[0].OldIDs[0] != "10" {
		t.Errorf("el candidato resuelto no viajó: %+v", payload)
	}
}

func TestMsgAskWhatToChange_AsksForTheValueNotTheField(t *testing.T) {
	got := askWhatToChange([]movement.MovementRow{{Amount: "1800", Description: "Cafe"}})

	if !strings.Contains(got, "Cuánto era") {
		t.Errorf("no pide el valor nuevo: %q", got)
	}
	if !strings.Contains(got, "tocá abajo") {
		t.Errorf("no ofrece los botones para los otros campos: %q", got)
	}
	if strings.Contains(got, "el monto, la categoría") {
		t.Errorf("volvió la lista de campos en el texto, que se lee como menú: %q", got)
	}
	if !strings.Contains(got, "1800") {
		t.Errorf("no nombra el movimiento: %q", got)
	}
}

func TestUpdate_NoOpCorrectionAsksInsteadOfConfirming(t *testing.T) {
	before := []movement.MovementRow{{Type: "expense", Amount: "1800", Currency: "ARS",
		Category: "Ocio y salidas", Subcategory: "Salir a comer", Date: "2026-08-01", Description: "Cafe"}}
	actions := &fakeActionsRepo{}
	store := &fakeConvStore{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow.NewAskUserFlow())
	engine.Register(flow.NewMovementUpdateConfirmFlow())
	svc := &fakeServices{
		engine: engine, actions: actions, movements: &fakeMovementRepoFull{},
		accounts:      &fakeAccountRepoFull{},
		subcategories: &fakeSubcategoryRepoFull{},
		orch: &fakeOrchestrator{updateResult: orchestrator.UpdateResult{
			Resolved:  true,
			Movements: []orchestrator.MovementDraft{rowToDraft(before[0])},
		}},
	}

	if err := proceedToUpdateConfirm(context.Background(), svc, &messenger.FakeChat{}, 1,
		"El café estaba mal", "", []string{"127"}, before, ChangeAsk{}); err != nil {
		t.Fatal(err)
	}

	if store.flowName == flow.MovementUpdateConfirmFlowName {
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

func TestUpdate_PickedFieldAsksForTheValueWithoutCallingTheModel(t *testing.T) {
	actions := &fakeActionsRepo{}
	engine := conversation.NewEngine(&fakeConvStore{}, func(string) string { return "algo" })
	engine.Register(flow.NewAskUserFlow())
	svc := &fakeServices{engine: engine, actions: actions, accounts: &fakeAccountRepoFull{}}

	err := proceedToUpdateConfirm(context.Background(), svc, &messenger.FakeChat{}, 1,
		"El café estaba mal La categoría", "", []string{"127"},
		[]movement.MovementRow{{Amount: "1800", Description: "Cafe"}},
		ChangeAsk{pickedField: true})
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
	if qs[0].Prompt != "Dale. ¿Y cuál es el valor nuevo?" {
		t.Errorf("la segunda vuelta pregunta el valor, no el campo: %q", qs[0].Prompt)
	}
	if len(qs[0].Options) != 0 {
		t.Errorf("la segunda vuelta no lleva botones: %+v", qs[0].Options)
	}
}

func TestUpdate_AmountAnswerSkipsTheModel(t *testing.T) {
	before := []movement.MovementRow{{Type: "expense", Amount: "1800", Currency: "ARS", AccountID: "46",
		Category: "Ocio y salidas", Subcategory: "Salir a comer", Description: "Cafe"}}
	store := &fakeConvStore{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow.NewMovementUpdateConfirmFlow())
	svc := &fakeServices{engine: engine, accounts: &fakeAccountRepoFull{}, subcategories: &fakeSubcategoryRepoFull{}}

	err := proceedToUpdateConfirm(context.Background(), svc, &messenger.FakeChat{}, 1,
		"el café estaba mal 2000", "", []string{"127"}, before,
		ChangeAsk{gaveValue: true, answer: "2000"})
	if err != nil {
		t.Fatal(err)
	}

	if store.flowName != flow.MovementUpdateConfirmFlowName {
		t.Fatalf("tenía que abrir el confirm, abrió %q", store.flowName)
	}
	after := movement.DecodeMovementRows(store.data)
	if len(after) != 1 || after[0].Amount != "2000" {
		t.Fatalf("el monto nuevo no llegó: %+v", after)
	}
	if after[0].AccountID != "46" || after[0].Subcategory != "Salir a comer" {
		t.Errorf("el atajo tocó algo que no era el monto: %+v", after[0])
	}
}

func TestAmountOnlyCorrection_FallsBackWhenItIsNotJustTheAmount(t *testing.T) {
	one := []movement.MovementRow{{Amount: "1800", Currency: "ARS"}}
	two := []movement.MovementRow{{Amount: "1800"}, {Amount: "1800"}}

	for name, tc := range map[string]struct {
		rows []movement.MovementRow
		ask  ChangeAsk
	}{
		"tocó un botón, el campo no es el monto": {one, ChangeAsk{gaveValue: true, pickedField: true, answer: "2000"}},
		"no es un número":                        {one, ChangeAsk{gaveValue: true, answer: "era en Delivery"}},
		"transferencia de dos piernas":           {two, ChangeAsk{gaveValue: true, answer: "2000"}},
		"monto cero (es un borrado)":             {one, ChangeAsk{gaveValue: true, answer: "0"}},
		"todavía no contestó nada":               {one, ChangeAsk{}},
	} {
		if _, ok := amountOnlyCorrection(tc.rows, tc.ask); ok {
			t.Errorf("%s: no puede tomar el atajo", name)
		}
	}
}

func TestUpdate_NoOpAfterAskingGivesUp(t *testing.T) {
	before := []movement.MovementRow{{Type: "expense", Amount: "1800", Currency: "ARS", Description: "Cafe"}}
	actions := &fakeActionsRepo{}
	metrics := &fakeMetricRepo{}
	svc := &fakeServices{
		engine:  conversation.NewEngine(&fakeConvStore{}, func(string) string { return "algo" }),
		actions: actions, metrics: metrics, movements: &fakeMovementRepoFull{},
		accounts:      &fakeAccountRepoFull{},
		subcategories: &fakeSubcategoryRepoFull{},
		orch: &fakeOrchestrator{updateResult: orchestrator.UpdateResult{
			Resolved: true, Movements: []orchestrator.MovementDraft{rowToDraft(before[0])},
		}},
	}

	if err := proceedToUpdateConfirm(context.Background(), svc, &messenger.FakeChat{}, 1,
		"El café estaba mal no sé", "", []string{"127"}, before, ChangeAsk{gaveValue: true}); err != nil {
		t.Fatal(err)
	}

	if len(actions.rows) != 0 {
		t.Errorf("ya se preguntó una vez: no puede volver a parkear la misma pregunta")
	}
	if len(metrics.resolved) != 1 || metrics.resolved[0] != outcomeLoopDidNothing {
		t.Errorf("el evento tiene que cerrar como fallo, got %v", metrics.resolved)
	}
}

func TestCorrectionIsNoOp_DetectsARealChange(t *testing.T) {
	before := []movement.MovementRow{{Type: "expense", Amount: "1800", Currency: "ARS",
		Category: "Ocio y salidas", Subcategory: "Salir a comer", Description: "Cafe"}}

	same := []orchestrator.MovementDraft{{Amount: "1800.00"}}
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

func TestApplyAnswers_ChangeAnswerIsAppended(t *testing.T) {
	action := &pendingaction.PendingAction{Payload: mustJSON(t, agentPayload{
		Change: "el café estaba mal", Candidates: []flow.CandidateGroup{{OldIDs: []string{"10"}}}, Chosen: 0,
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
	store := &fakeConvStore{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow.NewMovementUpdateConfirmFlow())
	svc := &fakeServices{engine: engine, subcategories: &fakeSubcategoryRepoFull{}, accounts: &fakeAccountRepoFull{}}

	result := orchestrator.UpdateResult{Resolved: true, Movements: []orchestrator.MovementDraft{
		{Type: "expense", Amount: "3500", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", Date: "2026-07-02"},
	}}
	if err := seedAndStartUpdateConfirm(context.Background(), svc, &messenger.FakeChat{}, 1, "eran 3500", []string{"7"}, nil, result); err != nil {
		t.Fatalf("seedAndStartUpdateConfirm: %v", err)
	}
	if store.flowName != flow.MovementUpdateConfirmFlowName {
		t.Errorf("started flow = %q, want %q", store.flowName, flow.MovementUpdateConfirmFlowName)
	}
}

func TestCorrectionIsDeletion(t *testing.T) {
	cases := []struct {
		name    string
		rows    []movement.MovementRow
		message string
		want    bool
	}{
		{"empty set is not a deletion", nil, "salió 0", false},
		{"single zero row deletes", []movement.MovementRow{{Amount: "0"}}, "en realidad fue 0", true},
		{"zero with decimals deletes", []movement.MovementRow{{Amount: "0.00"}}, "0 pesos", true},
		{"non-zero is a real correction", []movement.MovementRow{{Amount: "600"}}, "eran 600", false},
		{"mixed zero and non-zero is not a deletion", []movement.MovementRow{{Amount: "0"}, {Amount: "500"}}, "poné 0 y 500", false},
		{"unparseable amount is not a deletion", []movement.MovementRow{{Amount: ""}}, "gratis", false},

		{"todo cero SIN monto en el mensaje NO borra", []movement.MovementRow{{Amount: "0"}, {Amount: "0"}},
			"Editá los movimientos de lote de hoy", false},
		{"tampoco con un solo movimiento", []movement.MovementRow{{Amount: "0"}},
			"editá el café", false},

		{"me lo regalaron", []movement.MovementRow{{Amount: "0"}}, "me regalaron el helado", true},
		{"al final fue gratis", []movement.MovementRow{{Amount: "0"}}, "al final fue gratis", true},
		{"no me cobraron nada", []movement.MovementRow{{Amount: "0"}}, "no me cobraron nada", true},
		{"me invitaron, con acento de por medio", []movement.MovementRow{{Amount: "0"}}, "me invitó él", true},
	}
	for _, tc := range cases {
		if got := correctionIsDeletion(tc.rows, tc.message); got != tc.want {
			t.Errorf("%s: correctionIsDeletion(%q) = %v, want %v", tc.name, tc.message, got, tc.want)
		}
	}
}

func TestApplyAnswers_ButtonPlusValueBuildsTheChange(t *testing.T) {
	action := &pendingaction.PendingAction{Payload: mustJSON(t, agentPayload{
		Change: "editá la panadería", Candidates: []flow.CandidateGroup{{OldIDs: []string{"10"}}}, Chosen: 0,
	})}
	options := changeFieldOptions()
	picked, _ := applyAnswers(action, []pendingaction.OpenQuestion{
		{Key: questionKeyChange, Answer: labelChangeCategory, Options: options},
	})
	if picked.PickedField != string(fieldCategory) {
		t.Fatalf("picked_field = %q, want %q", picked.PickedField, fieldCategory)
	}
	if len(picked.Changes) != 0 {
		t.Errorf("el botón solo no alcanza: todavía falta el valor, y hay %d cambios", len(picked.Changes))
	}

	action.Payload = mustJSON(t, picked)
	resolvedPayload, resolved := applyAnswers(action, []pendingaction.OpenQuestion{
		{Key: questionKeyChange, Answer: "Vivienda", Options: nil},
	})
	if !resolved {
		t.Fatal("contestar el valor tiene que resolver la acción")
	}
	if len(resolvedPayload.Changes) != 1 {
		t.Fatalf("cambios = %d, want 1: el campo salió del botón y el valor del texto", len(resolvedPayload.Changes))
	}
	got := resolvedPayload.Changes[0]
	if got.Field != fieldCategory || got.Op != opSet || got.Value != "Vivienda" {
		t.Errorf("cambio = %+v, want {category set Vivienda}", got)
	}
}

func TestApplyAnswers_ValueWithoutAButtonBuildsNothing(t *testing.T) {
	action := &pendingaction.PendingAction{Payload: mustJSON(t, agentPayload{
		Change: "el café estaba mal", Candidates: []flow.CandidateGroup{{OldIDs: []string{"10"}}}, Chosen: 0,
	})}
	payload, _ := applyAnswers(action, []pendingaction.OpenQuestion{
		{Key: questionKeyChange, Answer: "2000"},
	})
	if len(payload.Changes) != 0 {
		t.Errorf("sin campo elegido no se arma un cambio, hay %d", len(payload.Changes))
	}
	if !payload.GaveChangeValue {
		t.Error("tenía que quedar marcado que dio un valor")
	}
}
