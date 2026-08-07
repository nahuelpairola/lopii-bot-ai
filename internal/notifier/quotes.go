package notifier

import (
	"context"
	"log/slog"
	"time"

	"lopiibot.com/internal/quote"
)

const (
	// ingestFireMin es a qué minuto ART corre la ingesta, una vez por día.
	// 20:00 porque a esa hora el valor del día ya cerró.
	//
	// A propósito NO se comparte con todayFireMin aunque hoy valgan lo mismo:
	// son dos restricciones independientes. Esta dice cuándo nos conviene
	// correr; la otra, a partir de cuándo el dato del día es definitivo. Si
	// alguien moviera la corrida a las 10:00, todayFireMin tiene que seguir
	// frenando el valor del día — con un solo const se guardaría un precio de
	// media rueda como si fuera el cierre.
	ingestFireMin = 20 * 60
	// retryEvery es el piso entre reintentos cuando la corrida del día falló.
	// Sin reintento, un blip de red a las 20:00 deja el día sin datos hasta
	// mañana; sin piso, una fuente caída son 288 requests contra un error.
	retryEvery = time.Hour
	// maxBackfillDays acota el trabajo de un tick tras una caída larga.
	maxBackfillDays = 30
	// settleWindowDays: cuántos días para atrás se vuelve a pedir al histórico
	// aunque ya haya filas. Es lo que convierte al valor de dolarapi en
	// provisorio: la fuente publica con uno o dos días de atraso, así que en 3
	// días toda fecha ya fue reescrita con el valor de argentinadatos.
	// ponytail: cuesta ~3 requests por tick en régimen. Si molestara, la
	// alternativa es una columna `provisional` y pedir sólo esas fechas.
	settleWindowDays = 3
	// todayFireMin es a partir de qué minuto ART se pide el valor del día en
	// vivo. El oficial cierra 15:00, así que a las 20:00 ya es definitivo.
	todayFireMin = 20 * 60
	// Cuántos años atrás arranca el sembrado. Las fuentes devuelven desde 2011
	// (30k cotizaciones, 1000 meses de IPC) y nadie va a mirar eso: se guarda
	// el año corriente de cotizaciones, y el IPC desde el año pasado porque el
	// deflactor encadena los meses previos al período que se muestra.
	// Es un piso del sembrado, no una retención: lo que entra no se borra.
	seedQuotesYearsBack = 0
	seedCPIYearsBack    = 1
)

// seedSince descarta del sembrado lo anterior a cutoff. Filtra acá y no en el
// cliente: qué historia vale la pena guardar es política del sweeper, el
// cliente sólo sabe traer lo que la fuente publica.
func seedSince[T any](xs []T, cutoff time.Time, at func(T) time.Time) []T {
	out := make([]T, 0, len(xs))
	for _, x := range xs {
		if !at(x).Before(cutoff) {
			out = append(out, x)
		}
	}
	return out
}

// januaryOf devuelve el 1 de enero de hace yearsBack años, en la zona de now.
func januaryOf(now time.Time, yearsBack int) time.Time {
	return time.Date(now.Year()-yearsBack, time.January, 1, 0, 0, 0, 0, now.Location())
}

// dailyRun decide si a este tick le toca correr la ingesta. Devuelve el día de
// hoy, que es la clave con la que el llamador marca la corrida como exitosa.
//
// Tres reglas, en orden:
//  1. Al arrancar el proceso corre una vez, sea la hora que sea. Sin esto, un
//     deploy a las 10 de la mañana deja las tablas vacías hasta las 20:00 y el
//     dashboard en rojo diez horas por nada.
//  2. Después manda el horario: una corrida por día, a partir de ingestFireMin.
//  3. Si la corrida del día no llegó a marcarse (falló), se reintenta con el
//     piso de retryEvery hasta que salga.
//
// La corrida de arranque a propósito NO consume el turno del día: si booteás a
// las 10:00, a las 20:00 igual corre. Es lo que hace que el valor del día en
// vivo no se pierda en un día de deploy.
func (s *Sweeper) dailyRun(now time.Time, booted *bool, ranOn time.Time, lastAttempt *time.Time) (today time.Time, ok bool) {
	today = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if *booted {
		if ranOn.Equal(today) {
			return today, false // ya salió bien hoy
		}
		if now.Hour()*60+now.Minute() < ingestFireMin {
			return today, false // todavía no es la hora
		}
	}
	if now.Sub(*lastAttempt) < retryEvery {
		return today, false // reintento demasiado pronto
	}
	*lastAttempt = now
	*booted = true
	return today, true
}

// ranToday marca la corrida como cumplida, pero sólo si pasó a horario. Una
// corrida de arranque temprana no gasta el turno del día.
func ranToday(now, today time.Time) time.Time {
	if now.Hour()*60+now.Minute() < ingestFireMin {
		return time.Time{}
	}
	return today
}

// sweepQuotes mantiene usd_quotes al día. Pide el histórico hasta ayer y, de
// noche, el valor de hoy en vivo — el histórico no lo tiene hasta mañana.
func (s *Sweeper) sweepQuotes(ctx context.Context, now time.Time) {
	if s.quotes == nil || s.quoteAPI == nil {
		return
	}
	today, ok := s.dailyRun(now, &s.quotesBooted, s.quotesRanOn, &s.lastQuoteAttempt)
	if !ok {
		return
	}

	latest, err := s.quotes.LatestQuoteDate()
	if err != nil {
		slog.ErrorContext(ctx, "notifier quotes latest date failed", "err", err)
		return
	}

	yesterday := today.AddDate(0, 0, -1)

	if latest == nil {
		all, err := s.quoteAPI.FetchAll()
		if err != nil {
			slog.ErrorContext(ctx, "notifier quotes seed failed", "err", err)
			return
		}
		all = seedSince(all, januaryOf(now, seedQuotesYearsBack), func(q quote.Quote) time.Time { return q.Date })
		if err := s.quotes.InsertQuotes(all); err != nil {
			slog.ErrorContext(ctx, "notifier quotes seed insert failed", "err", err)
			return
		}
		latest = &yesterday
	} else {
		// El backfill arranca en el hueco, PERO nunca después de la ventana de
		// asentamiento: los días que escribió dolarapi hay que volver a pedirlos
		// al histórico para que la serie guardada sea de una sola fuente. Sin
		// esto el loop arrancaría en latest+1, que es justo el día siguiente al
		// que escribió dolarapi, y ese valor provisorio quedaría fijo para
		// siempre.
		from := latest.AddDate(0, 0, 1)
		if w := today.AddDate(0, 0, -settleWindowDays); w.Before(from) {
			from = w
		}
		// Se enumera TODA fecha del rango, incluidos sábados y domingos: la
		// fuente arrastra el valor a algunos findes y a otros no, así que
		// saltearlos perdería filas reales. El 404 es el caso normal.
		for d, n := from, 0; !d.After(yesterday) && n < maxBackfillDays; d, n = d.AddDate(0, 0, 1), n+1 {
			qs, err := s.quoteAPI.FetchDate(d)
			if err != nil {
				slog.ErrorContext(ctx, "notifier quotes fetch date failed", "date", d, "err", err)
				continue
			}
			if err := s.quotes.InsertQuotes(qs); err != nil {
				slog.ErrorContext(ctx, "notifier quotes insert failed", "date", d, "err", err)
			}
		}
	}

	// Llegó hasta acá sin cortar: el histórico está al día. Se marca ANTES del
	// valor en vivo a propósito — que dolarapi falle no justifica repetir todo
	// el backfill dentro de una hora.
	s.quotesRanOn = ranToday(now, today)

	if now.Hour()*60+now.Minute() < todayFireMin || !latest.Before(today) {
		return
	}
	qs, err := s.quoteAPI.FetchToday(now)
	if err != nil {
		slog.ErrorContext(ctx, "notifier quotes fetch today failed", "err", err)
		return
	}
	if err := s.quotes.InsertQuotes(qs); err != nil {
		slog.ErrorContext(ctx, "notifier quotes insert today failed", "err", err)
	}
}

// sweepCPI trae la serie entera y la inserta; la PK descarta lo que ya está.
// A propósito no hay aritmética de "mes objetivo" ni chequeo de "ya cargado":
// el IPC de julio, publicado a mediados de agosto, aparece en la respuesta el
// día que existe y entra solo. Nada acá conoce el calendario de INDEC — que es
// justo lo que v1 hacía mal, mirando sólo los días 15 a 20.
func (s *Sweeper) sweepCPI(ctx context.Context, now time.Time) {
	if s.quotes == nil || s.quoteAPI == nil {
		return
	}
	today, ok := s.dailyRun(now, &s.cpiBooted, s.cpiRanOn, &s.lastCPIAttempt)
	if !ok {
		return
	}

	cs, err := s.quoteAPI.FetchCPI()
	if err != nil {
		slog.ErrorContext(ctx, "notifier cpi fetch failed", "err", err)
		return
	}
	cs = seedSince(cs, januaryOf(now, seedCPIYearsBack), func(c quote.CPI) time.Time { return c.Month })
	if err := s.quotes.InsertCPI(cs); err != nil {
		slog.ErrorContext(ctx, "notifier cpi insert failed", "err", err)
		return
	}
	s.cpiRanOn = ranToday(now, today)
}
