package messaging

import "sync"

// userLocks serializa los updates de UN usuario. Los de usuarios distintos
// siguen corriendo en paralelo — que es todo el punto: el bot atiende a muchos.
//
// POR QUÉ EXISTE. Todo el estado del bot está cuñado por usuario:
// `conversation_states` tiene el user_id de PRIMARY KEY (una fila por usuario),
// `pending_actions` se drena de a uno por usuario, y `intent_events.Resolve`
// cierra "el pendiente más reciente del usuario". Las tres cosas asumen que el
// usuario tiene UN mensaje en vuelo a la vez.
//
// Esa invariante la sostenía, sin proponérselo, el 429: dos mensajes seguidos no
// entraban los dos en el TPM, así que el segundo se encolaba y el drenaje los
// procesaba en fila. La cola no se diseñó como candado pero funcionaba de
// candado. Al agregar la cadena de modelos de respaldo (2026-08-12) el segundo
// mensaje dejó de rebotar, y los dos pasaron a correr a la vez.
//
// Se vio en la primera prueba: dos mensajes en el mismo segundo dejaron el
// intent_event de uno con el movimiento del otro. Los movimientos quedaron
// bien —esos no dependen de la invariante— pero el próximo caso sí importa: dos
// altas que abran gap de categoría se pisan la fila de `conversation_states`, y
// la primera se pierde sin que nada avise.
//
// ponytail: el candado es EN MEMORIA, o sea que vale para UN proceso. Con dos
// instancias, dos mensajes del mismo usuario pueden caer en procesos distintos y
// este mutex no los ve. Ahí la upgrade es `pg_advisory_xact_lock(user_id)`, que
// es una línea — pero recién cuando haya dos instancias.
//
// Tampoco se borran entradas: queda un mutex por usuario que haya escrito
// alguna vez. Son ~48 bytes cada uno; con miles de usuarios sigue siendo ruido.
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
