package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
)

// El gate de casi-duplicado viaja EN el recibo, como botones del mensaje que el
// usuario iba a recibir igual. No agrega un mensaje ni un paso bloqueante: el
// resultado por default —ignorarlo— es exactamente el de hoy, dos filas y
// totales correctos. Esa propiedad es la que lo hace shippeable.
const (
	// nearDupPrefix ·acción· id insertado · id previo. callback_data son 64
	// bytes: van ids, nunca etiquetas.
	nearDupPrefix  = "nd:"
	nearDupSeparte = "sep"
	nearDupMerge   = "mrg"
	nearDupReplace = "rpl"

	// nearDupRecentLimit acota cuántos previos se traen para comparar. La
	// ventana ya es de 10 minutos; esto es sólo un techo de seguridad.
	nearDupRecentLimit = 20
)

// nearDuplicateButtons arma las tres opciones del recibo.
func nearDuplicateButtons(insertedID, priorID uint) []conversation.Button {
	id := func(action string) string {
		return fmt.Sprintf("%s%s:%d:%d", nearDupPrefix, action, insertedID, priorID)
	}
	return []conversation.Button{
		{Label: "Va aparte", Data: id(nearDupSeparte)},
		{Label: "Sumalo a ese", Data: id(nearDupMerge)},
		{Label: "Reemplazalo", Data: id(nearDupReplace)},
	}
}

// maybeNearDuplicate corre el gate sobre lo recién insertado y, si marca,
// devuelve los botones para colgar del recibo.
//
// Best-effort: si la búsqueda de previos falla, no marca. Un gate que rompe un
// CREATE que ya se escribió sería peor que el duplicado que viene a evitar.
func (c *controller) maybeNearDuplicate(userID uint64, inserted []movement.Movement) []conversation.Button {
	if len(inserted) == 0 {
		return nil
	}
	sameTurn := make([]uint, 0, len(inserted))
	for _, m := range inserted {
		sameTurn = append(sameTurn, m.ID)
	}

	priors, err := c.movements.FindRecentlyCreatedForUser(
		userID, nearDuplicateWindowStart(time.Now()), nearDupRecentLimit)
	if err != nil {
		return nil
	}

	// Como mucho UNA pregunta por mensaje, sin importar cuántas filas entraron:
	// el presupuesto de interrupciones es de una por mensaje.
	for _, m := range inserted {
		if prior := findNearDuplicate(m, sameTurn, priors); prior != nil {
			slog.Info("near duplicate flagged", "user_id", userID, "inserted", m.ID, "prior", prior.ID)
			return nearDuplicateButtons(m.ID, prior.ID)
		}
	}
	return nil
}

// handleNearDuplicateChoice atiende el tap. Va ANTES del engine, como el de los
// tips: un flow abierto no se puede comer este callback como si fuera una
// opción suya.
func (c *controller) handleNearDuplicateChoice(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, data string) bool {
	if !strings.HasPrefix(data, nearDupPrefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(data, nearDupPrefix), ":")
	if len(parts) != 3 {
		return true // es nuestro, pero está roto: consumirlo igual
	}
	action := parts[0]
	insertedID, err1 := strconv.ParseUint(parts[1], 10, 64)
	priorID, err2 := strconv.ParseUint(parts[2], 10, 64)
	if err1 != nil || err2 != nil {
		return true
	}

	if action == nearDupSeparte {
		c.sendText(ctx, b, chatID, msgNearDupSeparate)
		return true
	}

	if err := c.applyNearDuplicateChoice(userID, action, uint(insertedID), uint(priorID)); err != nil {
		slog.ErrorContext(ctx, "near duplicate choice failed",
			"user_id", userID, "action", action, "inserted", insertedID, "prior", priorID, "err", err)
		c.sendText(ctx, b, chatID, msgCouldNotSave("el cambio"))
		return true
	}
	c.sendText(ctx, b, chatID, msgNearDupMerged)
	return true
}

// applyNearDuplicateChoice es el camino de plata: fusiona o reemplaza el monto
// del previo y borra el recién insertado.
//
// Se releen las dos filas de la base en vez de confiar en el callback: el tap
// puede llegar tarde, y sumarle un monto a una fila que ya cambió sería
// corromper un saldo por una pantalla vieja.
//
// ponytail: escribe sin pasar por movement.Normalize, y puede. Las dos filas ya
// están normalizadas —salieron del guard al insertarse— y nearDuplicateCandidate
// exige que compartan tipo, moneda y cuenta, así que comparten signo: la suma no
// puede dar cero ni invertirse, y ni la moneda ni la cuenta cambian acá. Llamar
// a Normalize obligaría a traer el mapa de cuentas para re-derivar lo que el
// candidato ya garantiza. El techo: si alguna vez se afloja la regla de mismo
// tipo en near_duplicate.go, esto SÍ necesita el guard.
func (c *controller) applyNearDuplicateChoice(userID uint64, action string, insertedID, priorID uint) error {
	recent, err := c.movements.FindRecentlyCreatedForUser(userID, nearDuplicateWindowStart(time.Now()), nearDupRecentLimit)
	if err != nil {
		return fmt.Errorf("near duplicate: find recent: %w", err)
	}
	var inserted, prior *movement.Movement
	for i := range recent {
		switch recent[i].ID {
		case insertedID:
			inserted = &recent[i]
		case priorID:
			prior = &recent[i]
		}
	}
	if inserted == nil || prior == nil {
		return fmt.Errorf("near duplicate: ya no están las dos filas (inserted=%v prior=%v)", inserted != nil, prior != nil)
	}

	merged := *prior
	switch action {
	case nearDupMerge:
		merged.Amount = prior.Amount.Add(inserted.Amount)
	case nearDupReplace:
		merged.Amount = inserted.Amount
	default:
		return fmt.Errorf("near duplicate: acción desconocida %q", action)
	}
	merged.ID = 0 // ReplaceMovements borra la vieja e inserta esta

	if err := c.movements.ReplaceMovements([]uint{prior.ID}, []movement.Movement{merged}); err != nil {
		return fmt.Errorf("near duplicate: replace: %w", err)
	}
	if err := c.movements.SoftDeleteByIDs([]uint{inserted.ID}); err != nil {
		return fmt.Errorf("near duplicate: delete inserted: %w", err)
	}
	return nil
}
