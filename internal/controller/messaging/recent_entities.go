package messaging

import (
	"fmt"
	"strings"
	"time"

	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

// recentEntitiesCap acota cuántas filas entran al prompt. La ventana ya es
// justCreatedWindow; esto es el techo de tokens.
const recentEntitiesCap = 8

// buildRecentEntities arma el bloque estructurado de movimientos recientes que
// va al prompt del loop.
//
// Reemplaza a la transcripción del hilo para resolver referencias: el problema
// es una anáfora ("el café de hoy"), no memoria conversacional. Best-effort —
// si la consulta falla, se corre sin bloque, igual que con el historial.
func (c *controller) buildRecentEntities(userID uint64) string {
	movs, err := c.movements.FindRecentlyCreatedForUser(
		userID, nearDuplicateWindowStart(time.Now()), recentEntitiesCap)
	if err != nil {
		return ""
	}
	return renderRecentEntities(movs)
}

// renderRecentEntities es la parte pura, para poder probarla sin base.
//
// El signo NUNCA sale de storage: el usuario y el modelo ven el valor absoluto
// y la dirección la da el tipo. Un "-12700" acá invitaría al modelo a copiar el
// signo de vuelta.
func renderRecentEntities(movs []movement.Movement) string {
	if len(movs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, m := range movs {
		desc := ""
		if m.Description != nil {
			desc = strings.TrimSpace(*m.Description)
		}
		if desc == "" && m.Subcategory != nil {
			desc = m.Subcategory.Subcategory
		}
		account := ""
		if m.Account != nil {
			account = " · " + m.Account.Name
		}
		fmt.Fprintf(&b, "#%d  %s  %s%s  (%s)\n",
			m.ID, desc, currency.FormatMoney(m.Amount.Abs(), m.Currency), account, agoLabel(m.CreatedAt))
	}
	return strings.TrimRight(b.String(), "\n")
}

// agoLabel dice hace cuánto, en la granularidad que le sirve al modelo para
// entender "de hoy" / "recién".
func agoLabel(at time.Time) string {
	d := time.Since(at)
	if d < time.Minute {
		return "recién"
	}
	return fmt.Sprintf("hace %d min", int(d.Minutes()))
}
