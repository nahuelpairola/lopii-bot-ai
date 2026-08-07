package notifier

import (
	"context"
	"log/slog"
	"time"

	"lopiibot.com/internal/quote"
)

const (
	// quoteAttemptEvery acota los intentos: en régimen es 1 request por día.
	quoteAttemptEvery = time.Hour
	// maxBackfillDays acota el trabajo de un tick tras una caída larga.
	maxBackfillDays = 30
	// todayFireMin es a partir de qué minuto ART se pide el valor del día en
	// vivo. El oficial cierra 15:00, así que a las 20:00 ya es definitivo.
	todayFireMin = 20 * 60
	// cpiAttemptEvery: el IPC cambia una vez al mes, mirarlo una vez al día es
	// de sobra. Son 53 KB, ~19 MB al año.
	cpiAttemptEvery = 24 * time.Hour
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

// sweepQuotes mantiene usd_quotes al día. Pide el histórico hasta ayer y, de
// noche, el valor de hoy en vivo — el histórico no lo tiene hasta mañana.
func (s *Sweeper) sweepQuotes(ctx context.Context, now time.Time) {
	if s.quotes == nil || s.quoteAPI == nil {
		return
	}
	if now.Sub(s.lastQuoteAttempt) < quoteAttemptEvery {
		return
	}
	s.lastQuoteAttempt = now

	latest, err := s.quotes.LatestQuoteDate()
	if err != nil {
		slog.ErrorContext(ctx, "notifier quotes latest date failed", "err", err)
		return
	}

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
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
		// Se enumera TODA fecha del hueco, incluidos sábados y domingos: la
		// fuente arrastra el valor a algunos findes y a otros no, así que
		// saltearlos perdería filas reales. El 404 es el caso normal.
		for d, n := latest.AddDate(0, 0, 1), 0; !d.After(yesterday) && n < maxBackfillDays; d, n = d.AddDate(0, 0, 1), n+1 {
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
	if now.Sub(s.lastCPIAttempt) < cpiAttemptEvery {
		return
	}
	s.lastCPIAttempt = now

	cs, err := s.quoteAPI.FetchCPI()
	if err != nil {
		slog.ErrorContext(ctx, "notifier cpi fetch failed", "err", err)
		return
	}
	cs = seedSince(cs, januaryOf(now, seedCPIYearsBack), func(c quote.CPI) time.Time { return c.Month })
	if err := s.quotes.InsertCPI(cs); err != nil {
		slog.ErrorContext(ctx, "notifier cpi insert failed", "err", err)
	}
}
