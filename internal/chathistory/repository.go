package chathistory

import (
	"time"

	"lopiibot.com/internal/database"
)

// ChatTurn is one answered turn: the user's message and the bot's reply.
// It used to be QUERY-only, which is what the package was named after; from
// stage 2 of the agent loop every intent shares the same thread, so the old
// name lied.
//
// Ephemeral by design — pruned, never soft-deleted (a turn carries no
// accounting value and must actually disappear so a stale conversation can
// never be re-read). CreatedAt is autopopulated by GORM (autoCreateTime).
type ChatTurn struct {
	ID        uint64    `gorm:"primaryKey"`
	UserID    uint64    `gorm:"column:user_id;not null"`
	Question  string    `gorm:"column:question;not null"`
	Answer    string    `gorm:"column:answer;not null"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (ChatTurn) TableName() string {
	return "chat_turns"
}

// Turn is the read shape the loop needs — just the two strings, no DB metadata.
type Turn struct {
	Question string
	Answer   string
}

type repository struct {
	db    *database.Connection
	ttl   time.Duration
	limit int
}

// InitRepository builds the repo with the freshness window (ttl) and the
// max turns returned by Recent (limit), both injected from config.
func InitRepository(conn *database.Connection, ttl time.Duration, limit int) *repository {
	return &repository{db: conn, ttl: ttl, limit: limit}
}

// Append inserts the new turn and, in the same call, deletes this user's
// turns older than the TTL — lazy self-cleanup, no cron. Because Recent only
// ever reads within the TTL window, stale rows are already invisible; this
// prune only bounds table growth.
// ponytail: two statements, not a tx — a lost prune at worst leaves a few
// dead rows Recent already ignores; never user data.
func (r *repository) Append(userID uint64, question, answer string) error {
	r.db.DB.Where("user_id = ? AND created_at < ?", userID, time.Now().Add(-r.ttl)).
		Delete(&ChatTurn{})
	return r.db.DB.Create(&ChatTurn{
		UserID:   userID,
		Question: question,
		Answer:   answer,
	}).Error
}

// Recent returns up to `limit` turns from the last `ttl`, oldest first, so the
// caller appends them to the message list in reading order. The query orders
// created_at DESC + LIMIT to grab the newest N, then reverses to chronological.
func (r *repository) Recent(userID uint64) ([]Turn, error) {
	var rows []ChatTurn
	err := r.db.DB.
		Where("user_id = ? AND created_at > ?", userID, time.Now().Add(-r.ttl)).
		Order("created_at DESC").
		Limit(r.limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	turns := make([]Turn, len(rows))
	for i, row := range rows {
		// reverse: newest-first rows → oldest-first turns
		turns[len(rows)-1-i] = Turn{Question: row.Question, Answer: row.Answer}
	}
	return turns, nil
}
