package messaging

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
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)

// candidateMovement arma un movimiento cargado recién, con descripción — los dos
// requisitos para que resolveCandidates lo devuelva por match textual.
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
	return newAgentExecutor(context.Background(), &controller{movements: &fakeMovementRepoFull{similar: movements}}, 1, userText, nil)
}

// taxonomyForTest es la taxonomía mínima que buildCreateSeed necesita para NO
// marcar gap de categoría: el par tiene que existir de verdad.
func taxonomyForTest() []orchestrator.TaxonomyEntry {
	return []orchestrator.TaxonomyEntry{{Category: "Alimentación", Subcategory: "Supermercado"}}
}

// accountsWithDefault da una cuenta ARS por defecto, que es lo que evita el
// camino de needsFirstAccount.
func accountsWithDefault() *fakeAccountRepoFull {
	acc := &account.Account{Model: gorm.Model{ID: 1}, Name: "Mercado Pago", Currency: currency.ARS, IsDefault: true}
	return &fakeAccountRepoFull{
		byCurrency: map[currency.Currency]*account.Account{currency.ARS: acc},
		byUserID:   []account.Account{*acc},
		byID:       map[uint64]*account.Account{1: acc},
	}
}

// movementsWithBalance fija el saldo de la cuenta 1, para poder disparar (o no)
// el gate de fondos insuficientes.
func movementsWithBalance(balance string) *fakeMovementRepoFull {
	return &fakeMovementRepoFull{balances: map[uint64]string{1: balance}}
}

// subcategoriesForTest tiene el mismo par que taxonomyForTest. Los dos tienen
// que coincidir: la taxonomía decide si hay gap, y el repo decide si la
// inserción encuentra la subcategoría — desalinearlos da un gap fantasma o un
// insert que falla al final.
func subcategoriesForTest() *fakeSubcategoryRepoFull {
	sub := subcategory.Subcategory{Model: gorm.Model{ID: 1}, Category: "Alimentación", Subcategory: "Supermercado"}
	return &fakeSubcategoryRepoFull{
		byCategoryAndSub: map[string]*subcategory.Subcategory{"Alimentación|Supermercado": &sub},
		all:              []subcategory.Subcategory{sub},
	}
}

// newCreateExecutor arma el ejecutor con lo mínimo que necesita un CREATE:
// cuentas, saldo y taxonomía.
// newCreateExecutor arma el executor con el clasificador scripteado.
//
// Desde que la clasificación salió del loop, record_movements NO trae el par:
// lo pone ClassifyCategories. Un test que no lo programe recibe PENDING_REVIEW
// en todas las filas y termina midiendo el gap-fill en vez de lo suyo.
func newCreateExecutor(t *testing.T, balance, userText string, pairs ...orchestrator.Pair) *agentExecutor {
	t.Helper()
	if pairs == nil {
		pairs = []orchestrator.Pair{{Category: "Alimentación", Subcategory: "Supermercado"}}
	}
	c := &controller{
		movements:     movementsWithBalance(balance),
		accounts:      accountsWithDefault(),
		subcategories: subcategoriesForTest(),
		orchestrator:  &fakeFullOrchestrator{classifyPairs: pairs},
	}
	return newAgentExecutor(context.Background(), c, 1, userText, taxonomyForTest())
}

// TestAgentExecutor_CleanCreateInsertsAndOwnsTheTurn: el camino sin fricción.
// Cierra el turno con ErrAgentTurnDone porque el recibo lo escribe la app —
// pedirle al modelo que narre "listo" cuesta el prompt entero otra vez, y
// medido en la etapa 2 el modelo NUNCA narra junto a los tool_calls.
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
	// El recibo lo pone la app: es lo que hace que el turno no necesite narración.
	if e.reply == "" {
		t.Error("falta el recibo; sin él el usuario no ve nada")
	}
}

// executeDone corre una tool que TIENE que cerrar el turno.
//
// Que devuelva ErrAgentTurnDone no es tolerancia, es LA aserción: si dejara de
// devolverlo, el loop volvería a gastar una segunda vuelta —el prompt entero de
// nuevo, ~5k tokens— narrando algo que la app ya sabe decir. Contra el TPM de
// 8.000 eso es un 429, y la corrección del usuario terminaba en la cola en vez
// de en el gate.
func executeDone(t *testing.T, e *agentExecutor, tool, args string) string {
	t.Helper()
	out, err := e.execute(tool, json.RawMessage(args))
	if !errors.Is(err, orchestrator.ErrAgentTurnDone) {
		t.Fatalf("%s: want ErrAgentTurnDone, got %v", tool, err)
	}
	return out
}

// TestAgentExecutor_CorrectResolvesInOneRound es el punto del rediseño: el
// modelo pide corregir y listo. No hay vuelta de búsqueda — el candidato lo
// resuelve la app — porque una segunda vuelta arrastra ~5k tokens de prompt y no
// entra en el TPM.
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
	// Los oldIDs salen de la búsqueda de la app, nunca del modelo.
	if got := a.Payload.Candidates[0].OldIDs; len(got) != 1 || got[0] != "10" {
		t.Errorf("los oldIDs no salieron de resolveCandidates: %+v", got)
	}
}

// TestAgentExecutor_InsufficientFundsParksTheGate: el gate de saldo negativo no
// se saltea desde el loop. Nada se inserta hasta que el usuario decide.
func TestAgentExecutor_InsufficientFundsParksTheGate(t *testing.T) {
	e := newCreateExecutor(t, "100", "gasté 50000 en el super")

	args := `{"movements":[{"type":"expense","amount":"50000","currency":"ARS",
		"category":"Alimentación","subcategory":"Supermercado","date":"2026-08-01",
		"description":"super","payment_method":"transfer"}]}`
	executeDone(t, e, orchestrator.ToolRecordMovements, args)

	if e.wrote {
		t.Error("wrote quedó en true sin haber insertado: bloquearía la cola del 429 sin razón")
	}
	if len(e.parked) != 1 || e.parked[0].Payload.Seed[keyGatePrompt] == nil {
		t.Fatalf("tenía que parkear el gate con su copy: %+v", e.parked)
	}
	// Y retoma como CREATE: es lo que hace que el drenaje sepa a qué flujo ir.
	if e.parked[0].Tool != orchestrator.ToolRecordMovements {
		t.Errorf("el gate retoma un CREATE, no otra cosa: %q", e.parked[0].Tool)
	}
}

// TestAgentExecutor_SearchesWithTheUsersTextNotTheModelParaphrase: el matcheo
// por tokens y el plegado de acentos están tuneados contra lo que escribe el
// usuario. La paráfrasis del modelo puede perder justo la palabra que matchea.
func TestAgentExecutor_SearchesWithTheUsersTextNotTheModelParaphrase(t *testing.T) {
	e := newExecutorWith(t, "la panaderia era 2000", candidateMovement(10, nil, "compra en panadería", 3000))

	// El change no nombra la panadería; sólo el texto original lo hace.
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
	// Y queda anotado, para que la métrica diga no_candidates y no un fracaso genérico.
	if !e.noCandidates {
		t.Error("falta la marca de que no había candidatos")
	}
	// La copy la pone la app: hacérsela narrar al modelo cuesta la vuelta entera.
	if e.reply != msgNoCandidatesFound {
		t.Errorf("want %q, got %q", msgNoCandidatesFound, e.reply)
	}
}

// TestAgentExecutor_BadArgsStillPark: un argumento ilegible no puede tumbar el
// turno. El pedido se entiende por el nombre de la tool y el candidato sale del
// texto del usuario, así que no hay nada que perder.
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
		{orchestrator.ToolAskRewrite, msgAskRewrite},
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

// El par estructural es un DEFAULT: si el clasificador dice algo útil, gana el
// clasificador.
//
// Medido en vivo el 2026-08-12: "Suscribi 3100000 a FCI" quedó en
// `Sistema | Transferencia` porque había un `return` que le impedía al
// clasificador ver el movimiento. Una suscripción de FCI entre dos cuentas
// propias en la misma moneda tiene la forma EXACTA de una transferencia —2 patas
// que suman cero— y no es una transferencia: sólo el mensaje las distingue.
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

// Y cuando el clasificador NO dice nada útil, el default entra: sin él un
// transfer sin clasificar caería en PENDING_REVIEW y abriría el picker por algo
// que la forma ya contesta.
func TestClassify_StructuralDefaultFillsWhatTheClassifierLeavesEmpty(t *testing.T) {
	movs := []orchestrator.MovementDraft{
		{Type: "transfer", Amount: "-50000", Currency: "ARS"},
		{Type: "transfer", Amount: "50000", Currency: "ARS"},
	}
	// El clasificador falló (429, timeout): devuelve PENDING_REVIEW.
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

// executorWithPairs arma un ejecutor cuyo clasificador devuelve los pares dados.
func executorWithPairs(t *testing.T, userText string, pairs []orchestrator.Pair) *agentExecutor {
	t.Helper()
	c := &controller{
		movements:     movementsWithBalance("1000000"),
		accounts:      accountsWithDefault(),
		subcategories: subcategoriesForTest(),
		orchestrator:  &fakeFullOrchestrator{classifyPairs: pairs},
	}
	return newAgentExecutor(context.Background(), c, 1, userText, taxonomyForTest())
}
