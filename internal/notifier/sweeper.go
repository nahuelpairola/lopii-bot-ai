package notifier

import (
	"context"
	"log/slog"
	"time"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/quote"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/user"
)

var artLoc *time.Location

func init() {
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		panic("notifier: cannot load ART location: " + err.Error())
	}
	artLoc = loc
}

type reminderStore interface {
	ListDue(before time.Time) ([]reminder.Reminder, error)
	SetLastRemindedOn(userID uint64, date time.Time) error
	ListWeeklyDue(before time.Time) ([]reminder.Reminder, error)
	SetLastSummaryOn(userID uint64, date time.Time) error
	ListMonthlyDue(before time.Time) ([]uint64, error)
	SetLastMonthlySummaryOn(userID uint64, date time.Time) error
}

type movementReader interface {
	FindRecentlyCreatedForUser(userID uint64, since time.Time, limit int) ([]movement.Movement, error)
	TopRecurringDescriptions(userID uint64, from, to time.Time, minCount, limit int) ([]string, error)
}

type userReader interface {
	FindByID(id uint64) (*user.User, error)
}

type chatResolver interface {
	ChatFor(userID uint64) (messenger.Chat, error)
}

type retentionStore interface {
	DeleteOlderThan(cutoff time.Time) error
}

type summaryReader interface {
	Build(userID uint64, from, to, prevFrom, prevTo time.Time) (string, error)
	BuildMonthly(userID uint64, from, to, prevFrom, prevTo time.Time) (conversation.Prompt, error)
}

type quoteStore interface {
	LatestQuoteDate() (*time.Time, error)
	InsertQuotes([]quote.Quote) error
	InsertCPI([]quote.CPI) error
}

type quoteClient interface {
	FetchAll() ([]quote.Quote, error)
	FetchDate(time.Time) ([]quote.Quote, error)
	FetchToday(time.Time) ([]quote.Quote, error)
	FetchCPI() ([]quote.CPI, error)
}

const retentionDays = 90

type Sweeper struct {
	reminders        reminderStore
	movements        movementReader
	users            userReader
	retention        retentionStore
	summaries        summaryReader
	quotes           quoteStore
	quoteAPI         quoteClient
	quotesBooted     bool
	cpiBooted        bool
	quotesRanOn      time.Time
	cpiRanOn         time.Time
	lastQuoteAttempt time.Time
	lastCPIAttempt   time.Time
	chats            chatResolver
	now              func() time.Time
}

func NewSweeper(chats chatResolver, r reminderStore, m movementReader, u userReader, ret retentionStore, sum summaryReader, q quoteStore, qa quoteClient) *Sweeper {
	return &Sweeper{
		reminders: r,
		movements: m,
		users:     u,
		retention: ret,
		summaries: sum,
		quotes:    q,
		quoteAPI:  qa,
		chats:     chats,
		now:       func() time.Time { return time.Now().In(artLoc) },
	}
}

func (s *Sweeper) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tick(ctx, s.now())
		}
	}
}

func (s *Sweeper) tick(ctx context.Context, now time.Time) {
	s.sweepReminders(ctx, now)
	s.sweepRetention(now)
	sentMonthly := s.sweepMonthlySummary(ctx, now)
	s.sweepWeeklySummary(ctx, now, sentMonthly)
	s.sweepQuotes(ctx, now)
	s.sweepCPI(ctx, now)
}

func (s *Sweeper) sweepRetention(now time.Time) {
	if s.retention == nil {
		return
	}
	if err := s.retention.DeleteOlderThan(now.AddDate(0, 0, -retentionDays)); err != nil {
		slog.Error("notifier retention purge failed", "err", err)
	}
}

func (s *Sweeper) sweepReminders(ctx context.Context, now time.Time) {
	const (
		recurringWindowDays = 30
		recurringMinCount   = 2
		recurringLimit      = 3
	)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	nowMin := now.Hour()*60 + now.Minute()

	due, err := s.reminders.ListDue(startOfDay)
	if err != nil {
		slog.ErrorContext(ctx, "notifier list due failed", "err", err)
		return
	}

	for _, r := range due {
		if nowMin < r.MidpointMin() {
			continue
		}
		moved, err := s.movements.FindRecentlyCreatedForUser(r.UserID, startOfDay, 0)
		if err != nil {
			slog.ErrorContext(ctx, "notifier movements lookup failed", "user_id", r.UserID, "err", err)
			continue
		}
		if len(moved) > 0 {
			continue
		}
		u, err := s.users.FindByID(r.UserID)
		if err != nil {
			slog.ErrorContext(ctx, "notifier user lookup failed", "user_id", r.UserID, "err", err)
			continue
		}
		chat, err := s.chats.ChatFor(u.ID)
		if err != nil {
			slog.ErrorContext(ctx, "notifier chat lookup failed", "user_id", r.UserID, "err", err)
			continue
		}
		text := reminder.PickMessage()
		recurring, err := s.movements.TopRecurringDescriptions(r.UserID, startOfDay.AddDate(0, 0, -recurringWindowDays), startOfDay, recurringMinCount, recurringLimit)
		if err != nil {
			slog.ErrorContext(ctx, "notifier recurring descriptions lookup failed", "user_id", r.UserID, "err", err)
		} else {
			text = reminder.Enrich(text, recurring)
		}
		if err := messenger.SendText(ctx, chat, text); err != nil {
			slog.ErrorContext(ctx, "notifier send failed", "user_id", r.UserID, "err", err)
			continue
		}
		if err := s.reminders.SetLastRemindedOn(r.UserID, startOfDay); err != nil {
			slog.ErrorContext(ctx, "notifier set last reminded failed", "user_id", r.UserID, "err", err)
		}
	}
}

const weeklySummaryFireMin = 9 * 60

func (s *Sweeper) sweepWeeklySummary(ctx context.Context, now time.Time, skip map[uint64]struct{}) {
	if now.Weekday() != time.Monday {
		return
	}
	if now.Hour()*60+now.Minute() < weeklySummaryFireMin {
		return
	}
	thisMonday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	from := thisMonday.AddDate(0, 0, -7)
	to := thisMonday.AddDate(0, 0, -1)
	prevFrom := from.AddDate(0, 0, -7)
	prevTo := from.AddDate(0, 0, -1)

	due, err := s.reminders.ListWeeklyDue(thisMonday)
	if err != nil {
		slog.ErrorContext(ctx, "notifier weekly list due failed", "err", err)
		return
	}

	for _, r := range due {
		if _, done := skip[r.UserID]; done {
			continue
		}
		text, err := s.summaries.Build(r.UserID, from, to, prevFrom, prevTo)
		if err != nil {
			slog.ErrorContext(ctx, "notifier weekly build failed", "user_id", r.UserID, "err", err)
			continue
		}
		u, err := s.users.FindByID(r.UserID)
		if err != nil {
			slog.ErrorContext(ctx, "notifier weekly user lookup failed", "user_id", r.UserID, "err", err)
			continue
		}
		chat, err := s.chats.ChatFor(u.ID)
		if err != nil {
			slog.ErrorContext(ctx, "notifier weekly chat lookup failed", "user_id", r.UserID, "err", err)
			continue
		}
		if err := messenger.SendText(ctx, chat, text); err != nil {
			slog.ErrorContext(ctx, "notifier weekly send failed", "user_id", r.UserID, "err", err)
			continue
		}
		if err := s.reminders.SetLastSummaryOn(r.UserID, thisMonday); err != nil {
			slog.ErrorContext(ctx, "notifier weekly set last summary failed", "user_id", r.UserID, "err", err)
		}
	}
}

const (
	monthlySummaryDay     = 3
	monthlySummaryFireMin = 9 * 60
)

func (s *Sweeper) sweepMonthlySummary(ctx context.Context, now time.Time) map[uint64]struct{} {
	sent := map[uint64]struct{}{}
	if now.Day() != monthlySummaryDay {
		return sent
	}
	if now.Hour()*60+now.Minute() < monthlySummaryFireMin {
		return sent
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	from := firstOfThisMonth.AddDate(0, -1, 0)
	to := firstOfThisMonth.AddDate(0, 0, -1)
	prevFrom := from.AddDate(0, -1, 0)
	prevTo := from.AddDate(0, 0, -1)

	due, err := s.reminders.ListMonthlyDue(today)
	if err != nil {
		slog.ErrorContext(ctx, "notifier monthly list due failed", "err", err)
		return sent
	}

	for _, userID := range due {
		p, err := s.summaries.BuildMonthly(userID, from, to, prevFrom, prevTo)
		if err != nil {
			slog.ErrorContext(ctx, "notifier monthly build failed", "user_id", userID, "err", err)
			continue
		}
		if p.Text == "" {
			if err := s.reminders.SetLastMonthlySummaryOn(userID, today); err != nil {
				slog.ErrorContext(ctx, "notifier monthly set last summary failed", "user_id", userID, "err", err)
			}
			continue
		}
		u, err := s.users.FindByID(userID)
		if err != nil {
			slog.ErrorContext(ctx, "notifier monthly user lookup failed", "user_id", userID, "err", err)
			continue
		}
		chat, err := s.chats.ChatFor(u.ID)
		if err != nil {
			slog.ErrorContext(ctx, "notifier monthly chat lookup failed", "user_id", userID, "err", err)
			continue
		}
		if err := chat.Send(ctx, p); err != nil {
			slog.ErrorContext(ctx, "notifier monthly send failed", "user_id", userID, "err", err)
			continue
		}
		sent[userID] = struct{}{}
		if err := s.reminders.SetLastMonthlySummaryOn(userID, today); err != nil {
			slog.ErrorContext(ctx, "notifier monthly set last summary failed", "user_id", userID, "err", err)
		}
	}
	return sent
}
