package flow

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
)

// El gate de casi-duplicado viaja EN el recibo, como botones del mensaje que el
// usuario iba a recibir igual. No agrega un mensaje ni un paso bloqueante: el
// resultado por default —ignorarlo— es exactamente el de hoy, dos filas y
// totales correctos. Esa propiedad es la que lo hace shippeable.
const (
	// NearDupPrefix ·acción· id insertado · id previo. callback_data son 64
	// bytes: van ids, nunca etiquetas. El prefijo y las acciones están
	// exportados porque el harness del borde arma los callbacks a mano.
	NearDupPrefix  = "nd:"
	NearDupSeparte = "sep"
	NearDupMerge   = "mrg"
	NearDupReplace = "rpl"

	// nearDupRecentLimit acota cuántos previos se traen para comparar. La
	// ventana ya es de 10 minutos; esto es sólo un techo de seguridad.
	nearDupRecentLimit = 20
)

// nearDuplicateButtons arma las tres opciones del recibo.
func nearDuplicateButtons(insertedID, priorID uint) []conversation.Button {
	id := func(action string) string {
		return fmt.Sprintf("%s%s:%d:%d", NearDupPrefix, action, insertedID, priorID)
	}
	return []conversation.Button{
		{Label: "Va aparte", Data: id(NearDupSeparte)},
		{Label: "Sumalo a ese", Data: id(NearDupMerge)},
		{Label: "Reemplazalo", Data: id(NearDupReplace)},
	}
}

// MaybeNearDuplicate corre el gate sobre lo recién insertado y, si marca,
// devuelve los botones para colgar del recibo.
//
// Best-effort: si la búsqueda de previos falla, no marca. Un gate que rompe un
// CREATE que ya se escribió sería peor que el duplicado que viene a evitar.
func MaybeNearDuplicate(r runner, userID uint64, inserted []movement.Movement) []conversation.Button {
	if len(inserted) == 0 {
		return nil
	}
	sameTurn := make([]uint, 0, len(inserted))
	for _, m := range inserted {
		sameTurn = append(sameTurn, m.ID)
	}

	priors, err := r.FindRecentlyCreatedForUser(
		userID, NearDuplicateWindowStart(time.Now()), nearDupRecentLimit)
	if err != nil {
		return nil
	}

	// Como mucho UNA pregunta por mensaje, sin importar cuántas filas entraron:
	// el presupuesto de interrupciones es de una por mensaje.
	for _, m := range inserted {
		if prior := FindNearDuplicate(m, sameTurn, priors); prior != nil {
			slog.Info("near duplicate flagged", "user_id", userID, "inserted", m.ID, "prior", prior.ID)
			return nearDuplicateButtons(m.ID, prior.ID)
		}
	}
	return nil
}

// HandleNearDuplicateChoice atiende el tap. Va ANTES del engine, como el de los
// tips: un flow abierto no se puede comer este callback como si fuera una
// opción suya. Devuelve true si el callback era nuestro.
func HandleNearDuplicateChoice(ctx context.Context, r runner, chat messenger.Chat, userID uint64, data string) bool {
	if !strings.HasPrefix(data, NearDupPrefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(data, NearDupPrefix), ":")
	if len(parts) != 3 {
		return true // es nuestro, pero está roto: consumirlo igual
	}
	action := parts[0]
	insertedID, err1 := strconv.ParseUint(parts[1], 10, 64)
	priorID, err2 := strconv.ParseUint(parts[2], 10, 64)
	if err1 != nil || err2 != nil {
		return true
	}

	if action == NearDupSeparte {
		r.SendText(ctx, chat, MsgNearDupSeparate)
		return true
	}

	if err := ApplyNearDuplicateChoice(r, userID, action, uint(insertedID), uint(priorID)); err != nil {
		slog.ErrorContext(ctx, "near duplicate choice failed",
			"user_id", userID, "action", action, "inserted", insertedID, "prior", priorID, "err", err)
		r.SendText(ctx, chat, MsgCouldNotSave("el cambio"))
		return true
	}
	r.SendText(ctx, chat, MsgNearDupMerged)
	return true
}

// ApplyNearDuplicateChoice es el camino de plata: fusiona o reemplaza el monto
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
func ApplyNearDuplicateChoice(r runner, userID uint64, action string, insertedID, priorID uint) error {
	recent, err := r.FindRecentlyCreatedForUser(userID, NearDuplicateWindowStart(time.Now()), nearDupRecentLimit)
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
	case NearDupMerge:
		merged.Amount = prior.Amount.Add(inserted.Amount)
	case NearDupReplace:
		merged.Amount = inserted.Amount
	default:
		return fmt.Errorf("near duplicate: acción desconocida %q", action)
	}
	merged.ID = 0 // ReplaceMovements borra la vieja e inserta esta

	if err := r.ReplaceMovements([]uint{prior.ID}, []movement.Movement{merged}); err != nil {
		return fmt.Errorf("near duplicate: replace: %w", err)
	}
	if err := r.SoftDeleteByIDs([]uint{inserted.ID}); err != nil {
		return fmt.Errorf("near duplicate: delete inserted: %w", err)
	}
	return nil
}
