package health

import "lopiibot.com/internal/database"

type health struct {
	conn *database.Connection
}

func NewHealthChecker(conn *database.Connection) *health {
	return &health{conn: conn}
}

func (c *health) IsHealthy() error {
	conn, err := c.conn.DB.DB()
	if err != nil {
		return err
	}
	return conn.Ping()
}
