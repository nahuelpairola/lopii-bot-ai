package registry

import (
	"fmt"

	"lopiibot.com/src/infrastructure/database"
)

func (r *registry) initDatabase() (*database.Connection, error) {
	conn, err := database.Initialize(database.Creds{
		Host:     r.config.Database.Host,
		Name:     r.config.Database.Name,
		Port:     r.config.Database.Port,
		User:     r.config.Database.User,
		Password: r.config.Database.Password,
	})
	if err != nil {
		return nil, err
	}
	if r.config.Database.RunMigrations {
		if err = database.RunMigrations("./migrations"); err != nil {
			fmt.Printf("Migrations error: %v", err.Error())
			return nil, err
		}
	}
	return conn, err
}
