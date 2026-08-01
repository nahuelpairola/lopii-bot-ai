package messaging

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
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
	return newAgentExecutor(&controller{movements: &fakeMovementRepoFull{similar: movements}}, 1, userText)
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
		orchestrator.ToolRecordMovements,
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
