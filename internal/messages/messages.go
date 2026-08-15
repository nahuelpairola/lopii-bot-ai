package messages

import (
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
)

// La copy que comparten el loop del agente (internal/agent) y el borde
// Telegram (internal/controller/messaging). Un solo origen, dos consumidores.
// Los nombres van en mayúscula porque se usan cruzando paquetes.

// MsgHelp es la explicación de qué puede hacer el bot. La manda el loop cuando
// el modelo elige ToolReplyHelp, y es copy tuneada que tiene que salir textual.
const MsgHelp = "Conmigo es fácil, me hablás normal:\n\n" +
	"📝 Anotar: «gasté 500 en el súper», «me pagaron 10 mil»\n" +
	"✏️ Corregir: «el súper eran 600»\n" +
	"🗑️ Borrar: «borrá el último gasto»\n" +
	"🔄 Transferir: «pasé 50 mil del banco a MP»\n" +
	"❓ Preguntar: «¿cuánto gasté esta semana?»\n" +
	"🗂️ Categorías: «creá una categoría para mascotas», «sacá la que repetí»\n" +
	"🏦 Cuentas y recordatorios: pedímelos cuando quieras."

// MsgAskRewrite: el modelo no entendió y pedirle reescribir es la vía honesta.
const MsgAskRewrite = "✍️ Dale, mandalo de nuevo con más detalle (monto, categoría, y si es un movimiento nuevo)."

// MsgNoCandidatesFound: no hay movimientos de la ventana para tocar. El picker
// de fallback cubre todos los demás casos.
const MsgNoCandidatesFound = "No tengo movimientos de ese día para tocar. ¿De qué fecha era?"

// MsgPartialSuccessAfterWrite: el turno ya insertó y después se quedó sin cupo.
// NO se encola — reintentarlo duplicaría la plata ya registrada — así que el
// mensaje tiene que dejar claras las dos mitades: lo que entró está guardado,
// y lo que falte lo tiene que volver a mandar él.
const MsgPartialSuccessAfterWrite = "Registré lo que me pediste, pero me quedé sin margen para el resto. Mandame de nuevo lo que falte."

// MsgConfirmMovements es el recibo del alta. Vive en flow (MsgConfirmMovements);
// el alias conserva el nombre corto para el ejecutor del loop.
func MsgConfirmMovements(movements []movement.Movement) string {
	return flow.MsgConfirmMovements(movements)
}

// MsgInsufficientFunds es la copy del gate de saldo negativo. Vive en flow
// (MsgInsufficientFunds); el alias conserva el nombre corto para el ejecutor.
func MsgInsufficientFunds(short []movement.AccountShortfall) string {
	return flow.MsgInsufficientFunds(short)
}

// MsgAskWhatToChange se usa cuando el movimiento SÍ se encontró pero el mensaje
// no dice qué cambiarle ("el café estaba mal"). Antes acá iba un "no me quedó
// claro, decímelo de nuevo" que era un callejón sin salida: el usuario había
// nombrado bien el movimiento y se quedaba sin nada.
//
// Pide el VALOR NUEVO, no el campo. La primera versión listaba "(el monto, la
// categoría, la fecha…)" y se leía como un menú: en la prueba real el usuario
// contestó "El monto" — nombró el campo, que es exactamente lo que no sirve.
// ResolveUpdate necesita con qué reemplazar, así que los ejemplos son
// respuestas COMPLETAS, no nombres de campo.
//
// Arranca por el monto porque es lo que se corrige casi siempre; el resto entra
// igual por el mismo texto libre.
func MsgAskWhatToChange(rows []movement.MovementRow) string {
	const ask = "¿Cuánto era? Escribime el monto — o tocá abajo si lo que está mal es otra cosa."
	if len(rows) == 0 {
		return ask
	}
	return "Encontré " + movement.MovementGapDescriptor(rows[0]) + ". " + ask
}

// MsgAskChangeValue es la segunda vuelta: ya sabemos QUÉ campo, falta el valor.
const MsgAskChangeValue = "Dale. ¿Y cuál es el valor nuevo?"

// MsgStillCannotCorrect va cuando YA le preguntamos qué cambiar y con la
// respuesta tampoco sale una corrección. Volver a preguntar lo mismo sería
// hacerlo girar; se corta nombrando el formato que sí funciona.
const MsgStillCannotCorrect = "Sigo sin darme cuenta qué cambiarle. Probá diciéndomelo derecho — ej: «el café fueron 2000»."

// MsgRefundWouldGrow: el mensaje dice que le devolvieron plata y el cambio
// haría crecer el gasto. Se nombra la contradicción y se pregunta el número,
// que es el dato que falta.
const MsgRefundWouldGrow = "Me dijiste que te devolvieron plata, pero el cambio que entendí lo dejaría más caro. ¿Cuánto te devolvieron?"

// MsgRefundExceeds: te devolvieron MÁS de lo que salió. Sin la guarda, restar
// daría vuelta el signo y Normalize lo re-firmaría como INGRESO: un gasto
// convertido en entrada de plata por un número mal leído.
const MsgRefundExceeds = "Me decís que te devolvieron más de lo que salió ese movimiento 🤔 ¿Cuánto fue?"

// MsgAmbiguousSetAll: "poné todos en 1500" sobre varios movimientos. Aplanar
// n montos distintos al mismo número no es algo que nadie quiera; preguntar
// cuál es más barato que deshacerlo después.
const MsgAmbiguousSetAll = "¿A cuál de todos le pongo ese monto? Decime cuál y lo cambio."

// MsgCorrectionChangesNothing: el cambio pedido deja el movimiento igual.
// Decirlo es mejor que confirmar un reemplazo que no reemplaza nada.
const MsgCorrectionChangesNothing = "Eso ya estaba así, no cambié nada."

// MsgAgentActionDiscarded sale cuando se agota el presupuesto de preguntas.
// NOMBRA lo que se cayó a propósito: tirar algo en silencio es la falla que
// todo el parking existe para evitar.
const MsgAgentActionDiscardedTemplate = "No terminé de entender %s, así que lo dejo sin hacer.\n\nSi querés, escribímelo de nuevo con un poco más de detalle."

// DisplayAmount renders a movement amount for any audience outside storage —
// the user and the LLM (as an UPDATE/DELETE candidate). The stored sign is
// internal; everyone sees the magnitude, direction comes from the type.
//
// Sin formato de miles: lo consume también el LLM (como candidato de
// UPDATE/DELETE) y ahí un "$3.000" es peor que un "3000" — para el usuario está
// rowMoney/currency.FormatMoney.
func DisplayAmount(d decimal.Decimal) string {
	return d.Abs().String()
}
