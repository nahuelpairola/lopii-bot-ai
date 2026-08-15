package messaging

import (
	"fmt"
	"strings"
	"time"

	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

// foldAccents y tokenAppearsInString viven en flow (movement_text.go), junto al
// foldAccents del matcher de nombres de cuenta; acá quedan los puentes que usa
// la resolución de referencias del borde.
func foldAccents(s string) string { return flow.FoldAccents(s) }

func startOfTodayArgentina() time.Time {
	now := time.Now().In(constants.ArgentinaZone)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, constants.ArgentinaZone)
}

const (
	dateAnchorMargin  = 24 * time.Hour
	fallbackRecentCap = 5
	// recencyLimit / recencyWindow acotan la ventana de "lo que tengo fresco".
	//
	// El límite REAL es por cantidad, no por tiempo: una ventana fija servía o no
	// según el ritmo de cada usuario. Quien carga 20 por día tenía 40 candidatos
	// para elegir; quien carga 3 por semana no llegaba ni a lo del miércoles
	// pasado. Con "los últimos N cargados" la ventana se ajusta sola: al que
	// carga mucho le cubre un día, al que carga poco le cubre semanas.
	//
	// recencyWindow queda como techo contra fósiles, no como la ventana real: sin
	// él, un usuario con 5 movimientos en total vería uno del año pasado como
	// candidato de "eran 1500".
	recencyLimit  = 30
	recencyWindow = 90 * 24 * time.Hour
	// justCreatedWindow: dentro de esta ventana, un mensaje SIN referente
	// textual se resuelve al último movimiento cargado en vez de abrir un
	// picker. Generosa a propósito — solo se consulta cuando no hay ninguna
	// otra señal, y el confirm sigue pidiendo el OK del usuario.
	justCreatedWindow = 10 * time.Minute
)

// transactionGroup is a set of movements sharing one transaction_id (or
// a single standalone movement without one) — the unit every UPDATE/
// DELETE reference-resolution step operates on, never a lone row.
type transactionGroup struct {
	TransactionID string
	Movements     []movement.Movement
}

func groupByTransaction(ms []movement.Movement) []transactionGroup {
	order := make([]string, 0, len(ms))
	byKey := make(map[string][]movement.Movement)

	for _, m := range ms {
		key := fmt.Sprintf("single:%d", m.ID)
		if m.TransactionID != nil {
			key = m.TransactionID.String()
		}
		if _, seen := byKey[key]; !seen {
			order = append(order, key)
		}
		byKey[key] = append(byKey[key], m)
	}

	groups := make([]transactionGroup, 0, len(order))
	for _, key := range order {
		txID := key
		if strings.HasPrefix(key, "single:") {
			txID = ""
		}
		groups = append(groups, transactionGroup{TransactionID: txID, Movements: byKey[key]})
	}
	return groups
}

// matchesMessage is a cheap, dependency-free relevance filter over one
// candidate group: it matches when any description TOKEN of length >= 4
// appears (case-insensitively) in the message, or the message literally
// contains one of the group's amounts. Token-level (not whole-phrase) so a
// verbose LLM description like "gasto en trabas" matches a message that
// shares only "trabas". The DB layer no longer pre-filters by similarity
// (see FindSimilarForUser / resolveCandidates), so this is the primary
// textual relevance check.
func matchesMessage(group transactionGroup, message string) bool {
	lower := foldAccents(strings.ToLower(message))
	for _, m := range group.Movements {
		if tokenAppearsIn(m.Description, lower) {
			return true
		}
		if !m.Amount.IsZero() && strings.Contains(message, m.Amount.String()) {
			return true
		}
	}
	return false
}

// tokenAppearsIn reports whether any whitespace-separated token of `field`
// with length >= minMatchTokenLen is a substring of the already-lowercased
// haystack.
func tokenAppearsIn(field *string, lowerHaystack string) bool {
	if field == nil || *field == "" {
		return false
	}
	return tokenAppearsInString(*field, lowerHaystack)
}

// tokenAppearsInString es la misma prueba sobre un string ya desreferenciado.
// La usa guessNamesOwnAccount, que compara contra la description de la fila y
// no contra el mensaje. La implementación vive en flow (movement_text.go).
func tokenAppearsInString(field, lowerHaystack string) bool {
	return flow.TokenAppearsInString(field, lowerHaystack)
}

// dropReservedGroups saca los grupos cuya categoría es interna (Sistema,
// PENDING_REVIEW). Solo se usa en el fallback de resolveCandidates — ver el
// comentario ahí.
func dropReservedGroups(groups []transactionGroup) []transactionGroup {
	out := make([]transactionGroup, 0, len(groups))
	for _, g := range groups {
		if len(g.Movements) > 0 && g.Movements[0].Subcategory != nil &&
			subcategory.IsReserved(g.Movements[0].Subcategory.Category) {
			continue
		}
		out = append(out, g)
	}
	return out
}

// resolveCandidates finds the transaction group(s) a message could refer
// to for UPDATE/DELETE. Default window is today (America/Argentina/
// Buenos_Aires); a mentioned dateFrom/dateTo anchors it instead. It fetches
// the whole window (see FindSimilarForUser) and matches in-process via
// matchesMessage. When nothing matches textually it does NOT dead-end —
// it returns the window's most-recent groups (capped) so the caller can
// ask "¿cuál?". It never auto-picks: the caller still confirms (1) or
// shows a picker (2+).
func (c *controller) resolveCandidates(userID uint64, message, dateFrom, dateTo string) ([]transactionGroup, error) {
	var matches []movement.Movement
	var err error
	if dateFrom == "" && dateTo == "" {
		// No date named → "what did I just do": recency of ENTRY (created_at),
		// not business date. A movement entered now but dated in the past
		// ("le pagué el asado de ayer") must still be a candidate.
		matches, err = c.movements.FindRecentlyCreatedForUser(userID, time.Now().Add(-recencyWindow), recencyLimit)
	} else {
		since := startOfTodayArgentina()
		if dateFrom != "" {
			if anchor, perr := time.Parse("2006-01-02", dateFrom); perr == nil {
				since = anchor.Add(-dateAnchorMargin)
			}
		}
		var until *time.Time
		if dateTo != "" {
			if anchor, perr := time.Parse("2006-01-02", dateTo); perr == nil {
				u := anchor.Add(dateAnchorMargin)
				until = &u
			}
		}
		matches, err = c.movements.FindSimilarForUser(userID, message, since, until)
	}
	if err != nil {
		return nil, err
	}

	groups := groupByTransaction(matches)

	var candidates []transactionGroup
	for _, g := range groups {
		if matchesMessage(g, message) {
			candidates = append(candidates, g)
		}
	}
	if len(candidates) > 0 {
		// Mismo techo que el fallback: un mensaje ambiguo ("el super") puede
		// matchear decenas de movimientos en una base con historia, y un picker
		// de veinte botones no se lee — además de que conversation_states
		// guardaría los veinte grupos enteros en JSONB. El corte es por
		// recencia porque candidates hereda el orden newest-first de groups.
		// ponytail: si el correcto queda afuera del corte seguido, el paso
		// siguiente es rankear por similitud en vez de cortar por recencia.
		if len(candidates) > fallbackRecentCap {
			candidates = candidates[:fallbackRecentCap]
		}
		return candidates, nil
	}

	// Nada matchó textualmente: esto ya no es resolver una referencia, es "te
	// muestro lo reciente". Los movimientos de una categoría reservada (Sistema:
	// saldos iniciales, ajustes, transferencias internas) no son candidatos de
	// una corrección — el usuario nunca los nombró, y encima no tienen
	// descripción, así que el botón sale como "21528105 ARS · · 2".
	// Se filtran SOLO acá: si el usuario nombra una transferencia, el match
	// textual de arriba ya la encontró y ahí sí es un candidato legítimo.
	groups = dropReservedGroups(groups)

	// Antes de ofrecer un picker: si el usuario acaba
	// de cargar un movimiento, casi seguro se refiere a ese. El caso real es
	// "Pan 2 mil" y treinta segundos después "Eran 1500" — un mensaje así no
	// tiene NINGÚN referente textual ("Pan" son 3 caracteres, y el 1500 es el
	// monto nuevo), así que la recencia de carga es la mejor señal disponible, y
	// mucho mejor que hacerlo elegir entre cinco movimientos cualesquiera.
	// El usuario confirma igual antes de que se aplique nada.
	if len(groups) > 0 && time.Since(groups[0].Movements[0].CreatedAt) <= justCreatedWindow {
		return groups[:1], nil
	}

	// Rather than dead-end, offer the most recent movements in the window as a
	// picker. groups is already ordered newest-first by the window query
	// (created_at DESC in the default no-date path, date DESC when a date was
	// mentioned).
	if len(groups) > fallbackRecentCap {
		groups = groups[:fallbackRecentCap]
	}
	return groups, nil
}
