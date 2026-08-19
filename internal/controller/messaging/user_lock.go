package messaging

import "sync"

// userLocks serializa los updates de UN usuario. Los de usuarios distintos
// siguen corriendo en paralelo — que es todo el punto: el bot atiende a muchos.
//
// Todo el estado por usuario asume UN mensaje en vuelo: `conversation_states`
// (user_id de PRIMARY KEY), el drenaje de `pending_actions` (WIP=1) y
// `intent_events.Resolve` ("el pendiente más reciente"). Quién sostenía esa
// invariante antes y por qué dejó de hacerlo:
// docs/decisions.md § Groq quota, the 429 queue and rate limits.
//
// ponytail: candado EN MEMORIA, o sea UN proceso. Con dos instancias la upgrade
// es `pg_advisory_xact_lock(user_id)` — recién cuando haya dos instancias.
// Tampoco se borran entradas: ~48 bytes por usuario que haya escrito alguna vez.
type userLocks struct {
	m sync.Map // userID -> *sync.Mutex
}

// lock toma el candado del usuario y devuelve la función que lo suelta.
//
//	defer c.userLocks.lock(userID)()
//
// El unlock devuelto es el del mutex que se tomó, no una re-búsqueda en el mapa:
// así no puede soltar el de otro.
func (l *userLocks) lock(userID uint64) func() {
	actual, _ := l.m.LoadOrStore(userID, &sync.Mutex{})
	mu := actual.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}
