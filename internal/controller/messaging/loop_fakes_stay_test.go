package messaging

// Copias del lado messaging de los fakes del loop. Los de agent viven en
// internal/agent/fake_services_test.go; messaging no puede importar los archivos
// _test de agent (ciclo de imports), así que se duplican acá. Son test-double
// puros, no lógica de negocio — el trade-off está documentado en el ledger C1-1.

import (
	"encoding/json"
	"errors"
	"time"

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

// fakeStoreForController es un store compatible con conversation.Engine — la
// satisfacción de interfaces en Go es estructural, así que este struct satisface
// la interfaz unexportada stateStore del motor puro por su set de métodos.
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

// errRunNotWired es lo que devuelven los fakes cuando el test no programó el
// loop. Un camino que llegue ahí sin quererlo migró antes de su etapa, y tiene que
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
