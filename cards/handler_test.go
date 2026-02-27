package cards_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"swucol/cards"
	"swucol/database"
	"swucol/models"
)

// newTestDatabase creates a Database backed by a temporary file that is
// cleaned up automatically when the test ends.
func newTestDatabase(t *testing.T) *database.Database {
	t.Helper()

	filePath := filepath.Join(t.TempDir(), "test.db")

	db, err := database.New(filePath)
	require.NoError(t, err, "expected no error opening test database")

	require.NoError(t, db.RunMigrations())

	t.Cleanup(func() {
		db.Shutdown()
	})

	return db
}

// postImport sends a POST request to the ImportCardsHandler with the given body.
func postImport(t *testing.T, db *database.Database, body string) *http.Response {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, "/cards/import", strings.NewReader(body))
	recorder := httptest.NewRecorder()

	cards.ImportCardsHandler(db)(recorder, request)

	return recorder.Result()
}

// validCSVHeader is the standard CSV header matching the expected format.
const validCSVHeader = "Set,Card Number,Card Name,Card Title,Card Type,Aspects,Variant Type,Rarity,Foil,Stamp,Artist,Owned Count,Group Owned Count"

func TestImportCardsHandler_ValidCSV_InsertsNewCards(t *testing.T) {
	db := newTestDatabase(t)

	csv := validCSVHeader + "\n" +
		"LAW,001,Chewbacca,Hero of Kessel,Character,Heroism,Normal,Rare,false,,Artist One,0,0\n" +
		"LAW,002,Luke Skywalker,Jedi Knight,Character,Heroism,Normal,Rare,false,,Artist Two,0,0"

	response := postImport(t, db, csv)

	assert.Equal(t, http.StatusNoContent, response.StatusCode)

	exists, err := db.CardExistsByName("Chewbacca, Hero of Kessel")
	require.NoError(t, err)
	assert.True(t, exists, "expected Chewbacca, Hero of Kessel to be inserted")

	exists, err = db.CardExistsByName("Luke Skywalker, Jedi Knight")
	require.NoError(t, err)
	assert.True(t, exists, "expected Luke Skywalker, Jedi Knight to be inserted")
}

func TestImportCardsHandler_InsertsCardsWithOwnedZero(t *testing.T) {
	db := newTestDatabase(t)

	csv := validCSVHeader + "\n" +
		"LAW,001,Chewbacca,Hero of Kessel,Character,Heroism,Normal,Rare,false,,Artist One,5,10"

	response := postImport(t, db, csv)

	assert.Equal(t, http.StatusNoContent, response.StatusCode)

	row := db.Connection().QueryRow(
		"SELECT owned FROM cards WHERE name = ?",
		"Chewbacca, Hero of Kessel",
	)
	var owned int
	require.NoError(t, row.Scan(&owned))
	assert.Equal(t, 0, owned, "expected owned to be 0 regardless of CSV Owned Count")
}

func TestImportCardsHandler_DuplicateCards_SkipsExisting(t *testing.T) {
	db := newTestDatabase(t)

	csv := validCSVHeader + "\n" +
		"LAW,001,Chewbacca,Hero of Kessel,Character,Heroism,Normal,Rare,false,,Artist One,0,0"

	// Import the same CSV twice.
	response := postImport(t, db, csv)
	assert.Equal(t, http.StatusNoContent, response.StatusCode)

	response = postImport(t, db, csv)
	assert.Equal(t, http.StatusNoContent, response.StatusCode)

	// Confirm only one row exists for this card.
	row := db.Connection().QueryRow(
		"SELECT COUNT(*) FROM cards WHERE name = ?",
		"Chewbacca, Hero of Kessel",
	)
	var count int
	require.NoError(t, row.Scan(&count))
	assert.Equal(t, 1, count, "expected exactly one row for a card imported twice")
}

func TestImportCardsHandler_DuplicateRowsInSameCSV_InsertedOnce(t *testing.T) {
	db := newTestDatabase(t)

	csv := validCSVHeader + "\n" +
		"LAW,001,Chewbacca,Hero of Kessel,Character,Heroism,Normal,Rare,false,,Artist One,0,0\n" +
		"LAW,001,Chewbacca,Hero of Kessel,Character,Heroism,Normal,Rare,false,,Artist One,0,0"

	response := postImport(t, db, csv)

	assert.Equal(t, http.StatusNoContent, response.StatusCode)

	row := db.Connection().QueryRow(
		"SELECT COUNT(*) FROM cards WHERE name = ?",
		"Chewbacca, Hero of Kessel",
	)
	var count int
	require.NoError(t, row.Scan(&count))
	assert.Equal(t, 1, count, "expected exactly one row when the same card appears twice in the CSV")
}

func TestImportCardsHandler_EmptyCardTitle_UsesCardNameOnly(t *testing.T) {
	db := newTestDatabase(t)

	csv := validCSVHeader + "\n" +
		"LAW,001,Chewbacca,,Character,Heroism,Normal,Rare,false,,Artist One,0,0"

	response := postImport(t, db, csv)

	assert.Equal(t, http.StatusNoContent, response.StatusCode)

	exists, err := db.CardExistsByName("Chewbacca")
	require.NoError(t, err)
	assert.True(t, exists, "expected card to be stored as 'Chewbacca' when title is empty")
}

func TestImportCardsHandler_MalformedCSV_Returns400(t *testing.T) {
	db := newTestDatabase(t)

	response := postImport(t, db, "this is not a valid csv\x00\xff")

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestImportCardsHandler_WrongHeaderFormat_Returns400(t *testing.T) {
	db := newTestDatabase(t)

	csv := "Wrong,Header,Format\n" +
		"LAW,001,Chewbacca,Hero of Kessel,Character,Heroism,Normal,Rare,false,,Artist One,0,0"

	response := postImport(t, db, csv)

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestImportCardsHandler_EmptyBody_Returns400(t *testing.T) {
	db := newTestDatabase(t)

	response := postImport(t, db, "")

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestImportCardsHandler_CSVWithNoDataRows_Returns400(t *testing.T) {
	db := newTestDatabase(t)

	response := postImport(t, db, validCSVHeader)

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

// getCard sends a GET request to GetCardHandler for the given raw id string.
func getCard(t *testing.T, db *database.Database, rawID string) *http.Response {
	t.Helper()

	target := fmt.Sprintf("/cards/%s", rawID)
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.SetPathValue("id", rawID)
	recorder := httptest.NewRecorder()

	cards.GetCardHandler(db)(recorder, request)

	return recorder.Result()
}

func TestGetCardHandler_ExistingCard_Returns200WithJSON(t *testing.T) {
	db := newTestDatabase(t)

	result, err := db.Connection().Exec(
		"INSERT INTO cards (name, image, owned) VALUES (?, ?, ?)",
		"Luke Skywalker, Jedi Knight", "https://example.com/luke.jpg", 3,
	)
	require.NoError(t, err)
	insertedID, err := result.LastInsertId()
	require.NoError(t, err)

	response := getCard(t, db, fmt.Sprintf("%d", insertedID))

	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "application/json", response.Header.Get("Content-Type"))

	var card models.Card
	require.NoError(t, json.NewDecoder(response.Body).Decode(&card))
	assert.Equal(t, int(insertedID), card.ID)
	assert.Equal(t, "Luke Skywalker, Jedi Knight", card.Name)
	assert.Equal(t, "https://example.com/luke.jpg", card.Image)
	assert.Equal(t, 3, card.Owned)
}

func TestGetCardHandler_NullImage_Returns200WithEmptyImageField(t *testing.T) {
	db := newTestDatabase(t)

	result, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?)",
		"Chewbacca, Hero of Kessel", 0,
	)
	require.NoError(t, err)
	insertedID, err := result.LastInsertId()
	require.NoError(t, err)

	response := getCard(t, db, fmt.Sprintf("%d", insertedID))

	assert.Equal(t, http.StatusOK, response.StatusCode)

	var card models.Card
	require.NoError(t, json.NewDecoder(response.Body).Decode(&card))
	assert.Equal(t, "", card.Image, "expected empty string for null image")
}

func TestGetCardHandler_NonExistentID_Returns404(t *testing.T) {
	db := newTestDatabase(t)

	response := getCard(t, db, "99999")

	assert.Equal(t, http.StatusNotFound, response.StatusCode)
}

func TestGetCardHandler_NonIntegerID_Returns400(t *testing.T) {
	db := newTestDatabase(t)

	response := getCard(t, db, "abc")

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestGetCardHandler_ZeroID_Returns400(t *testing.T) {
	db := newTestDatabase(t)

	response := getCard(t, db, "0")

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestGetCardHandler_NegativeID_Returns400(t *testing.T) {
	db := newTestDatabase(t)

	response := getCard(t, db, "-1")

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}
