package database

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/pressly/goose/v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Creds struct {
	Host     string
	Name     string
	Port     uint
	User     string
	Password string
}

type Connection struct {
	DB *gorm.DB
}

var conn = &Connection{}

// Initialize opens the GORM connection. debug gates GORM's own SQL logger —
// independent of internal/logging — which otherwise prints every slow query
// (default threshold 200ms) with its fully-interpolated values (raw text,
// amounts, descriptions) straight to stdout regardless of the app's configured
// log level. debug=false silences it to real errors only; debug=true (local
// dev) shows every statement.
func Initialize(c Creds, debug bool) (*Connection, error) {
	if conn.DB != nil {
		return conn, nil
	}
	dsn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s",
		c.Host, c.Port, c.Name, c.User, c.Password,
	)
	level := logger.Error
	if debug {
		level = logger.Info
	}
	gormLogger := logger.New(log.New(os.Stdout, "", log.LstdFlags), logger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  level,
		IgnoreRecordNotFoundError: true,
		Colorful:                  debug,
	})
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormLogger})
	if err != nil {
		return nil, err
	}
	conn.DB = db
	return conn, nil
}

func RunMigrations(migrationsDir string) error {
	if conn == nil || conn.DB == nil {
		return errors.New("database not initialized, call Initialize first")
	}
	sqlDB, err := conn.DB.DB()
	if err != nil {
		return err
	}
	goose.SetDialect("postgres")
	return goose.Up(sqlDB, migrationsDir)
}
