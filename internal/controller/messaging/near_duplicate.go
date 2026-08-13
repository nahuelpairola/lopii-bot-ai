package messaging

import (
	"strings"
	"time"

	"lopiibot.com/internal/movement"
)

// findNearDuplicate busca, entre los movimientos previos del usuario, uno que
// el recién insertado parezca estar corrigiendo.
//
// Es la capa 2 de la corrección, y la que carga el peso: NO involucra al modelo.
// No le importa POR QUÉ el modelo se equivocó —un `When` mal escrito, falta de
// contexto, un fraseo nuevo, una regresión futura—: la forma del daño es la
// misma y esto caza la forma.
//
// Reglas, todas necesarias:
//   - mismo usuario, misma moneda, misma cuenta y MISMO TIPO
//   - el previo se creó dentro de justCreatedWindow
//   - NO son dos patas del mismo transfer (mismo transaction_id)
//   - NO los insertó el mismo mensaje: "dos cafés de 1070" es un mensaje con dos
//     filas de monto idéntico en la misma cuenta, y sin esta regla el lote se
//     marcaría a sí mismo
//   - y comparten un token normalizado, o el |monto| es idéntico
//
// Devuelve UN candidato, no una lista: esto termina en un botón arriba del
// recibo, no en un picker. Gana el match por token sobre el match por monto, y
// entre dos del mismo tipo gana el más reciente.
func findNearDuplicate(inserted movement.Movement, sameTurnIDs []uint, priors []movement.Movement) *movement.Movement {
	excluded := make(map[uint]bool, len(sameTurnIDs))
	for _, id := range sameTurnIDs {
		excluded[id] = true
	}

	var byToken, byAmount *movement.Movement
	for i := range priors {
		p := &priors[i]
		if !nearDuplicateCandidate(inserted, *p, excluded) {
			continue
		}
		if sharesDescriptionToken(inserted, *p) {
			if byToken == nil || p.CreatedAt.After(byToken.CreatedAt) {
				byToken = p
			}
			continue
		}
		if byAmount == nil || p.CreatedAt.After(byAmount.CreatedAt) {
			byAmount = p
		}
	}
	if byToken != nil {
		return byToken
	}
	return byAmount
}

// nearDuplicateCandidate aplica todo lo que NO es la prueba textual.
func nearDuplicateCandidate(m, p movement.Movement, excluded map[uint]bool) bool {
	if p.ID == m.ID || excluded[p.ID] {
		return false
	}
	if p.DeletedAt.Valid {
		return false
	}
	if p.UserID != m.UserID || p.Currency != m.Currency {
		return false
	}
	// Distinto tipo, distinto hecho. Un reintegro tiene TODO en común con el gasto
	// que reintegra —cuenta, moneda, |monto|, token— salvo el signo, así que sin
	// esto el gate los marca, y las tres opciones del recibo escriben: fusionar
	// −5.000 con +5.000 guarda una fila de monto CERO, y reemplazar le copia el
	// +5.000 a una fila typada `expense`, o sea un gasto que suma plata. El guard
	// rechaza las dos, pero el gate no pasa por el guard.
	//
	// Con esta línea el par que llega a applyNearDuplicateChoice comparte tipo, y
	// por lo tanto signo: la suma no puede dar cero y el reemplazo no puede
	// invertir el signo. Es lo que hace segura la escritura de allá.
	if p.Type != m.Type {
		return false
	}
	if !sameAccount(m.AccountID, p.AccountID) {
		return false
	}
	// Una pata de transferencia NUNCA entra al gate, ni como recién insertada ni
	// como candidata. Da igual si comparten transaction_id o no.
	//
	// Antes la condición era `mismo transaction_id`, o sea que sólo se excluía a
	// las dos patas de UNA transferencia entre sí. Las patas de transferencias
	// DISTINTAS pasaban, y fusionarlas rompe los dos grupos: medido en vivo el
	// 2026-08-12 con dos suscripciones a FCI, quedó un grupo de UNA sola pata
	// (+500.000) y otro que no balanceaba (−810.000 + 310.000). Los saldos por
	// cuenta seguían bien —netean— pero la estructura quedó corrupta, y sobre eso
	// después opera cualquier corrección.
	//
	// Fusionar UNA pata es siempre incorrecto: una transferencia son dos patas que
	// se sostienen entre sí y se corrigen como grupo (ver applyChangesToSet, que
	// aplica el monto a las DOS preservando signos).
	if m.TransactionID != nil || p.TransactionID != nil {
		return false
	}
	if m.CreatedAt.Sub(p.CreatedAt) > justCreatedWindow || p.CreatedAt.After(m.CreatedAt) {
		return false
	}
	return sharesDescriptionToken(m, p) || sameAbsAmount(m, p)
}

func sameAccount(a, b *uint64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameAbsAmount(m, p movement.Movement) bool {
	return !m.Amount.IsZero() && m.Amount.Abs().Equal(p.Amount.Abs())
}

// sharesDescriptionToken reutiliza el mismo plegado de acentos y el mismo
// minMatchTokenLen que la resolución de referencias. No se escribe un segundo
// matcher: el de "panaderia" vs "panadería" ya está resuelto ahí.
func sharesDescriptionToken(m, p movement.Movement) bool {
	if m.Description == nil || p.Description == nil {
		return false
	}
	lowered := foldAccents(strings.ToLower(*p.Description))
	return tokenAppearsInString(*m.Description, lowered)
}

// nearDuplicateWindowStart es desde cuándo buscar previos, para el query.
func nearDuplicateWindowStart(at time.Time) time.Time { return at.Add(-justCreatedWindow) }
