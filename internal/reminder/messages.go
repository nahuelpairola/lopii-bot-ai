package reminder

import (
	"html"
	"math/rand"
	"strings"
)

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

func PickMessage() string {
	return reminderMessages[rand.Intn(len(reminderMessages))]
}

func Enrich(base string, items []string) string {
	const minItemsForAList = 2
	if len(items) < minItemsForAList {
		return base
	}
	escaped := make([]string, len(items))
	for i, it := range items {
		escaped[i] = html.EscapeString(it)
	}
	last := len(escaped) - 1
	list := strings.Join(escaped[:last], ", ") + " o " + escaped[last]
	return base + "\n\nPor acá suele haber " + list + "."
}
