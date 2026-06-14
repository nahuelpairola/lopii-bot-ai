package healthrepository

import "lopiibot.com/src/infrastructure/database"

type healthRepository struct {
	db *database.Connection
}

func NewHealthRepository(db *database.Connection) *healthRepository {
	return &healthRepository{
		db: db,
	}
}

func (r *healthRepository) IsHealthy() error {
	sql, err := r.db.DB.DB()
	if err != nil {
		return err
	}
	return sql.Ping()
}
