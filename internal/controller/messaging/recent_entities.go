package messaging

import (
	"fmt"
	"strings"
	"time"

	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

// recentEntitiesCap acota cuántas filas entran al prompt: las 8 más recientes.
// Es el techo de tokens (~160), y es lo que hace que ensanchar la ventana no
// cueste nada — cambia CUÁLES 8 filas entran, no cuántas.
const recentEntitiesCap = 8

// buildRecentEntities arma el bloque estructurado de movimientos recientes que
// va al prompt del loop.
//
// Reemplaza a la transcripción del hilo para resolver referencias: el problema
// es una anáfora ("el café de hoy"), no memoria conversacional. Best-effort —
// si la consulta falla, se corre sin bloque, igual que con el historial.
func (c *controller) buildRecentEntities(userID uint64) string {
	// La ventana es HOY, no justCreatedWindow.
	//
	// Compartían constante y son dos preguntas distintas: "¿acabás de cargar
	// esto dos veces?" (10 minutos, el gate de casi-duplicado) y "¿a qué te
	// podés estar refiriendo?" (hoy). Nadie dice "el café de los últimos diez
	// minutos"; dice "el café de hoy".
	//
	// Medido en vivo el 2026-08-12: "El peaje ponelo en banco galicia" 17
	// minutos después de cargarlo. El bloque traía sólo la nafta, así que el
	// modelo no tenía NINGÚN peaje al que referirse y pidió el monto para
	// registrarlo — que es la inferencia correcta con lo que podía ver. No fue
	// un error del modelo: le faltaba el ancla.
	movs, err := c.movements.FindRecentlyCreatedForUser(
		userID, startOfTodayArgentina(), recentEntitiesCap)
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
