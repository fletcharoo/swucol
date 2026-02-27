// Package database provides SQLite database initialization and schema management.
package database

import (
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite" // Register the SQLite driver.

	"swucol/models"
)

// ErrCardNotFound is returned by GetCardByID when no card with the given ID exists.
var ErrCardNotFound = errors.New("card not found")

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

// CardExistsByName returns true if a card with the given name already exists
// in the cards table. Returns an error if the name is empty or the query fails.
func (database *Database) CardExistsByName(name string) (bool, error) {
	if name == "" {
		return false, errors.New("card name must not be empty")
	}

	var count int
	err := database.connection.QueryRow(
		"SELECT COUNT(*) FROM cards WHERE name = ?",
		name,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check card exists by name: %w", err)
	}

	return count > 0, nil
}

// InsertCard inserts a new card with the given name into the cards table.
// The owned field is always set to 0 on insert. Returns an error if the name
// is empty or the insert fails.
func (database *Database) InsertCard(name string) error {
	if name == "" {
		return errors.New("card name must not be empty")
	}

	_, err := database.connection.Exec(
		"INSERT INTO cards (name, owned) VALUES (?, 0)",
		name,
	)
	if err != nil {
		return fmt.Errorf("insert card: %w", err)
	}

	return nil
}

// GetCardByID retrieves the card with the given id from the cards table.
// Returns ErrCardNotFound if no card with that id exists.
// Returns an error if id is not a positive integer or the query fails.
func (database *Database) GetCardByID(id int) (*models.Card, error) {
	if id <= 0 {
		return nil, errors.New("card id must be a positive integer")
	}

	var card models.Card
	var image sql.NullString

	err := database.connection.QueryRow(
		"SELECT id, name, image, owned FROM cards WHERE id = ?",
		id,
	).Scan(&card.ID, &card.Name, &image, &card.Owned)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCardNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get card by id: %w", err)
	}

	if image.Valid {
		card.Image = image.String
	}

	return &card, nil
}

// Shutdown closes the database connection. It should be called when the
// application is shutting down to release resources cleanly.
func (database *Database) Shutdown() error {
	if err := database.connection.Close(); err != nil {
		return fmt.Errorf("close sqlite database: %w", err)
	}

	return nil
}
