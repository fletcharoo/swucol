package cards_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
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

// postImport sends a POST request to the ImportCardsHandler with the given
// HTTP client, images directory, image base URL, and CSV body.
func postImport(t *testing.T, db *database.Database, httpClient *http.Client, imagesDir, imageBaseURL, body string) *http.Response {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, "/cards/import", strings.NewReader(body))
	recorder := httptest.NewRecorder()

	cards.ImportCardsHandler(db, httpClient, imagesDir, imageBaseURL)(recorder, request)

	return recorder.Result()
}

// validCSVHeader is the standard CSV header matching the expected format.
const validCSVHeader = "Set,Card Number,Card Name,Card Title,Card Type,Aspects,Variant Type,Rarity,Foil,Stamp,Artist,Owned Count,Group Owned Count"

func TestImportCardsHandler_ValidCSV_InsertsNewCards(t *testing.T) {
	db := newTestDatabase(t)
	imagesDir := t.TempDir()

	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fake-png-data"))
	}))
	defer imageServer.Close()

	csv := validCSVHeader + "\n" +
		"LAW,001,Chewbacca,Hero of Kessel,Character,Heroism,Normal,Rare,false,,Artist One,0,0\n" +
		"LAW,002,Luke Skywalker,Jedi Knight,Character,Heroism,Normal,Rare,false,,Artist Two,0,0"

	response := postImport(t, db, imageServer.Client(), imagesDir, imageServer.URL, csv)

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
	imagesDir := t.TempDir()

	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fake-png-data"))
	}))
	defer imageServer.Close()

	csv := validCSVHeader + "\n" +
		"LAW,001,Chewbacca,Hero of Kessel,Character,Heroism,Normal,Rare,false,,Artist One,5,10"

	response := postImport(t, db, imageServer.Client(), imagesDir, imageServer.URL, csv)

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
	imagesDir := t.TempDir()

	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fake-png-data"))
	}))
	defer imageServer.Close()

	csv := validCSVHeader + "\n" +
		"LAW,001,Chewbacca,Hero of Kessel,Character,Heroism,Normal,Rare,false,,Artist One,0,0"

	// Import the same CSV twice.
	response := postImport(t, db, imageServer.Client(), imagesDir, imageServer.URL, csv)
	assert.Equal(t, http.StatusNoContent, response.StatusCode)

	response = postImport(t, db, imageServer.Client(), imagesDir, imageServer.URL, csv)
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
	imagesDir := t.TempDir()

	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fake-png-data"))
	}))
	defer imageServer.Close()

	csv := validCSVHeader + "\n" +
		"LAW,001,Chewbacca,Hero of Kessel,Character,Heroism,Normal,Rare,false,,Artist One,0,0\n" +
		"LAW,001,Chewbacca,Hero of Kessel,Character,Heroism,Normal,Rare,false,,Artist One,0,0"

	response := postImport(t, db, imageServer.Client(), imagesDir, imageServer.URL, csv)

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
	imagesDir := t.TempDir()

	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fake-png-data"))
	}))
	defer imageServer.Close()

	csv := validCSVHeader + "\n" +
		"LAW,001,Chewbacca,,Character,Heroism,Normal,Rare,false,,Artist One,0,0"

	response := postImport(t, db, imageServer.Client(), imagesDir, imageServer.URL, csv)

	assert.Equal(t, http.StatusNoContent, response.StatusCode)

	exists, err := db.CardExistsByName("Chewbacca")
	require.NoError(t, err)
	assert.True(t, exists, "expected card to be stored as 'Chewbacca' when title is empty")
}

func TestImportCardsHandler_MalformedCSV_Returns400(t *testing.T) {
	db := newTestDatabase(t)
	imagesDir := t.TempDir()

	response := postImport(t, db, http.DefaultClient, imagesDir, "", "this is not a valid csv\x00\xff")

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestImportCardsHandler_WrongHeaderFormat_Returns400(t *testing.T) {
	db := newTestDatabase(t)
	imagesDir := t.TempDir()

	csv := "Wrong,Header,Format\n" +
		"LAW,001,Chewbacca,Hero of Kessel,Character,Heroism,Normal,Rare,false,,Artist One,0,0"

	response := postImport(t, db, http.DefaultClient, imagesDir, "", csv)

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestImportCardsHandler_UTF8BOMPrefix_ParsesSuccessfully(t *testing.T) {
	db := newTestDatabase(t)
	imagesDir := t.TempDir()

	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fake-png-data"))
	}))
	defer imageServer.Close()

	// Prepend a UTF-8 BOM as Excel and similar tools do when exporting CSVs.
	bom := "\xEF\xBB\xBF"
	csv := bom + validCSVHeader + "\n" +
		"LAW,001,Chewbacca,Hero of Kessel,Character,Heroism,Normal,Rare,false,,Artist One,0,0"

	response := postImport(t, db, imageServer.Client(), imagesDir, imageServer.URL, csv)

	assert.Equal(t, http.StatusNoContent, response.StatusCode)

	exists, err := db.CardExistsByName("Chewbacca, Hero of Kessel")
	require.NoError(t, err)
	assert.True(t, exists, "expected card to be inserted despite BOM prefix")
}

func TestImportCardsHandler_EmptyBody_Returns400(t *testing.T) {
	db := newTestDatabase(t)
	imagesDir := t.TempDir()

	response := postImport(t, db, http.DefaultClient, imagesDir, "", "")

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestImportCardsHandler_CSVWithNoDataRows_Returns400(t *testing.T) {
	db := newTestDatabase(t)
	imagesDir := t.TempDir()

	response := postImport(t, db, http.DefaultClient, imagesDir, "", validCSVHeader)

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestImportCardsHandler_ValidCSV_DownloadsAndSavesImage(t *testing.T) {
	db := newTestDatabase(t)
	imagesDir := t.TempDir()

	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fake-png-data"))
	}))
	defer imageServer.Close()

	csv := validCSVHeader + "\n" +
		"LAW,001,Chewbacca,Hero of Kessel,Character,Heroism,Normal,Rare,false,,Artist One,0,0"

	response := postImport(t, db, imageServer.Client(), imagesDir, imageServer.URL, csv)

	assert.Equal(t, http.StatusNoContent, response.StatusCode)

	expectedFilePath := filepath.Join(imagesDir, "LAW001.png")
	_, err := os.Stat(expectedFilePath)
	assert.NoError(t, err, "expected image file to exist at %s", expectedFilePath)

	row := db.Connection().QueryRow(
		"SELECT image FROM cards WHERE name = ?",
		"Chewbacca, Hero of Kessel",
	)
	var image string
	require.NoError(t, row.Scan(&image))
	assert.Equal(t, expectedFilePath, image, "expected image field to contain the file path")
}

func TestImportCardsHandler_ImageDownloadFails_InsertsCardWithoutImage(t *testing.T) {
	db := newTestDatabase(t)
	imagesDir := t.TempDir()

	// Server always returns 404.
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer imageServer.Close()

	csv := validCSVHeader + "\n" +
		"LAW,001,Chewbacca,Hero of Kessel,Character,Heroism,Normal,Rare,false,,Artist One,0,0"

	response := postImport(t, db, imageServer.Client(), imagesDir, imageServer.URL, csv)

	assert.Equal(t, http.StatusNoContent, response.StatusCode)

	// Card must still be inserted despite the download failure.
	exists, err := db.CardExistsByName("Chewbacca, Hero of Kessel")
	require.NoError(t, err)
	assert.True(t, exists, "expected card to be inserted even when image download fails")

	// Image field must be empty (NULL in the database).
	row := db.Connection().QueryRow(
		"SELECT image FROM cards WHERE name = ?",
		"Chewbacca, Hero of Kessel",
	)
	var image sql.NullString
	require.NoError(t, row.Scan(&image))
	assert.False(t, image.Valid, "expected NULL image field when download fails")
}

func TestImportCardsHandler_ImageAlreadyExists_SkipsDownload(t *testing.T) {
	db := newTestDatabase(t)
	imagesDir := t.TempDir()

	// Pre-create the image file so it already exists on disk.
	existingImagePath := filepath.Join(imagesDir, "LAW001.png")
	require.NoError(t, os.WriteFile(existingImagePath, []byte("existing-image"), 0644))

	// The image server must not be called; track requests with a counter.
	requestCount := 0
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("new-image-data"))
	}))
	defer imageServer.Close()

	csv := validCSVHeader + "\n" +
		"LAW,001,Chewbacca,Hero of Kessel,Character,Heroism,Normal,Rare,false,,Artist One,0,0"

	response := postImport(t, db, imageServer.Client(), imagesDir, imageServer.URL, csv)

	assert.Equal(t, http.StatusNoContent, response.StatusCode)
	assert.Equal(t, 0, requestCount, "expected no download requests when image file already exists")

	// The existing file must not have been overwritten.
	content, err := os.ReadFile(existingImagePath)
	require.NoError(t, err)
	assert.Equal(t, "existing-image", string(content), "expected existing image file to be unchanged")

	// The image field in the DB must point to the existing file.
	row := db.Connection().QueryRow(
		"SELECT image FROM cards WHERE name = ?",
		"Chewbacca, Hero of Kessel",
	)
	var image string
	require.NoError(t, row.Scan(&image))
	assert.Equal(t, existingImagePath, image, "expected image field to reference the existing file path")
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

// incrementCardOwned sends a POST request to IncrementCardOwnedHandler for the given raw id string.
func incrementCardOwned(t *testing.T, db *database.Database, rawID string) *http.Response {
	t.Helper()

	target := fmt.Sprintf("/cards/%s/increment", rawID)
	request := httptest.NewRequest(http.MethodPost, target, nil)
	request.SetPathValue("id", rawID)
	recorder := httptest.NewRecorder()

	cards.IncrementCardOwnedHandler(db)(recorder, request)

	return recorder.Result()
}

// decrementCardOwned sends a POST request to DecrementCardOwnedHandler for the given raw id string.
func decrementCardOwned(t *testing.T, db *database.Database, rawID string) *http.Response {
	t.Helper()

	target := fmt.Sprintf("/cards/%s/decrement", rawID)
	request := httptest.NewRequest(http.MethodPost, target, nil)
	request.SetPathValue("id", rawID)
	recorder := httptest.NewRecorder()

	cards.DecrementCardOwnedHandler(db)(recorder, request)

	return recorder.Result()
}

func TestIncrementCardOwnedHandler_ExistingCard_Returns204AndIncrementsOwned(t *testing.T) {
	db := newTestDatabase(t)

	result, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?)",
		"Luke Skywalker, Jedi Knight", 2,
	)
	require.NoError(t, err)
	insertedID, err := result.LastInsertId()
	require.NoError(t, err)

	response := incrementCardOwned(t, db, fmt.Sprintf("%d", insertedID))

	assert.Equal(t, http.StatusNoContent, response.StatusCode)

	row := db.Connection().QueryRow("SELECT owned FROM cards WHERE id = ?", insertedID)
	var owned int
	require.NoError(t, row.Scan(&owned))
	assert.Equal(t, 3, owned)
}

func TestIncrementCardOwnedHandler_NonExistentID_Returns404(t *testing.T) {
	db := newTestDatabase(t)

	response := incrementCardOwned(t, db, "99999")

	assert.Equal(t, http.StatusNotFound, response.StatusCode)
}

func TestIncrementCardOwnedHandler_NonIntegerID_Returns400(t *testing.T) {
	db := newTestDatabase(t)

	response := incrementCardOwned(t, db, "abc")

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestIncrementCardOwnedHandler_ZeroID_Returns400(t *testing.T) {
	db := newTestDatabase(t)

	response := incrementCardOwned(t, db, "0")

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestIncrementCardOwnedHandler_NegativeID_Returns400(t *testing.T) {
	db := newTestDatabase(t)

	response := incrementCardOwned(t, db, "-1")

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestDecrementCardOwnedHandler_ExistingCardWithPositiveOwned_Returns204AndDecrementsOwned(t *testing.T) {
	db := newTestDatabase(t)

	result, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?)",
		"Chewbacca, Hero of Kessel", 3,
	)
	require.NoError(t, err)
	insertedID, err := result.LastInsertId()
	require.NoError(t, err)

	response := decrementCardOwned(t, db, fmt.Sprintf("%d", insertedID))

	assert.Equal(t, http.StatusNoContent, response.StatusCode)

	row := db.Connection().QueryRow("SELECT owned FROM cards WHERE id = ?", insertedID)
	var owned int
	require.NoError(t, row.Scan(&owned))
	assert.Equal(t, 2, owned)
}

func TestDecrementCardOwnedHandler_ExistingCardWithZeroOwned_Returns204AndKeepsAtZero(t *testing.T) {
	db := newTestDatabase(t)

	result, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?)",
		"Chewbacca, Hero of Kessel", 0,
	)
	require.NoError(t, err)
	insertedID, err := result.LastInsertId()
	require.NoError(t, err)

	response := decrementCardOwned(t, db, fmt.Sprintf("%d", insertedID))

	assert.Equal(t, http.StatusNoContent, response.StatusCode)

	row := db.Connection().QueryRow("SELECT owned FROM cards WHERE id = ?", insertedID)
	var owned int
	require.NoError(t, row.Scan(&owned))
	assert.Equal(t, 0, owned)
}

func TestDecrementCardOwnedHandler_NonExistentID_Returns404(t *testing.T) {
	db := newTestDatabase(t)

	response := decrementCardOwned(t, db, "99999")

	assert.Equal(t, http.StatusNotFound, response.StatusCode)
}

func TestDecrementCardOwnedHandler_NonIntegerID_Returns400(t *testing.T) {
	db := newTestDatabase(t)

	response := decrementCardOwned(t, db, "abc")

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestDecrementCardOwnedHandler_ZeroID_Returns400(t *testing.T) {
	db := newTestDatabase(t)

	response := decrementCardOwned(t, db, "0")

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestDecrementCardOwnedHandler_NegativeID_Returns400(t *testing.T) {
	db := newTestDatabase(t)

	response := decrementCardOwned(t, db, "-1")

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

// searchCards sends a GET request to SearchCardsHandler with the given query string.
// Pass an empty query to omit the "q" parameter entirely.
func searchCards(t *testing.T, db *database.Database, query string) *http.Response {
	t.Helper()

	target := "/cards/search"
	if query != "" {
		target = fmt.Sprintf("/cards/search?q=%s", query)
	}

	request := httptest.NewRequest(http.MethodGet, target, nil)
	recorder := httptest.NewRecorder()

	cards.SearchCardsHandler(db)(recorder, request)

	return recorder.Result()
}

func TestSearchCardsHandler_EmptyDatabase_NoQuery_Returns200WithEmptyArray(t *testing.T) {
	db := newTestDatabase(t)

	response := searchCards(t, db, "")

	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "application/json", response.Header.Get("Content-Type"))

	var result []models.Card
	require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
	assert.Empty(t, result)
}

func TestSearchCardsHandler_NoQuery_Returns200WithAllCards(t *testing.T) {
	db := newTestDatabase(t)

	_, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?), (?, ?)",
		"Luke Skywalker, Jedi Knight", 0,
		"Chewbacca, Hero of Kessel", 0,
	)
	require.NoError(t, err)

	response := searchCards(t, db, "")

	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "application/json", response.Header.Get("Content-Type"))

	var result []models.Card
	require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
	assert.Len(t, result, 2)
}

func TestSearchCardsHandler_PartialQuery_Returns200WithMatchingCards(t *testing.T) {
	db := newTestDatabase(t)

	_, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?), (?, ?), (?, ?)",
		"Luke Skywalker, Jedi Knight", 0,
		"Luke Skywalker, Rebel Hero", 0,
		"Chewbacca, Hero of Kessel", 0,
	)
	require.NoError(t, err)

	response := searchCards(t, db, "Luke")

	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "application/json", response.Header.Get("Content-Type"))

	var result []models.Card
	require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
	assert.Len(t, result, 2)
	for _, card := range result {
		assert.Contains(t, card.Name, "Luke")
	}
}

func TestSearchCardsHandler_QueryWithNoMatch_Returns200WithEmptyArray(t *testing.T) {
	db := newTestDatabase(t)

	_, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?)",
		"Luke Skywalker, Jedi Knight", 0,
	)
	require.NoError(t, err)

	response := searchCards(t, db, "Darth+Vader")

	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "application/json", response.Header.Get("Content-Type"))

	var result []models.Card
	require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
	assert.Empty(t, result)
}

// newTestTemplates loads the application HTML templates relative to this
// test file's location in the cards/ package directory.
func newTestTemplates(t *testing.T) *template.Template {
	t.Helper()

	tmpl, err := template.ParseGlob("../templates/*.html")
	require.NoError(t, err, "expected no error loading test templates")

	return tmpl
}

// postImportHTML sends a POST request to ImportCardsHTMLHandler with a
// multipart/form-data body containing a "file" field with the given CSV content.
func postImportHTML(t *testing.T, db *database.Database, httpClient *http.Client, imagesDir, imageBaseURL, csvContent string) *http.Response {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", "cards.csv")
	require.NoError(t, err)

	_, err = io.WriteString(part, csvContent)
	require.NoError(t, err)

	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "/cards/import/html", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()

	cards.ImportCardsHTMLHandler(db, httpClient, imagesDir, imageBaseURL)(recorder, request)

	return recorder.Result()
}

// searchCardsHTML sends a GET request to SearchCardsHTMLHandler with the
// given query string. Pass an empty query to omit the "q" parameter entirely.
func searchCardsHTML(t *testing.T, db *database.Database, tmpl *template.Template, query string) *http.Response {
	t.Helper()

	target := "/cards/search/html"
	if query != "" {
		target = fmt.Sprintf("/cards/search/html?q=%s", query)
	}

	request := httptest.NewRequest(http.MethodGet, target, nil)
	recorder := httptest.NewRecorder()

	cards.SearchCardsHTMLHandler(db, tmpl)(recorder, request)

	return recorder.Result()
}

// incrementCardOwnedHTML sends a POST request to IncrementCardOwnedHTMLHandler
// for the given raw id string.
func incrementCardOwnedHTML(t *testing.T, db *database.Database, tmpl *template.Template, rawID string) *http.Response {
	t.Helper()

	target := fmt.Sprintf("/cards/%s/increment/html", rawID)
	request := httptest.NewRequest(http.MethodPost, target, nil)
	request.SetPathValue("id", rawID)
	recorder := httptest.NewRecorder()

	cards.IncrementCardOwnedHTMLHandler(db, tmpl)(recorder, request)

	return recorder.Result()
}

// decrementCardOwnedHTML sends a POST request to DecrementCardOwnedHTMLHandler
// for the given raw id string.
func decrementCardOwnedHTML(t *testing.T, db *database.Database, tmpl *template.Template, rawID string) *http.Response {
	t.Helper()

	target := fmt.Sprintf("/cards/%s/decrement/html", rawID)
	request := httptest.NewRequest(http.MethodPost, target, nil)
	request.SetPathValue("id", rawID)
	recorder := httptest.NewRecorder()

	cards.DecrementCardOwnedHTMLHandler(db, tmpl)(recorder, request)

	return recorder.Result()
}

func TestIndexHandler_Returns200WithHTMLPage(t *testing.T) {
	db := newTestDatabase(t)
	tmpl := newTestTemplates(t)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()

	cards.IndexHandler(db, tmpl)(recorder, request)

	response := recorder.Result()
	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Contains(t, response.Header.Get("Content-Type"), "text/html")

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "<!DOCTYPE html>")
	assert.Contains(t, string(body), "SWU Collection")
}

func TestIndexHandler_WithCards_RendersCardNames(t *testing.T) {
	db := newTestDatabase(t)
	tmpl := newTestTemplates(t)

	_, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?), (?, ?)",
		"Luke Skywalker, Jedi Knight", 0,
		"Chewbacca, Hero of Kessel", 0,
	)
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()

	cards.IndexHandler(db, tmpl)(recorder, request)

	response := recorder.Result()
	assert.Equal(t, http.StatusOK, response.StatusCode)

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "Luke Skywalker, Jedi Knight")
	assert.Contains(t, string(body), "Chewbacca, Hero of Kessel")
}

func TestSearchCardsHTMLHandler_NoQuery_Returns200WithAllCards(t *testing.T) {
	db := newTestDatabase(t)
	tmpl := newTestTemplates(t)

	_, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?), (?, ?)",
		"Luke Skywalker, Jedi Knight", 0,
		"Chewbacca, Hero of Kessel", 0,
	)
	require.NoError(t, err)

	response := searchCardsHTML(t, db, tmpl, "")

	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Contains(t, response.Header.Get("Content-Type"), "text/html")

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "Luke Skywalker, Jedi Knight")
	assert.Contains(t, string(body), "Chewbacca, Hero of Kessel")
}

func TestSearchCardsHTMLHandler_WithQuery_ReturnsFilteredCards(t *testing.T) {
	db := newTestDatabase(t)
	tmpl := newTestTemplates(t)

	_, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?), (?, ?), (?, ?)",
		"Luke Skywalker, Jedi Knight", 0,
		"Luke Skywalker, Rebel Hero", 0,
		"Chewbacca, Hero of Kessel", 0,
	)
	require.NoError(t, err)

	response := searchCardsHTML(t, db, tmpl, "Luke")

	assert.Equal(t, http.StatusOK, response.StatusCode)

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	bodyStr := string(body)
	assert.Contains(t, bodyStr, "Luke Skywalker, Jedi Knight")
	assert.Contains(t, bodyStr, "Luke Skywalker, Rebel Hero")
	assert.NotContains(t, bodyStr, "Chewbacca, Hero of Kessel")
}

func TestSearchCardsHTMLHandler_EmptyDatabase_ReturnsNoCardsMessage(t *testing.T) {
	db := newTestDatabase(t)
	tmpl := newTestTemplates(t)

	response := searchCardsHTML(t, db, tmpl, "")

	assert.Equal(t, http.StatusOK, response.StatusCode)

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "No cards found.")
}

func TestImportCardsHTMLHandler_ValidCSV_Returns200WithHXTriggerHeader(t *testing.T) {
	db := newTestDatabase(t)
	imagesDir := t.TempDir()

	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fake-png-data"))
	}))
	defer imageServer.Close()

	csvContent := validCSVHeader + "\n" +
		"LAW,001,Chewbacca,Hero of Kessel,Character,Heroism,Normal,Rare,false,,Artist One,0,0"

	response := postImportHTML(t, db, imageServer.Client(), imagesDir, imageServer.URL, csvContent)

	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "cardsImported", response.Header.Get("HX-Trigger"))

	exists, err := db.CardExistsByName("Chewbacca, Hero of Kessel")
	require.NoError(t, err)
	assert.True(t, exists, "expected card to be inserted")
}

func TestImportCardsHTMLHandler_MalformedCSV_Returns400(t *testing.T) {
	db := newTestDatabase(t)
	imagesDir := t.TempDir()

	response := postImportHTML(t, db, http.DefaultClient, imagesDir, "", "this is not valid csv")

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestImportCardsHTMLHandler_MissingFileField_Returns400(t *testing.T) {
	db := newTestDatabase(t)

	// Send a multipart form with a different field name (not "file").
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("wrong-field", "cards.csv")
	require.NoError(t, err)
	_, err = io.WriteString(part, validCSVHeader+"\nLAW,001,Chewbacca,,Character,Heroism,Normal,Rare,false,,A,0,0")
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "/cards/import/html", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()

	cards.ImportCardsHTMLHandler(db, http.DefaultClient, t.TempDir(), "")(recorder, request)

	assert.Equal(t, http.StatusBadRequest, recorder.Result().StatusCode)
}

func TestIncrementCardOwnedHTMLHandler_ExistingCard_Returns200WithUpdatedFragment(t *testing.T) {
	db := newTestDatabase(t)
	tmpl := newTestTemplates(t)

	result, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?)",
		"Luke Skywalker, Jedi Knight", 0,
	)
	require.NoError(t, err)
	insertedID, err := result.LastInsertId()
	require.NoError(t, err)

	response := incrementCardOwnedHTML(t, db, tmpl, fmt.Sprintf("%d", insertedID))

	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Contains(t, response.Header.Get("Content-Type"), "text/html")

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	bodyStr := string(body)
	assert.Contains(t, bodyStr, "Owned: 1")
	assert.Contains(t, bodyStr, fmt.Sprintf("id=\"owned-%d\"", insertedID))
}

func TestIncrementCardOwnedHTMLHandler_NonExistentID_Returns404(t *testing.T) {
	db := newTestDatabase(t)
	tmpl := newTestTemplates(t)

	response := incrementCardOwnedHTML(t, db, tmpl, "99999")

	assert.Equal(t, http.StatusNotFound, response.StatusCode)
}

func TestIncrementCardOwnedHTMLHandler_NonIntegerID_Returns400(t *testing.T) {
	db := newTestDatabase(t)
	tmpl := newTestTemplates(t)

	response := incrementCardOwnedHTML(t, db, tmpl, "abc")

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestIncrementCardOwnedHTMLHandler_ZeroID_Returns400(t *testing.T) {
	db := newTestDatabase(t)
	tmpl := newTestTemplates(t)

	response := incrementCardOwnedHTML(t, db, tmpl, "0")

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestDecrementCardOwnedHTMLHandler_PositiveOwned_Returns200WithDecrementedCount(t *testing.T) {
	db := newTestDatabase(t)
	tmpl := newTestTemplates(t)

	result, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?)",
		"Chewbacca, Hero of Kessel", 3,
	)
	require.NoError(t, err)
	insertedID, err := result.LastInsertId()
	require.NoError(t, err)

	response := decrementCardOwnedHTML(t, db, tmpl, fmt.Sprintf("%d", insertedID))

	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Contains(t, response.Header.Get("Content-Type"), "text/html")

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "Owned: 2")
}

func TestDecrementCardOwnedHTMLHandler_ZeroOwned_Returns200WithZeroCount(t *testing.T) {
	db := newTestDatabase(t)
	tmpl := newTestTemplates(t)

	result, err := db.Connection().Exec(
		"INSERT INTO cards (name, owned) VALUES (?, ?)",
		"Chewbacca, Hero of Kessel", 0,
	)
	require.NoError(t, err)
	insertedID, err := result.LastInsertId()
	require.NoError(t, err)

	response := decrementCardOwnedHTML(t, db, tmpl, fmt.Sprintf("%d", insertedID))

	assert.Equal(t, http.StatusOK, response.StatusCode)

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "Owned: 0")
}

func TestDecrementCardOwnedHTMLHandler_NonExistentID_Returns404(t *testing.T) {
	db := newTestDatabase(t)
	tmpl := newTestTemplates(t)

	response := decrementCardOwnedHTML(t, db, tmpl, "99999")

	assert.Equal(t, http.StatusNotFound, response.StatusCode)
}

func TestDecrementCardOwnedHTMLHandler_NonIntegerID_Returns400(t *testing.T) {
	db := newTestDatabase(t)
	tmpl := newTestTemplates(t)

	response := decrementCardOwnedHTML(t, db, tmpl, "abc")

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}
