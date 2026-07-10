package reminder

import "math/rand"

// reminderMessages is the rotating pool of expense-logging nudges. Argentine,
// mixed tone (motivador, canchero, seco, dato útil). "No siempre iguales" is
// solved here — extend with a commit, no LLM needed. Keep each line short and
// standalone (a push notification, no context assumed).
var reminderMessages = []string{
	"¿Cargaste tus gastos de hoy? Un minuto ahora te ahorra el quilombo de fin de mes 💪",
	"Che, ¿anotamos lo de hoy antes de que se te escape? 📲",
	"Recordatorio suave: tus gastos del día todavía no están cargados 🙌",
	"Lo que no se mide, no se cuida. ¿Sumamos los gastos de hoy? 📊",
	"¿Todo bajo control hoy? Pasame lo que gastaste y lo dejamos anotado 👌",
	"Dos minutos de carga hoy = cero sorpresas a fin de mes. ¿Vamos? ⏱️",
	"Tu yo del futuro te va a agradecer que cargues los gastos de hoy 🚀",
	"¿Un cafecito, el súper, la nafta? Contame en qué se te fue el día ☕",
	"Todavía no vi movimientos tuyos hoy. ¿Anotamos algo? ✍️",
	"Mantené la racha: cargá los gastos del día y seguimos al día 🔥",
	"Pequeño hábito, gran diferencia: registrá lo de hoy 📝",
	"¿Se te pasó algo hoy? Tirámelo y lo clasifico al toque ✅",
	"Ordenar las finanzas empieza por anotar. ¿Arrancamos con lo de hoy? 🧮",
	"No dejes que los gastos de hoy queden en la nebulosa. Cargalos 🌫️➡️📒",
	"Un ratito de carga hoy y llegás a fin de mes sabiendo exactamente en qué se fue la plata 💸",
}

// PickMessage returns a random nudge from the pool.
func PickMessage() string {
	return reminderMessages[rand.Intn(len(reminderMessages))]
}
