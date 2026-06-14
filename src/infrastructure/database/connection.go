package database

import (
	"errors"
	"fmt"

	"github.com/pressly/goose/v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Creds struct {
	Host     string
	Name     string
	Port     uint
	User     string
	Password string
}

type Connection struct {
	db *gorm.DB
}

var conn = &Connection{}

func Initialize(c Creds) (*Connection, error) {
	if conn.db != nil {
		return conn, nil
	}
	dsn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s",
		c.Host, c.Port, c.Name, c.User, c.Password,
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	conn.db = db
	return conn, nil
}

func RunMigrations(migrationsDir string) error {
	if conn == nil || conn.db == nil {
		return errors.New("database not initialized, call Initialize first")
	}
	sqlDB, err := conn.db.DB()
	if err != nil {
		return err
	}
	goose.SetDialect("postgres")
	return goose.Up(sqlDB, migrationsDir)
}
