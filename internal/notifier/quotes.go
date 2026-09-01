package notifier

import (
	"context"
	"log/slog"
	"time"

	"lopiibot.com/internal/quote"
)

const (
	ingestFireMin       = 20 * 60
	retryEvery          = time.Hour
	maxBackfillDays     = 30
	settleWindowDays    = 3
	todayFireMin        = 20 * 60
	seedQuotesYearsBack = 0
	seedCPIYearsBack    = 1
)

func seedSince[T any](xs []T, cutoff time.Time, at func(T) time.Time) []T {
	out := make([]T, 0, len(xs))
	for _, x := range xs {
		if !at(x).Before(cutoff) {
			out = append(out, x)
		}
	}
	return out
}

func januaryOf(now time.Time, yearsBack int) time.Time {
	return time.Date(now.Year()-yearsBack, time.January, 1, 0, 0, 0, 0, now.Location())
}

func (s *Sweeper) dailyRun(now time.Time, booted *bool, ranOn time.Time, lastAttempt *time.Time) (today time.Time, ok bool) {
	today = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if *booted {
		if ranOn.Equal(today) {
			return today, false
		}
		if now.Hour()*60+now.Minute() < ingestFireMin {
			return today, false
		}
	}
	if now.Sub(*lastAttempt) < retryEvery {
		return today, false
	}
	*lastAttempt = now
	*booted = true
	return today, true
}

func ranToday(now, today time.Time) time.Time {
	if now.Hour()*60+now.Minute() < ingestFireMin {
		return time.Time{}
	}
	return today
}

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
		from := latest.AddDate(0, 0, 1)
		if w := today.AddDate(0, 0, -settleWindowDays); w.Before(from) {
			from = w
		}
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
