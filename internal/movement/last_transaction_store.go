package movement

import "sync"

// LastTransactionStore guarda, por usuario, el grupo de movimientos de
// la última transacción que creó o corrigió (todas las filas que
// comparten un transaction_id, no una sola fila). En memoria — no
// persiste entre reinicios del proceso. Eso es intencional: una entrada
// vacía o vieja solo cuesta una consulta extra de búsqueda en DB (ver
// reference_resolution.go), nunca produce una respuesta incorrecta.
type LastTransactionStore struct {
	mu   sync.RWMutex
	data map[uint64][]Movement
}

func NewLastTransactionStore() *LastTransactionStore {
	return &LastTransactionStore{data: make(map[uint64][]Movement)}
}

func (s *LastTransactionStore) Set(userID uint64, movements []Movement) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[userID] = movements
}

func (s *LastTransactionStore) Get(userID uint64) ([]Movement, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ms, ok := s.data[userID]
	return ms, ok
}

func (s *LastTransactionStore) Clear(userID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, userID)
}
