package notifier

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
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
}

type movementReader interface {
	FindRecentlyCreatedForUser(userID uint64, since time.Time, limit int) ([]movement.Movement, error)
}

type userReader interface {
	FindByID(id uint64) (*user.User, error)
}

type retentionStore interface {
	DeleteOlderThan(cutoff time.Time) error
}

type summaryReader interface {
	Build(userID uint64, from, to, prevFrom, prevTo time.Time) (string, error)
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

// retentionDays es cuánto se conservan las tablas operativas (llm_calls,
// request_traces) antes de purgarse. Va en Go, no pg_cron.
const retentionDays = 90

// Sweeper drives all scheduled system->user notifications. Today it hosts one
// tenant (reminders); future tenants add a sibling sweepX call in tick(). send
// is injected so it is reachable by tests and, later, an admin broadcast — the
// single reused asset. now is injected for deterministic tests.
type Sweeper struct {
	reminders reminderStore
	movements movementReader
	users     userReader
	retention retentionStore
	summaries summaryReader
	quotes    quoteStore
	quoteAPI  quoteClient
	// El estado del scheduler de la ingesta, todo en memoria. Se reinicia al
	// arrancar el proceso, y eso está bien: toda escritura es idempotente por
	// PK, así que una corrida de más no cuesta nada. Ver dailyRun.
	//
	//   *Booted — ya corrió una vez en este proceso.
	//   *RanOn  — el día en que la corrida a horario salió bien.
	//   last*Attempt — cuándo se intentó por última vez, el piso del reintento.
	quotesBooted     bool
	cpiBooted        bool
	quotesRanOn      time.Time
	cpiRanOn         time.Time
	lastQuoteAttempt time.Time
	lastCPIAttempt   time.Time
	send             func(ctx context.Context, chatID int64, text string, markup *models.InlineKeyboardMarkup) error
	now              func() time.Time
}

func NewSweeper(b *bot.Bot, r reminderStore, m movementReader, u userReader, ret retentionStore, sum summaryReader, q quoteStore, qa quoteClient) *Sweeper {
	return &Sweeper{
		reminders: r,
		movements: m,
		users:     u,
		retention: ret,
		summaries: sum,
		quotes:    q,
		quoteAPI:  qa,
		send: func(ctx context.Context, chatID int64, text string, markup *models.InlineKeyboardMarkup) error {
			// ParseMode HTML: el resumen semanal usa <b> para que se pueda
			// escanear. Todo lo que viene del usuario se escapa en
			// summary.Builder — sin eso Telegram devuelve 400 y no llega nada.
			p := &bot.SendMessageParams{ChatID: chatID, Text: text, ParseMode: models.ParseModeHTML}
			if markup != nil {
				p.ReplyMarkup = markup
			}
			_, err := b.SendMessage(ctx, p)
			return err
		},
		now: func() time.Time { return time.Now().In(artLoc) },
	}
}

// Run ticks every interval until ctx is cancelled.
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

// tick runs every notifier for this instant. now is in ART.
func (s *Sweeper) tick(ctx context.Context, now time.Time) {
	s.sweepReminders(ctx, now)
	s.sweepRetention(now)
	s.sweepWeeklySummary(ctx, now)
	// Van últimos: el tick que siembra baja 2.9 MB una sola vez en la vida del
	// deploy, y los sweeps de arriba están gateados por ventana de minuto-del-
	// día, no por instante exacto. Unos segundos no les cuestan nada.
	s.sweepQuotes(ctx, now)
	s.sweepCPI(ctx, now)
	// future tenants:
	// s.sweepCafecito(ctx, now)
}

// sweepRetention purga métricas operativas más viejas que retentionDays. Corre
// cada tick: el DELETE es idempotente y barato (índice created_at), casi siempre
// 0 filas. ponytail: si el tick fuera caro, gatear a 1/día por la hora.
func (s *Sweeper) sweepRetention(now time.Time) {
	if s.retention == nil {
		return
	}
	if err := s.retention.DeleteOlderThan(now.AddDate(0, 0, -retentionDays)); err != nil {
		slog.Error("notifier retention purge failed", "err", err)
	}
}

// sweepReminders sends a nudge to any enabled user past their window midpoint
// who has logged nothing today and hasn't been reminded today.
func (s *Sweeper) sweepReminders(ctx context.Context, now time.Time) {
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	nowMin := now.Hour()*60 + now.Minute()

	due, err := s.reminders.ListDue(startOfDay)
	if err != nil {
		slog.ErrorContext(ctx, "notifier list due failed", "err", err)
		return
	}

	for _, r := range due {
		if nowMin < r.MidpointMin() {
			continue // not time yet
		}
		moved, err := s.movements.FindRecentlyCreatedForUser(r.UserID, startOfDay, 0)
		if err != nil {
			slog.ErrorContext(ctx, "notifier movements lookup failed", "user_id", r.UserID, "err", err)
			continue
		}
		if len(moved) > 0 {
			continue // already engaged today
		}
		u, err := s.users.FindByID(r.UserID)
		if err != nil {
			slog.ErrorContext(ctx, "notifier user lookup failed", "user_id", r.UserID, "err", err)
			continue
		}
		chatID, err := strconv.ParseInt(u.TelegramID, 10, 64)
		if err != nil {
			slog.ErrorContext(ctx, "notifier bad telegram_id", "user_id", r.UserID, "err", err)
			continue
		}
		if err := s.send(ctx, chatID, reminder.PickMessage(), nil); err != nil {
			slog.ErrorContext(ctx, "notifier send failed", "user_id", r.UserID, "err", err)
			continue
		}
		if err := s.reminders.SetLastRemindedOn(r.UserID, startOfDay); err != nil {
			slog.ErrorContext(ctx, "notifier set last reminded failed", "user_id", r.UserID, "err", err)
		}
	}
}

// weeklySummaryFireMin is the ART minute-of-day the Monday summary fires at (09:00).
const weeklySummaryFireMin = 9 * 60

// sweepWeeklySummary sends the previous-week (Mon–Sun) summary to opted-in users,
// once per week, on Mondays at/after 09:00 ART. Idempotent via last_summary_on.
func (s *Sweeper) sweepWeeklySummary(ctx context.Context, now time.Time) {
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
		chatID, err := strconv.ParseInt(u.TelegramID, 10, 64)
		if err != nil {
			slog.ErrorContext(ctx, "notifier weekly bad telegram_id", "user_id", r.UserID, "err", err)
			continue
		}
		if err := s.send(ctx, chatID, text, nil); err != nil {
			slog.ErrorContext(ctx, "notifier weekly send failed", "user_id", r.UserID, "err", err)
			continue
		}
		if err := s.reminders.SetLastSummaryOn(r.UserID, thisMonday); err != nil {
			slog.ErrorContext(ctx, "notifier weekly set last summary failed", "user_id", r.UserID, "err", err)
		}
	}
}
