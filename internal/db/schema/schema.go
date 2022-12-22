package schema

import (
	"database/sql"
	"embed"
	"fmt"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

func Migrate(databaseURL string) error {
	var db *sql.DB
	// setup database
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return fmt.Errorf("error open connection with database: %w", err)
	}

	defer db.Close()

	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("error setting dialect: %w", err)
	}

	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("error trying to run migrations: %w", err)
	}

	return nil
}