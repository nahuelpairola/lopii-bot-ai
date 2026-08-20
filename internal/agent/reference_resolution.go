package agent

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

// foldAccents y TokenCoverage viven en flow (movement_text.go), junto al
// foldAccents del matcher de nombres de cuenta; acá quedan los puentes que usa
// la resolución de referencias.
func foldAccents(s string) string { return flow.FoldAccents(s) }

// StartOfTodayArgentina es el inicio del día de hoy en la zona de Argentina.
// Exportada porque los tests de borde (que quedaron en messaging) la usan.
func StartOfTodayArgentina() time.Time {
	now := time.Now().In(constants.ArgentinaZone)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, constants.ArgentinaZone)
}

const (
	dateAnchorMargin = 24 * time.Hour
	// pickerMaxOptions acota cuántos botones ve el usuario. Un mensaje ambiguo
	// ("el super") puede matchear decenas de movimientos en una base con
	// historia, y un picker de veinte botones no se lee — además de que
	// conversation_states guardaría los veinte grupos enteros en JSONB.
	pickerMaxOptions = 5
	// recencyLimit / recencyWindow acotan la ventana de "lo que tengo fresco".
	//
	// El límite REAL es por cantidad, no por tiempo: una ventana fija servía o no
	// según el ritmo de cada usuario. Quien carga 20 por día tenía 40 candidatos
	// para elegir; quien carga 3 por semana no llegaba ni a lo del miércoles
	// pasado. Con "los últimos N cargados" la ventana se ajusta sola: al que
	// carga mucho le cubre un día, al que carga poco le cubre semanas.
	//
	// 60 y no 30: con 30 el débito de tarjeta del 2026-08-16 quedaba en la fila
	// 32, dos afuera, y ocho intentos seguidos fallaron. Ver docs/decisions.md.
	//
	// recencyWindow queda como techo contra fósiles, no como la ventana real: sin
	// él, un usuario con 5 movimientos en total vería uno del año pasado como
	// candidato de "eran 1500".
	recencyLimit  = 60
	recencyWindow = 90 * 24 * time.Hour
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

// dateTieBreak es el techo de lo que puede aportar la cercanía de fecha al
// puntaje. Vale menos que la diferencia de cobertura más chica que nos importa
// (1/2 - 1/3 = 0,17), así que DESEMPATA y no da vuelta una diferencia de
// cobertura. Números medidos en docs/decisions.md.
const dateTieBreak = 0.25

// scoreGroup puntúa cuánto se parece un grupo candidato al mensaje. 0 significa
// "no matchea" y el grupo no entra: sin esa condición el término de fecha haría
// candidato a cualquier cosa y el fallback por recencia dejaría de existir.
//
// La cobertura es del lado de la DESCRIPCIÓN, no del mensaje: lo que preguntamos
// es "cuánto de lo que dice esta fila está en lo que escribió el usuario". El
// monto literal presente en el mensaje vale 1: nombrar el número es tan bueno
// como nombrar la cosa entera. Se compara contra Abs(): el signo contable
// nunca llega al usuario, así que tampoco puede ser parte del match.
//
// Se toma el máximo sobre los movimientos del grupo — un grupo es una
// transacción y puede tener dos piernas; alcanza con que una la nombre.
func scoreGroup(g transactionGroup, message string, anchor time.Time) float64 {
	lower := foldAccents(strings.ToLower(message))
	best := 0.0
	for _, m := range g.Movements {
		cover := 0.0
		if !m.Amount.IsZero() && strings.Contains(message, m.Amount.Abs().String()) {
			cover = 1
		}
		if m.Description != nil {
			if c := flow.TokenCoverage(*m.Description, lower); c > cover {
				cover = c
			}
		}
		if cover > best {
			best = cover
		}
	}
	if best == 0 {
		return 0
	}
	days := math.Abs(anchor.Sub(g.Movements[0].Date).Hours()) / 24
	return best + dateTieBreak/(1+days)
}

// matchesMessage dice si el grupo matchea, sin importar cuánto. Queda porque el
// cartel del picker lo necesita (agent_executor.go): saber si la lista salió de
// un match textual o del fallback por recencia.
func matchesMessage(group transactionGroup, message string) bool {
	return scoreGroup(group, message, time.Now()) > 0
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

// parseDateAnchor devuelve la fecha YYYY-MM-DD, o nil si está vacía o no
// parsea. Nil y no un error: una fecha que el modelo mandó mal degrada a la
// ventana por defecto, no rompe la corrección.
func parseDateAnchor(s string) *time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

// resolveCandidates finds the transaction group(s) a message could refer
// to for UPDATE/DELETE. Default window is today (America/Argentina/
// Buenos_Aires); a mentioned dateFrom/dateTo anchors it instead. It fetches
// the whole window (see FindSimilarForUser) and matches in-process via
// matchesMessage. When nothing matches textually it does NOT dead-end —
// it returns the window's most-recent groups (capped) so the caller can
// ask "¿cuál?". It never auto-picks: the caller still confirms (1) or
// shows a picker (2+).
func resolveCandidates(svc agentServices, userID uint64, message, dateFrom, dateTo string) ([]transactionGroup, error) {
	var matches []movement.Movement
	var err error
	if dateFrom == "" && dateTo == "" {
		// No date named → "what did I just do": recency of ENTRY (created_at),
		// not business date. A movement entered now but dated in the past
		// ("le pagué el asado de ayer") must still be a candidate.
		matches, err = svc.FindRecentlyCreatedForUser(userID, time.Now().Add(-recencyWindow), recencyLimit)
	} else {
		// Una fecha sola tiene que cerrar la ventana de los DOS lados, y por eso
		// cada extremo cae de vuelta en el otro:
		//
		//   - Sólo dateFrom dejaba until en nil, o sea abierta hasta hoy. Eso no
		//     acota nada: es lo mismo que no haber pasado fecha, que es justo el
		//     bug del 2026-08-15 —el movimiento del 04/08 estaba en la posición 36
		//     de una ventana de 30 y el picker ofrecía cualquier otra cosa—.
		//   - Sólo dateTo dejaba since en el arranque de HOY contra un until
		//     pasado: la ventana salía invertida y no podía devolver nada.
		from, to := parseDateAnchor(dateFrom), parseDateAnchor(dateTo)
		if from == nil {
			from = to
		}
		if to == nil {
			to = from
		}
		since := StartOfTodayArgentina()
		var until *time.Time
		if from != nil {
			since = from.Add(-dateAnchorMargin)
			u := to.Add(dateAnchorMargin)
			until = &u
		}
		matches, err = svc.MovementsFindSimilarForUser(userID, message, since, until)
	}
	if err != nil {
		return nil, err
	}

	groups := groupByTransaction(matches)

	// El ancla del desempate por fecha: la fecha que nombró el mensaje si hay
	// una, hoy si no. Es la misma que acotó la ventana unas líneas más arriba.
	anchor := StartOfTodayArgentina()
	if from := parseDateAnchor(dateFrom); from != nil {
		anchor = *from
	} else if to := parseDateAnchor(dateTo); to != nil {
		anchor = *to
	}

	type scored struct {
		group transactionGroup
		score float64
	}
	var candidates []scored
	for _, g := range groups {
		if s := scoreGroup(g, message, anchor); s > 0 {
			candidates = append(candidates, scored{group: g, score: s})
		}
	}
	if len(candidates) > 0 {
		// Estable a propósito: a puntaje igual gana el que vino primero de la
		// consulta, o sea el más reciente. Sin SliceStable, dos candidatos
		// idénticos salen en orden arbitrario y el picker cambia entre corridas.
		sort.SliceStable(candidates, func(i, j int) bool {
			return candidates[i].score > candidates[j].score
		})
		if len(candidates) > pickerMaxOptions {
			candidates = candidates[:pickerMaxOptions]
		}
		out := make([]transactionGroup, 0, len(candidates))
		for _, c := range candidates {
			out = append(out, c.group)
		}
		return out, nil
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
	if len(groups) > 0 && time.Since(groups[0].Movements[0].CreatedAt) <= flow.JustCreatedWindow {
		return groups[:1], nil
	}

	// Rather than dead-end, offer the most recent movements in the window as a
	// picker. groups is already ordered newest-first by the window query
	// (created_at DESC in the default no-date path, date DESC when a date was
	// mentioned).
	if len(groups) > pickerMaxOptions {
		groups = groups[:pickerMaxOptions]
	}
	return groups, nil
}
