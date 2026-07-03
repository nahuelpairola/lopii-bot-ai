package subcategory

// CategoryIcon mapea cada categoría (las 14 sembradas en
// migrations/20260625234857_seed_subcategories_table.sql más las dos
// pseudo-categorías reservadas) a un emoji, para los mensajes de
// confirmación/corrección. Mapa estático a propósito: la taxonomía es
// cerrada y solo el admin agrega categorías globales, así que esto se
// actualiza a mano el día que aparezca una nueva — mismo lugar donde ya
// se actualizan los mensajes de este paquete.
var CategoryIcon = map[string]string{
	"Alimentación":   "🍔",
	"Vivienda":       "🏠",
	"Transporte":     "🚗",
	"Salud":          "🩺",
	"Ocio y salidas": "🎉",
	"Bienestar":      "🧘",
	"Indumentaria":   "👕",
	"Tecnología":     "💻",
	"Suscripciones":  "🔁",
	"Educación":      "📚",
	"Inversiones":    "📈",
	"Otros":          "🗂️",
	"Finanzas":       "🏦",
	"Ingresos":       "💵",
	"PENDING_REVIEW": "⏳",
	"Sistema":        "⚙️",
}

// IconFor devuelve el emoji de una categoría, o el genérico 📂 si no
// está mapeada (ej. una categoría creada por el usuario).
func IconFor(category string) string {
	if icon, ok := CategoryIcon[category]; ok {
		return icon
	}
	return "📂"
}
