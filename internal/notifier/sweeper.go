package notifier

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/movement"
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
}

type movementReader interface {
	FindRecentlyCreatedForUser(userID uint64, since time.Time) ([]movement.Movement, error)
}

type userReader interface {
	FindByID(id uint64) (*user.User, error)
}

// Sweeper drives all scheduled system->user notifications. Today it hosts one
// tenant (reminders); future tenants add a sibling sweepX call in tick(). send
// is injected so it is reachable by tests and, later, an admin broadcast — the
// single reused asset. now is injected for deterministic tests.
type Sweeper struct {
	reminders reminderStore
	movements movementReader
	users     userReader
	send      func(ctx context.Context, chatID int64, text string) error
	now       func() time.Time
}

func NewSweeper(b *bot.Bot, r reminderStore, m movementReader, u userReader) *Sweeper {
	return &Sweeper{
		reminders: r,
		movements: m,
		users:     u,
		send: func(ctx context.Context, chatID int64, text string) error {
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text})
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
	// future tenants:
	// s.sweepCafecito(ctx, now)
	// s.sweepWeeklySummary(ctx, now)
}

// sweepReminders sends a nudge to any enabled user past their window midpoint
// who has logged nothing today and hasn't been reminded today.
func (s *Sweeper) sweepReminders(ctx context.Context, now time.Time) {
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	nowMin := now.Hour()*60 + now.Minute()

	due, err := s.reminders.ListDue(startOfDay)
	if err != nil {
		log.Printf("notifier: ListDue: %v", err)
		return
	}

	for _, r := range due {
		if nowMin < r.MidpointMin() {
			continue // not time yet
		}
		moved, err := s.movements.FindRecentlyCreatedForUser(r.UserID, startOfDay)
		if err != nil {
			log.Printf("notifier: movements for user %d: %v", r.UserID, err)
			continue
		}
		if len(moved) > 0 {
			continue // already engaged today
		}
		u, err := s.users.FindByID(r.UserID)
		if err != nil {
			log.Printf("notifier: user %d: %v", r.UserID, err)
			continue
		}
		chatID, err := strconv.ParseInt(u.TelegramID, 10, 64)
		if err != nil {
			log.Printf("notifier: bad telegram_id %q for user %d: %v", u.TelegramID, r.UserID, err)
			continue
		}
		if err := s.send(ctx, chatID, reminder.PickMessage()); err != nil {
			log.Printf("notifier: send to user %d: %v", r.UserID, err)
			continue
		}
		if err := s.reminders.SetLastRemindedOn(r.UserID, startOfDay); err != nil {
			log.Printf("notifier: SetLastRemindedOn user %d: %v", r.UserID, err)
		}
	}
}
