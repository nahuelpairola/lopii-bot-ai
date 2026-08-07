package quote

import (
	"time"

	"gorm.io/gorm/clause"
	"lopiibot.com/internal/database"
)

// insertBatchSize acota el INSERT del sembrado: la carga inicial son ~15 años
// de historia por 7 casas, demasiadas filas para una sola sentencia.
const insertBatchSize = 1000

type repository struct{ conn *database.Connection }

func NewRepository(conn *database.Connection) *repository { return &repository{conn: conn} }

// LatestQuoteDate devuelve la fecha más nueva guardada, o nil si la tabla está
// vacía. Es lo único que el sweeper necesita para saber qué pedir.
func (r *repository) LatestQuoteDate() (*time.Time, error) {
	var dates []time.Time
	err := r.conn.DB.Model(&Quote{}).
		Order("date DESC").Limit(1).
		Pluck("date", &dates).Error
	if err != nil || len(dates) == 0 {
		return nil, err
	}
	return &dates[0], nil
}

// InsertQuotes inserta en lote, ignorando lo que ya está. DoNothing y no
// DoUpdates a propósito: el valor del día en curso lo escribe dolarapi y el
// histórico no debe pisarlo al día siguiente.
func (r *repository) InsertQuotes(qs []Quote) error {
	if len(qs) == 0 {
		return nil
	}
	return r.conn.DB.Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(qs, insertBatchSize).Error
}

// InsertCPI inserta la lista entera cada vez. Son 53 KB y una sentencia por
// día: calcular qué meses faltan cuesta más que la query que lo calcula.
func (r *repository) InsertCPI(cs []CPI) error {
	if len(cs) == 0 {
		return nil
	}
	return r.conn.DB.Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(cs, insertBatchSize).Error
}
