package conversation

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"lopiibot.com/internal/database"
)

// state es el registro persistido en conversation_states. flow_name
// identifica qué Flow está corriendo, step_name en qué paso está, y data
// es el JSON serializado del Data acumulado del flujo.
type state struct {
	UserID    uint64    `gorm:"column:user_id;primaryKey"`
	FlowName  string    `gorm:"column:flow_name"`
	StepName  string    `gorm:"column:step_name"`
	Data      []byte    `gorm:"column:data;type:jsonb"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (state) TableName() string {
	return "conversation_states"
}

// repository implementa StateStore contra Postgres vía GORM.
type repository struct {
	conn *database.Connection
}

func NewRepository(conn *database.Connection) *repository {
	return &repository{conn: conn}
}

func (r *repository) Get(userID uint64) (flowName, stepName string, data Data, found bool, err error) {
	var s state
	err = r.conn.DB.First(&s, "user_id = ?", userID).Error
	if err == gorm.ErrRecordNotFound {
		return "", "", nil, false, nil
	}
	if err != nil {
		return "", "", nil, false, err
	}

	var d Data
	if err := json.Unmarshal(s.Data, &d); err != nil {
		return "", "", nil, false, err
	}
	return s.FlowName, s.StepName, d, true, nil
}

func (r *repository) Set(userID uint64, flowName, stepName string, data Data) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	s := state{
		UserID:    userID,
		FlowName:  flowName,
		StepName:  stepName,
		Data:      payload,
		UpdatedAt: time.Now(),
	}
	return r.conn.DB.Save(&s).Error
}

func (r *repository) Clear(userID uint64) error {
	return r.conn.DB.Delete(&state{}, "user_id = ?", userID).Error
}
