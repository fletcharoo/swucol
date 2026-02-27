package database_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"swucol/database"
)

// newTestDatabase creates a Database backed by a temporary file that is
// cleaned up automatically when the test ends.
func newTestDatabase(t *testing.T) *database.Database {
	t.Helper()

	filePath := filepath.Join(t.TempDir(), "test.db")

	db, err := database.New(filePath)
	require.NoError(t, err, "expected no error opening test database")

	t.Cleanup(func() {
		db.Shutdown()
	})

	return db
}

func TestNew_EmptyFilePath_ReturnsError(t *testing.T) {
	db, err := database.New("")

	assert.Nil(t, db)
	assert.ErrorContains(t, err, "must not be empty")
}

func TestNew_ValidFilePath_OpensSuccessfully(t *testing.T) {
	db := newTestDatabase(t)

	assert.NotNil(t, db)
}

func TestRunMigrations_CreatesCardsTable(t *testing.T) {
	db := newTestDatabase(t)

	err := db.RunMigrations()
	require.NoError(t, err, "expected migrations to run without error")

	// Query sqlite_master to confirm the cards table exists.
	row := db.Connection().QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='cards'",
	)

	var tableName string
	err = row.Scan(&tableName)
	require.NoError(t, err, "expected cards table to exist in database")
	assert.Equal(t, "cards", tableName)
}

func TestRunMigrations_CardsTableHasCorrectColumns(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	rows, err := db.Connection().Query("PRAGMA table_info(cards)")
	require.NoError(t, err)
	defer rows.Close()

	type columnInfo struct {
		name     string
		dataType string
		notNull  bool
	}

	columns := map[string]columnInfo{}
	for rows.Next() {
		var (
			cid          int
			name         string
			dataType     string
			notNull      int
			defaultValue interface{}
			primaryKey   int
		)
		require.NoError(t, rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey))
		columns[name] = columnInfo{name: name, dataType: dataType, notNull: notNull == 1}
	}
	require.NoError(t, rows.Err())

	assert.Contains(t, columns, "id")
	assert.Contains(t, columns, "name")
	assert.Contains(t, columns, "image")
	assert.Contains(t, columns, "owned")

	assert.Equal(t, "INTEGER", columns["id"].dataType)
	assert.Equal(t, "TEXT", columns["name"].dataType)
	assert.True(t, columns["name"].notNull, "name column should be NOT NULL")
	assert.Equal(t, "TEXT", columns["image"].dataType)
	assert.Equal(t, "INTEGER", columns["owned"].dataType)
	assert.True(t, columns["owned"].notNull, "owned column should be NOT NULL")
}

func TestRunMigrations_IsIdempotent(t *testing.T) {
	db := newTestDatabase(t)

	require.NoError(t, db.RunMigrations())
	require.NoError(t, db.RunMigrations(), "running migrations a second time should not error")
}

func TestCardsTable_InsertAndQuery(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	// Insert a card.
	_, err := db.Connection().Exec(
		"INSERT INTO cards (name, image, owned) VALUES (?, ?, ?)",
		"Luke Skywalker", "https://example.com/luke.jpg", 2,
	)
	require.NoError(t, err, "expected insert to succeed")

	// Query it back.
	row := db.Connection().QueryRow("SELECT name, image, owned FROM cards WHERE name = ?", "Luke Skywalker")

	var name, image string
	var owned int
	require.NoError(t, row.Scan(&name, &image, &owned))

	assert.Equal(t, "Luke Skywalker", name)
	assert.Equal(t, "https://example.com/luke.jpg", image)
	assert.Equal(t, 2, owned)
}

func TestCardsTable_NameIsRequired(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	_, err := db.Connection().Exec(
		"INSERT INTO cards (name, image, owned) VALUES (?, ?, ?)",
		nil, "https://example.com/image.jpg", 1,
	)

	assert.Error(t, err, "expected error when inserting card with NULL name")
}
