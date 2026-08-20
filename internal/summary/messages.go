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
	//
	// El paréntesis del recordatorio diario se queda aunque la nota se haya
	// acortado: sin él, "cortalo" se lee como cortar TODO lo que manda el bot.
	msgNote = "\n<blockquote>📩 Va cada lunes (no es tu recordatorio diario). Para cortarlo, " +
		"escribime «no quiero más el resumen semanal».</blockquote>"

	msgEmptyWeek = "📊 Esta semana no registraste movimientos. ¿Arrancamos? 💪" + msgNote
)
