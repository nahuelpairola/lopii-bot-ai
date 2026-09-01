package messages

import (
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/movement"
)

const MsgHelp = "Conmigo es fácil, me hablás normal:\n\n" +
	"📝 Anotar: «gasté 500 en el súper», «me pagaron 10 mil»\n" +
	"✏️ Corregir: «el súper eran 600»\n" +
	"🗑️ Borrar: «borrá el último gasto»\n" +
	"🔄 Transferir: «pasé 50 mil del banco a MP»\n" +
	"❓ Preguntar: «¿cuánto gasté esta semana?»\n" +
	"🗂️ Categorías: «creá una categoría para mascotas», «sacá la que repetí»\n" +
	"🏦 Cuentas y recordatorios: pedímelos cuando quieras."

const MsgAskRewrite = "✍️ Dale, mandalo de nuevo con más detalle (monto, categoría, y si es un movimiento nuevo)."

const MsgNoCandidatesFound = "No tengo movimientos de ese día para tocar. ¿De qué fecha era?"

const MsgPartialSuccessAfterWrite = "Registré lo que me pediste, pero me quedé sin margen para el resto. Mandame de nuevo lo que falte."

func MsgAskWhatToChange(rows []movement.MovementRow) string {
	const ask = "¿Cuánto era? Escribime el monto — o tocá abajo si lo que está mal es otra cosa."
	if len(rows) == 0 {
		return ask
	}
	return "Encontré " + movement.MovementGapDescriptor(rows[0]) + ". " + ask
}

const MsgAskChangeValue = "Dale. ¿Y cuál es el valor nuevo?"

const MsgStillCannotCorrect = "Sigo sin darme cuenta qué cambiarle. Probá diciéndomelo derecho — ej: «el café fueron 2000»."

const MsgRefundWouldGrow = "Me dijiste que te devolvieron plata, pero el cambio que entendí lo dejaría más caro. ¿Cuánto te devolvieron?"

const MsgRefundExceeds = "Me decís que te devolvieron más de lo que salió ese movimiento 🤔 ¿Cuánto fue?"

const MsgAmbiguousSetAll = "¿A cuál de todos le pongo ese monto? Decime cuál y lo cambio."

const MsgCorrectionChangesNothing = "Eso ya estaba así, no cambié nada."

const MsgAgentActionDiscardedTemplate = "No terminé de entender %s, así que lo dejo sin hacer.\n\nSi querés, escribímelo de nuevo con un poco más de detalle."

func DisplayAmount(d decimal.Decimal) string {
	return d.Abs().String()
}
