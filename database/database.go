// Package database provides SQLite database initialization and schema management.
package database

import (
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite" // Register the SQLite driver.
)

// Database wraps a sql.DB connection and provides schema management.
type Database struct {
	connection *sql.DB
}

// New opens (or creates) a SQLite database file at the given filePath and
// returns a Database instance. Returns an error if the path is empty or the
// connection cannot be established.
func New(filePath string) (*Database, error) {
	if filePath == "" {
		return nil, errors.New("database file path must not be empty")
	}

	connection, err := sql.Open("sqlite", filePath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	if err := connection.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}

	return &Database{connection: connection}, nil
}

// RunMigrations creates all required tables if they do not already exist.
// It is safe to call multiple times; existing tables are not modified.
func (database *Database) RunMigrations() error {
	createCardsTable := `
		CREATE TABLE IF NOT EXISTS cards (
			id    INTEGER PRIMARY KEY AUTOINCREMENT,
			name  TEXT    NOT NULL,
			image TEXT,
			owned INTEGER NOT NULL DEFAULT 0
		);
	`

	if _, err := database.connection.Exec(createCardsTable); err != nil {
		return fmt.Errorf("create cards table: %w", err)
	}

	return nil
}

// Connection returns the underlying *sql.DB so that other packages can
// execute queries against the database.
func (database *Database) Connection() *sql.DB {
	return database.connection
}

// Shutdown closes the database connection. It should be called when the
// application is shutting down to release resources cleanly.
func (database *Database) Shutdown() error {
	if err := database.connection.Close(); err != nil {
		return fmt.Errorf("close sqlite database: %w", err)
	}

	return nil
}
