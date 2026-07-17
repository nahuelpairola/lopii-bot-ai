package summary

// UI strings for the weekly summary (Argentine Spanish). Kept together so copy
// edits touch one file.
const (
	msgNote = "\n—\n📩 Te mando este resumen cada lunes porque lo activaste. No es tu " +
		"recordatorio diario — son cosas distintas. Si no te suma, tocá 🔕 y no te llega " +
		"más (el recordatorio diario queda intacto)."

	msgEmptyWeek = "📊 Esta semana no registraste movimientos. ¿Arrancamos? 💪" + msgNote
)

var weekdayEs = [...]string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"}
