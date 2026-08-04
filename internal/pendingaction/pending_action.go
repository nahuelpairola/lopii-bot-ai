// Package pendingaction guarda las acciones que el agent loop no pudo
// resolver en el turno: le falta un dato que sólo el usuario tiene.
//
// Es una cola durable por usuario, hermana de internal/pendingjob — pero
// pendingjob cachea un MENSAJE que no se pudo procesar (429 de Groq), y esto
// guarda una ACCIÓN ya interpretada a la que le falta una respuesta. Se drena
// de a una: mientras hay una abierta, no se abre otra.
package pendingaction

import "time"

// OpenQuestion es un dato faltante y la pregunta que lo consigue.
//
// Key dice DÓNDE va la respuesta dentro del Payload de la acción (p.ej.
// "row0.category"); el paquete que parkeó la acción es el que la interpreta.
// Options son botones que aceleran — nunca encierran: el texto libre siempre
// se acepta, y ésa es justamente la salida del callejón donde el picker viejo
// sólo ofrecía lo que ya existía.
type OpenQuestion struct {
	Key     string   `json:"key"`
	Prompt  string   `json:"prompt"`
	Options []string `json:"options,omitempty"`
	Answer  string   `json:"answer,omitempty"`
}

// PendingAction es una fila de pending_actions.
//
// Budget se guarda, no se calcula al drenar: sale de la cantidad de preguntas
// abiertas EN EL MOMENTO DE PARKEAR. Recalcularlo después dejaría que una
// lista de preguntas que crece se suba su propio techo.
type PendingAction struct {
	ID        uint64    `gorm:"primaryKey"`
	UserID    uint64    `gorm:"column:user_id;not null"`
	Tool      string    `gorm:"column:tool;not null"`
	Payload   []byte    `gorm:"column:payload;type:jsonb"`
	Questions []byte    `gorm:"column:questions;type:jsonb"`
	Budget    int       `gorm:"column:budget;not null"`
	Position  int       `gorm:"column:position;not null"`
	TraceID   string    `gorm:"column:trace_id"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (PendingAction) TableName() string {
	return "pending_actions"
}
