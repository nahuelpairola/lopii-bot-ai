package messaging

import (
	"time"

	"lopiibot.com/internal/agent"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

// nudgeStatsWindow es la ventana más ancha que necesita cualquier gate (el
// comparador de dos meses). Una sola query la cubre y el resto se recorta en Go.
const nudgeStatsWindow = 62

// nudgeStats es la snapshot de actividad que comparten todos los gates. Sale
// de UNA llamada a CountByDayForUser: la suma de Count da "cuántos movimientos
// en la ventana" y la cantidad de filas da "en cuántos días distintos cargó".
// Son las dos magnitudes que piden los gates, sin una query por gate.
//
// CountByDayForUser cuenta todo (cualquier tipo y moneda, transferencias
// incluidas) a propósito: los gates miden si el usuario está CARGANDO, no
// cuánto gasta.
type nudgeStats struct {
	days  []movement.DayCount
	total int64
	// sent son las keys ya enviadas a este usuario. La llena maybeNudge con lo
	// que ya trajo SentKeys — cero queries extra. Solo la lee el gate del menú.
	sent map[string]bool
}

func (c *controller) buildNudgeStats(userID uint64) *nudgeStats {
	s := &nudgeStats{}
	today := agent.StartOfTodayArgentina()
	// Best-effort: si una de las dos falla, el stats queda en cero y ningún
	// gate de densidad abre. Nunca mandar un tip es el fallo correcto acá.
	s.days, _ = c.movements.CountByDayForUser(userID, today.AddDate(0, 0, -nudgeStatsWindow), today)
	s.total, _ = c.movements.CountForUser(userID)
	return s
}

// movsSince: cuántos movimientos cargó en los últimos n días.
func (s *nudgeStats) movsSince(n int) int {
	cutoff := agent.StartOfTodayArgentina().AddDate(0, 0, -n)
	total := 0
	for _, d := range s.days {
		if !d.Date.Before(cutoff) {
			total += d.Count
		}
	}
	return total
}

// activeDaysSince: en cuántos días DISTINTOS cargó algo en los últimos n días.
// Es lo que separa "ocho gastos el mismo día" de "ocho gastos en la semana".
func (s *nudgeStats) activeDaysSince(n int) int {
	cutoff := agent.StartOfTodayArgentina().AddDate(0, 0, -n)
	days := 0
	for _, d := range s.days {
		if !d.Date.Before(cutoff) && d.Count > 0 {
			days++
		}
	}
	return days
}

// movsInMonth: movimientos de un mes calendario. 0 = el mes en curso, 1 = el
// anterior.
func (s *nudgeStats) movsInMonth(monthsAgo int) int {
	start := startOfMonth().AddDate(0, -monthsAgo, 0)
	end := start.AddDate(0, 1, 0)
	total := 0
	for _, d := range s.days {
		if !d.Date.Before(start) && d.Date.Before(end) {
			total += d.Count
		}
	}
	return total
}

// startOfMonth: primer día del mes en curso, en hora argentina.
func startOfMonth() time.Time {
	today := agent.StartOfTodayArgentina()
	return time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())
}

// dayOfMonth es el día del mes en hora argentina.
func dayOfMonth() int { return agent.StartOfTodayArgentina().Day() }

// Umbrales. Todos juntos: retocar el ritmo de los tips es cambiar un número.
const (
	activityFloorMovs   = 3  // movimientos en los últimos 7 días
	activityFloorDays   = 7  //
	recentTipMovs       = 5  // movimientos en los últimos 7 días
	topCategoryTipMovs  = 8  // movimientos del mes...
	topCategoryTipCats  = 3  // ...repartidos en al menos estas categorías
	balanceTipMovs      = 10 // movimientos históricos
	balanceTipAccounts  = 2  // cuentas
	paceTipMovs         = 8  // movimientos del mes...
	paceTipActiveDays   = 4  // ...en al menos estos días distintos
	compareTipMonthMovs = 8  // movimientos en CADA uno de los dos meses

	// Reglas de día del mes. Los gates de arriba miden al usuario; estas miden
	// el calendario. Sin ellas, "¿gasté más que el mes pasado?" disparada un
	// día 2 compara dos días contra treinta y contesta cualquier cosa.
	compareTipMinDay = 10 // antes de esto el mes en curso no es comparable
	enoughTipMinDay  = 20 // "¿me alcanzó?" a mitad de mes todavía no se sabe
	paceTipMinDay    = 8  // antes, la proyección es ruido...
	paceTipMaxDay    = 25 // ...después ya no es proyección, es el dato
)

// hasActivityFloor: ninguna pregunta analítica sale si el usuario no está
// cargando. Un usuario dormido no necesita "¿en qué gasté más?", necesita el
// reminder_offer, que tiene su propio gate y no pasa por acá.
func hasActivityFloor(s *nudgeStats) bool {
	return s.movsSince(activityFloorDays) >= activityFloorMovs
}

// distinctCategoriesThisMonth: cuántas categorías distintas tocó este mes.
// SumForUser devuelve una fila por grupo, así que la cantidad de filas ES la
// cantidad de categorías (el Total de cada fila no se usa acá).
func (c *controller) distinctCategoriesThisMonth(userID uint64) int {
	rows, err := c.movements.SumForUser(movement.MovementQuery{
		UserID:   userID,
		From:     startOfMonth(),
		To:       agent.StartOfTodayArgentina(),
		Currency: currency.ARS,
	}, "category")
	if err != nil {
		return 0
	}
	return len(rows)
}

// hasIncomeThisMonth: ¿entró algo este mes? Sin esto, "¿me alcanzó lo que
// entró?" no tiene una de sus dos mitades.
func (c *controller) hasIncomeThisMonth(userID uint64) bool {
	income := string(movement.Income)
	rows, err := c.movements.SumForUser(movement.MovementQuery{
		UserID:   userID,
		From:     startOfMonth(),
		To:       agent.StartOfTodayArgentina(),
		Currency: currency.ARS,
		Type:     &income,
	}, "none")
	return err == nil && len(rows) > 0 && !rows[0].Total.IsZero()
}

// hasEnoughDataForMonthVerdict: hay ingreso y hay gasto suficiente este mes.
// "¿Me alcanzó?" sin una de las dos mitades no es un veredicto.
func (c *controller) hasEnoughDataForMonthVerdict(userID uint64, s *nudgeStats) bool {
	return s.movsInMonth(0) >= topCategoryTipMovs && c.hasIncomeThisMonth(userID)
}

// hasUsdHoldings: tiene una cuenta en dólares con saldo distinto de cero.
// Tener la cuenta no alcanza: una cuenta USD vacía contesta "cero dólares",
// que es peor que no preguntar.
func (c *controller) hasUsdHoldings(userID uint64) bool {
	accs, err := c.accounts.FindByUserID(userID)
	if err != nil {
		return false
	}
	for _, a := range accs {
		if a.Currency != currency.USD {
			continue
		}
		bal, err := c.movements.SumAmountForAccount(uint64(a.ID))
		if err == nil && !bal.IsZero() {
			return true
		}
	}
	return false
}

// countAccounts es el gate de saldos: con una sola cuenta no hace falta que
// nadie te enseñe a pedir "cada cuenta".
func (c *controller) countAccounts(userID uint64) int {
	accs, err := c.accounts.FindByUserID(userID)
	if err != nil {
		return 0
	}
	return len(accs)
}
