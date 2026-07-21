package summary

// UI strings for the weekly summary (Argentine Spanish). Kept together so copy
// edits touch one file.
const (
	// La salida se explica en texto y no con un botón a propósito. Un botón
	// inline de Telegram queda tocable para siempre en el historial, así que un
	// resumen por semana deja un disparador vivo por semana: al año, cincuenta
	// y pico, todos a un toque de apagar la función en silencio. Desactivarlo
	// es algo que se hace una vez en la vida; el hub de recordatorios ya lo
	// hace, y encima muestra el estado real y deja volver a activarlo.
	msgNote = "\n—\n📩 Te mando este resumen cada lunes porque lo activaste. No es tu " +
		"recordatorio diario — son cosas distintas. Si no te suma, escribime " +
		"«no quiero más el resumen semanal» (el recordatorio diario queda intacto)."

	msgEmptyWeek = "📊 Esta semana no registraste movimientos. ¿Arrancamos? 💪" + msgNote
)

var weekdayEs = [...]string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"}
