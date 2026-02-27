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

// MainboardMinimumOwned is the minimum number of copies required for mainboard cards.
const MainboardMinimumOwned = 6

// NonMainboardMinimumOwned is the minimum number of copies required for non-mainboard cards.
const NonMainboardMinimumOwned = 3

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

// RunMigrations creates all required tables if they do not already exist and
// applies any incremental schema changes. It is safe to call multiple times;
// existing tables and columns are not modified or re-created.
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

	if err := database.addColumnIfNotExists("cards", "mainboard", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return fmt.Errorf("add mainboard column: %w", err)
	}

	return nil
}

// addColumnIfNotExists adds a column with the given definition to tableName
// only when the column does not already exist. This provides idempotent
// schema migrations without relying on the ADD COLUMN IF NOT EXISTS syntax
// that older SQLite versions do not support.
func (database *Database) addColumnIfNotExists(tableName, columnName, columnDefinition string) error {
	if tableName == "" {
		return errors.New("table name must not be empty")
	}
	if columnName == "" {
		return errors.New("column name must not be empty")
	}
	if columnDefinition == "" {
		return errors.New("column definition must not be empty")
	}

	rows, err := database.connection.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return fmt.Errorf("query table info: %w", err)
	}

	columnExists := false
	for rows.Next() {
		var (
			cid          int
			name         string
			dataType     string
			notNull      int
			defaultValue interface{}
			primaryKey   int
		)
		if scanErr := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); scanErr != nil {
			rows.Close()
			return fmt.Errorf("scan table info: %w", scanErr)
		}
		if name == columnName {
			columnExists = true
			break
		}
	}

	if closeErr := rows.Close(); closeErr != nil {
		return fmt.Errorf("close table info rows: %w", closeErr)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("table info rows: %w", err)
	}

	if columnExists {
		return nil
	}

	_, err = database.connection.Exec(
		fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, columnDefinition),
	)
	if err != nil {
		return fmt.Errorf("alter table: %w", err)
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

// InsertCard inserts a new card with the given name, optional image path, and
// mainboard flag into the cards table. The owned field is always set to 0 on
// insert. If imagePath is empty, the image column is set to NULL. Returns an
// error if the name is empty or the insert fails.
func (database *Database) InsertCard(name, imagePath string, mainboard bool) error {
	if name == "" {
		return errors.New("card name must not be empty")
	}

	var image sql.NullString
	if imagePath != "" {
		image = sql.NullString{String: imagePath, Valid: true}
	}

	mainboardInt := 0
	if mainboard {
		mainboardInt = 1
	}

	_, err := database.connection.Exec(
		"INSERT INTO cards (name, image, owned, mainboard) VALUES (?, ?, 0, ?)",
		name, image, mainboardInt,
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
	var mainboardInt int

	err := database.connection.QueryRow(
		"SELECT id, name, image, owned, mainboard FROM cards WHERE id = ?",
		id,
	).Scan(&card.ID, &card.Name, &image, &card.Owned, &mainboardInt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCardNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get card by id: %w", err)
	}

	if image.Valid {
		card.Image = image.String
	}

	card.Mainboard = mainboardInt != 0

	return &card, nil
}

// IncrementCardOwned increments the owned count by 1 for the card with the
// given id. Returns ErrCardNotFound if no card with that id exists.
// Returns an error if id is not a positive integer or the update fails.
func (database *Database) IncrementCardOwned(id int) error {
	if id <= 0 {
		return errors.New("card id must be a positive integer")
	}

	result, err := database.connection.Exec(
		"UPDATE cards SET owned = owned + 1 WHERE id = ?",
		id,
	)
	if err != nil {
		return fmt.Errorf("increment card owned: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("increment card owned rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrCardNotFound
	}

	return nil
}

// DecrementCardOwned decrements the owned count by 1 for the card with the
// given id, clamping at 0 so it never goes negative. Returns ErrCardNotFound
// if no card with that id exists. Returns an error if id is not a positive
// integer or the update fails.
func (database *Database) DecrementCardOwned(id int) error {
	if id <= 0 {
		return errors.New("card id must be a positive integer")
	}

	result, err := database.connection.Exec(
		"UPDATE cards SET owned = MAX(owned - 1, 0) WHERE id = ?",
		id,
	)
	if err != nil {
		return fmt.Errorf("decrement card owned: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("decrement card owned rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrCardNotFound
	}

	return nil
}

// SearchCards returns all cards whose name contains query as a substring,
// matched case-insensitively. If query is empty, all cards are returned.
// Returns an empty slice (never nil) when no cards match.
func (database *Database) SearchCards(query string) ([]models.Card, error) {
	var (
		rows *sql.Rows
		err  error
	)

	if query == "" {
		rows, err = database.connection.Query(
			"SELECT id, name, image, owned, mainboard FROM cards",
		)
	} else {
		rows, err = database.connection.Query(
			"SELECT id, name, image, owned, mainboard FROM cards WHERE name LIKE ? COLLATE NOCASE",
			"%"+query+"%",
		)
	}

	if err != nil {
		return nil, fmt.Errorf("search cards: %w", err)
	}
	defer rows.Close()

	result := []models.Card{}

	for rows.Next() {
		var card models.Card
		var image sql.NullString
		var mainboardInt int

		if err := rows.Scan(&card.ID, &card.Name, &image, &card.Owned, &mainboardInt); err != nil {
			return nil, fmt.Errorf("search cards: scan: %w", err)
		}

		if image.Valid {
			card.Image = image.String
		}

		card.Mainboard = mainboardInt != 0

		result = append(result, card)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search cards: rows: %w", err)
	}

	return result, nil
}

// GetWishlistCards returns all cards where the owned count is below the minimum
// threshold: MainboardMinimumOwned for mainboard cards and NonMainboardMinimumOwned
// for non-mainboard cards. An optional name query filters results by a
// case-insensitive substring match. Returns an empty slice (never nil) when no
// cards are below their threshold or when the query matches none.
func (database *Database) GetWishlistCards(query string) ([]models.Card, error) {
	var (
		rows *sql.Rows
		err  error
	)

	if query == "" {
		rows, err = database.connection.Query(
			"SELECT id, name, image, owned, mainboard FROM cards WHERE (mainboard = 1 AND owned < ?) OR (mainboard = 0 AND owned < ?)",
			MainboardMinimumOwned,
			NonMainboardMinimumOwned,
		)
	} else {
		rows, err = database.connection.Query(
			"SELECT id, name, image, owned, mainboard FROM cards WHERE ((mainboard = 1 AND owned < ?) OR (mainboard = 0 AND owned < ?)) AND name LIKE ? COLLATE NOCASE",
			MainboardMinimumOwned,
			NonMainboardMinimumOwned,
			"%"+query+"%",
		)
	}

	if err != nil {
		return nil, fmt.Errorf("get wishlist cards: %w", err)
	}
	defer rows.Close()

	result := []models.Card{}

	for rows.Next() {
		var card models.Card
		var image sql.NullString
		var mainboardInt int

		if err := rows.Scan(&card.ID, &card.Name, &image, &card.Owned, &mainboardInt); err != nil {
			return nil, fmt.Errorf("get wishlist cards: scan: %w", err)
		}

		if image.Valid {
			card.Image = image.String
		}

		card.Mainboard = mainboardInt != 0

		result = append(result, card)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get wishlist cards: rows: %w", err)
	}

	return result, nil
}

// Shutdown closes the database connection. It should be called when the
// application is shutting down to release resources cleanly.
func (database *Database) Shutdown() error {
	if err := database.connection.Close(); err != nil {
		return fmt.Errorf("close sqlite database: %w", err)
	}

	return nil
}
