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
	assert.Contains(t, columns, "mainboard")

	assert.Equal(t, "INTEGER", columns["id"].dataType)
	assert.Equal(t, "TEXT", columns["name"].dataType)
	assert.True(t, columns["name"].notNull, "name column should be NOT NULL")
	assert.Equal(t, "TEXT", columns["image"].dataType)
	assert.Equal(t, "INTEGER", columns["owned"].dataType)
	assert.True(t, columns["owned"].notNull, "owned column should be NOT NULL")
	assert.Equal(t, "INTEGER", columns["mainboard"].dataType)
	assert.True(t, columns["mainboard"].notNull, "mainboard column should be NOT NULL")
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
		"INSERT INTO cards (name, image, owned, mainboard) VALUES (?, ?, ?, ?)",
		"Luke Skywalker", "https://example.com/luke.jpg", 2, 1,
	)
	require.NoError(t, err, "expected insert to succeed")

	// Query it back.
	row := db.Connection().QueryRow("SELECT name, image, owned, mainboard FROM cards WHERE name = ?", "Luke Skywalker")

	var name, image string
	var owned, mainboard int
	require.NoError(t, row.Scan(&name, &image, &owned, &mainboard))

	assert.Equal(t, "Luke Skywalker", name)
	assert.Equal(t, "https://example.com/luke.jpg", image)
	assert.Equal(t, 2, owned)
	assert.Equal(t, 1, mainboard)
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

func TestCardExistsByName_CardDoesNotExist_ReturnsFalse(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	exists, err := db.CardExistsByName("Nonexistent Card")

	require.NoError(t, err)
	assert.False(t, exists)
}

func TestCardExistsByName_CardExists_ReturnsTrue(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	_, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, 0)",
		"Luke Skywalker, Jedi Knight",
	)
	require.NoError(t, err)

	exists, err := db.CardExistsByName("Luke Skywalker, Jedi Knight")

	require.NoError(t, err)
	assert.True(t, exists)
}

func TestCardExistsByName_EmptyName_ReturnsError(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	exists, err := db.CardExistsByName("")

	assert.False(t, exists)
	assert.ErrorContains(t, err, "must not be empty")
}

func TestInsertCard_ValidNameWithImage_InsertsWithOwnedZeroAndImage(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	err := db.InsertCard("Chewbacca, Hero of Kessel", "images/LAW001.png", true)
	require.NoError(t, err)

	row := db.Connection().QueryRow(
		"SELECT name, image, owned, mainboard FROM cards WHERE name = ?",
		"Chewbacca, Hero of Kessel",
	)

	var name, image string
	var owned, mainboard int
	require.NoError(t, row.Scan(&name, &image, &owned, &mainboard))
	assert.Equal(t, "Chewbacca, Hero of Kessel", name)
	assert.Equal(t, "images/LAW001.png", image)
	assert.Equal(t, 0, owned)
	assert.Equal(t, 1, mainboard)
}

func TestInsertCard_MainboardTrue_StoresOne(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	err := db.InsertCard("Luke Skywalker, Jedi Knight", "", true)
	require.NoError(t, err)

	row := db.Connection().QueryRow(
		"SELECT mainboard FROM cards WHERE name = ?",
		"Luke Skywalker, Jedi Knight",
	)

	var mainboard int
	require.NoError(t, row.Scan(&mainboard))
	assert.Equal(t, 1, mainboard)
}

func TestInsertCard_MainboardFalse_StoresZero(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	err := db.InsertCard("Mace Windu, Party Crasher", "", false)
	require.NoError(t, err)

	row := db.Connection().QueryRow(
		"SELECT mainboard FROM cards WHERE name = ?",
		"Mace Windu, Party Crasher",
	)

	var mainboard int
	require.NoError(t, row.Scan(&mainboard))
	assert.Equal(t, 0, mainboard)
}

func TestInsertCard_ValidNameWithEmptyImage_InsertsWithNullImage(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	err := db.InsertCard("Chewbacca, Hero of Kessel", "", true)
	require.NoError(t, err)

	row := db.Connection().QueryRow(
		"SELECT name, owned FROM cards WHERE name = ?",
		"Chewbacca, Hero of Kessel",
	)

	var name string
	var owned int
	require.NoError(t, row.Scan(&name, &owned))
	assert.Equal(t, "Chewbacca, Hero of Kessel", name)
	assert.Equal(t, 0, owned)
}

func TestInsertCard_EmptyName_ReturnsError(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	err := db.InsertCard("", "images/LAW001.png", true)

	assert.ErrorContains(t, err, "must not be empty")
}

func TestGetCardByID_ExistingCard_ReturnsCard(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	result, err := db.Connection().Exec(
		"INSERT INTO cards (name, image, owned) VALUES (?, ?, ?)",
		"Luke Skywalker, Jedi Knight", "https://example.com/luke.jpg", 2,
	)
	require.NoError(t, err)
	insertedID, err := result.LastInsertId()
	require.NoError(t, err)

	card, err := db.GetCardByID(int(insertedID))

	require.NoError(t, err)
	assert.Equal(t, int(insertedID), card.ID)
	assert.Equal(t, "Luke Skywalker, Jedi Knight", card.Name)
	assert.Equal(t, "https://example.com/luke.jpg", card.Image)
	assert.Equal(t, 2, card.Owned)
	assert.True(t, card.Mainboard, "expected mainboard to default to true")
}

func TestGetCardByID_MainboardFalse_ReturnsMainboardFalse(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	result, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned, mainboard) VALUES (?, ?, ?)",
		"Mace Windu, Vaapad Form Master", 0, 0,
	)
	require.NoError(t, err)
	insertedID, err := result.LastInsertId()
	require.NoError(t, err)

	card, err := db.GetCardByID(int(insertedID))

	require.NoError(t, err)
	assert.False(t, card.Mainboard, "expected mainboard to be false for a leader card")
}

func TestGetCardByID_NullImage_ReturnsEmptyString(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	result, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?)",
		"Chewbacca, Hero of Kessel", 0,
	)
	require.NoError(t, err)
	insertedID, err := result.LastInsertId()
	require.NoError(t, err)

	card, err := db.GetCardByID(int(insertedID))

	require.NoError(t, err)
	assert.Equal(t, "", card.Image, "expected empty string for null image")
}

func TestGetCardByID_NonExistentID_ReturnsErrCardNotFound(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	card, err := db.GetCardByID(99999)

	assert.Nil(t, card)
	assert.ErrorIs(t, err, database.ErrCardNotFound)
}

func TestGetCardByID_ZeroID_ReturnsError(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	card, err := db.GetCardByID(0)

	assert.Nil(t, card)
	assert.ErrorContains(t, err, "must be a positive integer")
}

func TestGetCardByID_NegativeID_ReturnsError(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	card, err := db.GetCardByID(-1)

	assert.Nil(t, card)
	assert.ErrorContains(t, err, "must be a positive integer")
}

func TestIncrementCardOwned_ExistingCard_IncrementsOwned(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	result, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?)",
		"Luke Skywalker, Jedi Knight", 2,
	)
	require.NoError(t, err)
	insertedID, err := result.LastInsertId()
	require.NoError(t, err)

	err = db.IncrementCardOwned(int(insertedID))

	require.NoError(t, err)

	row := db.Connection().QueryRow("SELECT owned FROM cards WHERE id = ?", insertedID)
	var owned int
	require.NoError(t, row.Scan(&owned))
	assert.Equal(t, 3, owned)
}

func TestIncrementCardOwned_NonExistentID_ReturnsErrCardNotFound(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	err := db.IncrementCardOwned(99999)

	assert.ErrorIs(t, err, database.ErrCardNotFound)
}

func TestIncrementCardOwned_ZeroID_ReturnsError(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	err := db.IncrementCardOwned(0)

	assert.ErrorContains(t, err, "must be a positive integer")
}

func TestIncrementCardOwned_NegativeID_ReturnsError(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	err := db.IncrementCardOwned(-1)

	assert.ErrorContains(t, err, "must be a positive integer")
}

func TestDecrementCardOwned_ExistingCardWithPositiveOwned_DecrementsOwned(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	result, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?)",
		"Chewbacca, Hero of Kessel", 3,
	)
	require.NoError(t, err)
	insertedID, err := result.LastInsertId()
	require.NoError(t, err)

	err = db.DecrementCardOwned(int(insertedID))

	require.NoError(t, err)

	row := db.Connection().QueryRow("SELECT owned FROM cards WHERE id = ?", insertedID)
	var owned int
	require.NoError(t, row.Scan(&owned))
	assert.Equal(t, 2, owned)
}

func TestDecrementCardOwned_ExistingCardWithZeroOwned_StaysAtZero(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	result, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?)",
		"Chewbacca, Hero of Kessel", 0,
	)
	require.NoError(t, err)
	insertedID, err := result.LastInsertId()
	require.NoError(t, err)

	err = db.DecrementCardOwned(int(insertedID))

	require.NoError(t, err)

	row := db.Connection().QueryRow("SELECT owned FROM cards WHERE id = ?", insertedID)
	var owned int
	require.NoError(t, row.Scan(&owned))
	assert.Equal(t, 0, owned)
}

func TestDecrementCardOwned_NonExistentID_ReturnsErrCardNotFound(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	err := db.DecrementCardOwned(99999)

	assert.ErrorIs(t, err, database.ErrCardNotFound)
}

func TestDecrementCardOwned_ZeroID_ReturnsError(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	err := db.DecrementCardOwned(0)

	assert.ErrorContains(t, err, "must be a positive integer")
}

func TestDecrementCardOwned_NegativeID_ReturnsError(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	err := db.DecrementCardOwned(-1)

	assert.ErrorContains(t, err, "must be a positive integer")
}

func TestSearchCards_EmptyDatabase_EmptyQuery_ReturnsEmptySlice(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	result, err := db.SearchCards("")

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestSearchCards_EmptyQuery_ReturnsAllCards(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	_, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?), (?, ?)",
		"Luke Skywalker, Jedi Knight", 0,
		"Chewbacca, Hero of Kessel", 0,
	)
	require.NoError(t, err)

	result, err := db.SearchCards("")

	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestSearchCards_ExactNameMatch_ReturnsMatchingCard(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	_, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?)",
		"Luke Skywalker, Jedi Knight", 2,
	)
	require.NoError(t, err)

	result, err := db.SearchCards("Luke Skywalker, Jedi Knight")

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "Luke Skywalker, Jedi Knight", result[0].Name)
}

func TestSearchCards_PartialNameMatch_ReturnsMatchingCards(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	_, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?), (?, ?), (?, ?)",
		"Luke Skywalker, Jedi Knight", 0,
		"Luke Skywalker, Rebel Hero", 0,
		"Chewbacca, Hero of Kessel", 0,
	)
	require.NoError(t, err)

	result, err := db.SearchCards("Luke")

	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestSearchCards_CaseInsensitiveMatch_ReturnsMatchingCards(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	_, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?)",
		"Luke Skywalker, Jedi Knight", 0,
	)
	require.NoError(t, err)

	result, err := db.SearchCards("LUKE")

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "Luke Skywalker, Jedi Knight", result[0].Name)
}

func TestSearchCards_NoMatch_ReturnsEmptySlice(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	_, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?)",
		"Luke Skywalker, Jedi Knight", 0,
	)
	require.NoError(t, err)

	result, err := db.SearchCards("Darth Vader")

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestSearchCards_NullImage_ReturnsEmptyStringForImage(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	_, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?)",
		"Luke Skywalker, Jedi Knight", 0,
	)
	require.NoError(t, err)

	result, err := db.SearchCards("Luke")

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "", result[0].Image, "expected empty string for null image")
}

func TestSearchCards_DefaultInsert_ReturnsMainboardTrue(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	_, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?)",
		"Luke Skywalker, Jedi Knight", 0,
	)
	require.NoError(t, err)

	result, err := db.SearchCards("Luke")

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.True(t, result[0].Mainboard, "expected mainboard to default to true")
}

func TestSearchCards_MainboardFalse_ReturnsMainboardFalse(t *testing.T) {
	db := newTestDatabase(t)
	require.NoError(t, db.RunMigrations())

	_, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned, mainboard) VALUES (?, ?, ?)",
		"Mace Windu, Vaapad Form Master", 0, 0,
	)
	require.NoError(t, err)

	result, err := db.SearchCards("Mace Windu")

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.False(t, result[0].Mainboard, "expected mainboard to be false")
}
