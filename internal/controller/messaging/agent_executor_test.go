package messaging

import (
	"encoding/json"
	"strings"
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

func newExecutorWith(t *testing.T, movements ...movement.Movement) *agentExecutor {
	t.Helper()
	return newAgentExecutor(&controller{movements: &fakeMovementRepoFull{similar: movements}}, 1)
}

func TestAgentExecutor_FindReturnsIdsTheModelCanPassBack(t *testing.T) {
	e := newExecutorWith(t, candidateMovement(10, nil, "compra en panadería", 3000))

	out, err := e.execute(orchestrator.ToolFindMovementsToCorrect, json.RawMessage(`{"text":"la panaderia era 2k"}`))
	if err != nil {
		t.Fatal(err)
	}
	// El handle es propio ("1"), no el transaction_id: un movimiento sin agrupar
	// lo tiene vacío y todos colisionarían.
	if !strings.Contains(out, "\n1 | ") {
		t.Fatalf("el id que el modelo tiene que devolver no está en la respuesta:\n%s", out)
	}
	if !strings.Contains(out, "panadería") {
		t.Errorf("falta la etiqueta legible:\n%s", out)
	}
	if _, ok := e.groups["1"]; !ok {
		t.Errorf("el grupo no quedó cacheado: %+v", e.groups)
	}
	if len(e.parked) != 0 {
		t.Errorf("buscar no parkea nada: %+v", e.parked)
	}
}

func TestAgentExecutor_FindWithNoMatchesSaysSo(t *testing.T) {
	e := newExecutorWith(t)
	out, err := e.execute(orchestrator.ToolFindMovementsToCorrect, json.RawMessage(`{"text":"algo que no existe"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out != resultNoCandidates {
		t.Errorf("want %q, got %q", resultNoCandidates, out)
	}
}

func TestAgentExecutor_CorrectParksTheCachedGroupWithNoQuestion(t *testing.T) {
	e := newExecutorWith(t, candidateMovement(10, nil, "compra en panadería", 3000))
	if _, err := e.execute(orchestrator.ToolFindMovementsToCorrect, json.RawMessage(`{"text":"panaderia"}`)); err != nil {
		t.Fatal(err)
	}

	out, err := e.execute(orchestrator.ToolCorrectMovement,
		json.RawMessage(`{"transaction_id":"1","change":"eran 2000"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out != resultParked {
		t.Errorf("want %q, got %q", resultParked, out)
	}
	if len(e.parked) != 1 {
		t.Fatalf("want 1 parked action, got %d", len(e.parked))
	}
	a := e.parked[0]
	if a.Tool != orchestrator.ToolCorrectMovement {
		t.Errorf("tool equivocada: %q", a.Tool)
	}
	if len(a.Questions) != 0 {
		t.Errorf("con el candidato resuelto no se pregunta nada: %+v", a.Questions)
	}
	if a.Payload.Chosen != 0 || len(a.Payload.Candidates) != 1 {
		t.Fatalf("el candidato no quedó elegido: %+v", a.Payload)
	}
	if a.Payload.Change != "eran 2000" {
		t.Errorf("se perdió el cambio pedido: %q", a.Payload.Change)
	}
	// oldIDs y las filas salen del cache, nunca de lo que dictó el modelo.
	if got := a.Payload.Candidates[0].OldIDs; len(got) != 1 || got[0] != "10" {
		t.Errorf("los oldIDs no salieron del cache: %+v", got)
	}
}

// TestAgentExecutor_UnknownTransactionIDNeverActsOnIt es la garantía de §4.11: un
// id inventado no puede terminar corrigiendo un movimiento cualquiera.
func TestAgentExecutor_UnknownTransactionIDNeverActsOnIt(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	e := newExecutorWith(t,
		candidateMovement(10, &a, "compra en panadería", 3000),
		candidateMovement(11, &b, "panadería del barrio", 5000),
	)
	if _, err := e.execute(orchestrator.ToolFindMovementsToCorrect, json.RawMessage(`{"text":"panaderia"}`)); err != nil {
		t.Fatal(err)
	}

	out, err := e.execute(orchestrator.ToolCorrectMovement,
		json.RawMessage(`{"transaction_id":"me-lo-invente","change":"eran 2000"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out != resultParked {
		t.Fatalf("want %q, got %q", resultParked, out)
	}
	action := e.parked[0]
	if action.Payload.Chosen != -1 {
		t.Errorf("un id desconocido NO puede quedar elegido: %+v", action.Payload)
	}
	if len(action.Questions) != 1 || action.Questions[0].Key != questionKeyCandidate {
		t.Fatalf("tiene que caer a preguntar cuál: %+v", action.Questions)
	}
	if len(action.Questions[0].Options) != 2 {
		t.Errorf("las opciones son los candidatos cacheados: %+v", action.Questions[0].Options)
	}
}

func TestAgentExecutor_ParksWithoutFindingFirst(t *testing.T) {
	// El modelo puede saltearse find_movements_to_correct. La app resuelve igual.
	e := newExecutorWith(t, candidateMovement(10, nil, "compra en panadería", 3000))

	out, err := e.execute(orchestrator.ToolCorrectMovement,
		json.RawMessage(`{"change":"la panaderia era 2000"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out != resultParked {
		t.Fatalf("want %q, got %q", resultParked, out)
	}
	if e.parked[0].Payload.Chosen != 0 {
		t.Errorf("con un solo candidato se elige, no se pregunta: %+v", e.parked[0])
	}
}

func TestAgentExecutor_NothingToCorrectDoesNotPark(t *testing.T) {
	e := newExecutorWith(t)
	out, err := e.execute(orchestrator.ToolCorrectMovement, json.RawMessage(`{"change":"corregí el asado"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out != resultNoCandidates {
		t.Errorf("want %q, got %q", resultNoCandidates, out)
	}
	if len(e.parked) != 0 {
		t.Errorf("sin candidatos no se parkea nada: %+v", e.parked)
	}
}

func TestAgentExecutor_DeleteParksToo(t *testing.T) {
	e := newExecutorWith(t, candidateMovement(10, nil, "compra en panadería", 3000))
	if _, err := e.execute(orchestrator.ToolFindMovementsToCorrect, json.RawMessage(`{"text":"panaderia"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.execute(orchestrator.ToolDeleteMovements, json.RawMessage(`{"transaction_id":"1"}`)); err != nil {
		t.Fatal(err)
	}
	if len(e.parked) != 1 || e.parked[0].Tool != orchestrator.ToolDeleteMovements {
		t.Fatalf("delete no parkeó: %+v", e.parked)
	}
	if e.parked[0].Payload.Chosen != 0 {
		t.Errorf("el candidato cacheado tiene que quedar elegido: %+v", e.parked[0].Payload)
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
		e := newExecutorWith(t)
		if _, err := e.execute(tc.tool, json.RawMessage(`{}`)); err != nil {
			t.Fatal(err)
		}
		if e.reply != tc.want {
			t.Errorf("%s: want reply %q, got %q", tc.tool, tc.want, e.reply)
		}
		if len(e.parked) != 0 {
			t.Errorf("%s: se resuelve en el turno, no se parkea: %+v", tc.tool, e.parked)
		}
	}
}

func TestAgentExecutor_UnwiredToolsSayNotAvailable(t *testing.T) {
	e := newExecutorWith(t)
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
