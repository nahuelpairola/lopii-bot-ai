package chathistory

import (
	"time"

	"lopiibot.com/internal/database"
)

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

type Turn struct {
	Question string
	Answer   string
}

type repository struct {
	db    *database.Connection
	ttl   time.Duration
	limit int
}

func InitRepository(conn *database.Connection, ttl time.Duration, limit int) *repository {
	return &repository{db: conn, ttl: ttl, limit: limit}
}

func (r *repository) Append(userID uint64, question, answer string) error {
	r.db.DB.Where("user_id = ? AND created_at < ?", userID, time.Now().Add(-r.ttl)).
		Delete(&ChatTurn{})
	return r.db.DB.Create(&ChatTurn{
		UserID:   userID,
		Question: question,
		Answer:   answer,
	}).Error
}

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
		turns[len(rows)-1-i] = Turn{Question: row.Question, Answer: row.Answer}
	}
	return turns, nil
}
