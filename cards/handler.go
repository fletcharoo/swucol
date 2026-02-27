// Package cards provides HTTP handlers for card-related API endpoints.
package cards

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"swucol/database"
	"swucol/models"
)

// utf8BOM is the three-byte UTF-8 byte order mark prepended by some editors
// and spreadsheet applications (e.g. Excel) when exporting CSV files.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// csvColumnCount is the expected number of columns in a valid card CSV.
const csvColumnCount = 13

// csvHeaderSet is the value expected in the first column of the header row.
const csvHeaderSet = "Set"

// imageDownloadInterval is the minimum duration between image downloads to
// stay within the rate limit of 10 images per second.
const imageDownloadInterval = 100 * time.Millisecond

// importError wraps an error with an HTTP status code so callers can return
// the correct error response without inspecting error strings.
type importError struct {
	statusCode int
	message    string
}

// Error implements the error interface.
func (e *importError) Error() string {
	return e.message
}

// parseCardsCSV reads a CSV from reader and returns a slice of CardCSV records.
// The first row must be the header row. Returns an error if the CSV is empty,
// malformed, or has an unexpected number of columns. A UTF-8 BOM at the start
// of the stream is silently stripped before parsing.
func parseCardsCSV(reader io.Reader) ([]models.CardCSV, error) {
	if reader == nil {
		return nil, errors.New("reader must not be nil")
	}

	// Wrap in a buffered reader so we can peek ahead and strip any UTF-8 BOM
	// that Excel and similar tools prepend to CSV exports.
	buffered := bufio.NewReader(reader)
	if peeked, err := buffered.Peek(len(utf8BOM)); err == nil && bytes.Equal(peeked, utf8BOM) {
		buffered.Discard(len(utf8BOM))
	}

	csvReader := csv.NewReader(buffered)

	header, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}

	if len(header) != csvColumnCount || header[0] != csvHeaderSet {
		return nil, errors.New("CSV header does not match expected format")
	}

	var cards []models.CardCSV
	for {
		record, err := csvReader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read CSV record: %w", err)
		}

		cards = append(cards, models.CardCSV{
			Set:             record[0],
			CardNumber:      record[1],
			CardName:        record[2],
			CardTitle:       record[3],
			CardType:        record[4],
			Aspects:         record[5],
			VariantType:     record[6],
			Rarity:          record[7],
			Foil:            record[8],
			Stamp:           record[9],
			Artist:          record[10],
			OwnedCount:      record[11],
			GroupOwnedCount: record[12],
		})
	}

	return cards, nil
}

// cardCSVToName converts a CardCSV record to the card name used in the database.
// The name is formed by combining CardName and CardTitle with a comma-space
// separator. If CardTitle is empty, only CardName is returned.
func cardCSVToName(card models.CardCSV) string {
	if strings.TrimSpace(card.CardTitle) == "" {
		return card.CardName
	}
	return card.CardName + ", " + card.CardTitle
}

// buildImageURL constructs the remote image URL for a card using the given
// base URL, set, and card number. Returns an error if any argument is empty.
func buildImageURL(imageBaseURL, set, cardNumber string) (string, error) {
	if imageBaseURL == "" {
		return "", errors.New("image base URL must not be empty")
	}
	if set == "" {
		return "", errors.New("set must not be empty")
	}
	if cardNumber == "" {
		return "", errors.New("card number must not be empty")
	}
	return fmt.Sprintf("%s/%s/%s.png", imageBaseURL, set, cardNumber), nil
}

// buildImageFilePath constructs the local file path where a card image is
// saved, using the provided images directory, set, and card number.
// Returns an error if any argument is empty.
func buildImageFilePath(imagesDir, set, cardNumber string) (string, error) {
	if imagesDir == "" {
		return "", errors.New("images directory must not be empty")
	}
	if set == "" {
		return "", errors.New("set must not be empty")
	}
	if cardNumber == "" {
		return "", errors.New("card number must not be empty")
	}
	return filepath.Join(imagesDir, set+cardNumber+".png"), nil
}

// downloadCardImage downloads the image at imageURL and writes it to destPath.
// The parent directory of destPath is created if it does not already exist.
// Returns an error if the HTTP request fails, the server returns a non-200
// status, or the file cannot be written.
func downloadCardImage(httpClient *http.Client, imageURL, destPath string) error {
	if httpClient == nil {
		return errors.New("http client must not be nil")
	}
	if imageURL == "" {
		return errors.New("image URL must not be empty")
	}
	if destPath == "" {
		return errors.New("destination path must not be empty")
	}

	resp, err := httpClient.Get(imageURL)
	if err != nil {
		return fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("image download returned status %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("create image directory: %w", err)
	}

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create image file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("write image file: %w", err)
	}

	return nil
}

// importCards parses a CSV from reader, and inserts any cards not already in
// the database. For each new card, it attempts to download the image from
// imageBaseURL and save it to imagesDir. Downloads are rate-limited to 10 per
// second. If a download fails, the card is inserted with an empty image. If
// the image already exists on disk, the download is skipped. Cards that
// already exist in the database or appear more than once in the CSV are
// silently skipped. Returns an *importError with a status code of 400 for
// invalid CSV input or 500 for unexpected database errors.
func importCards(db *database.Database, httpClient *http.Client, imagesDir, imageBaseURL string, reader io.Reader) *importError {
	csvCards, err := parseCardsCSV(reader)
	if err != nil {
		slog.Error("failed to parse CSV", "error", err)
		return &importError{statusCode: http.StatusBadRequest, message: "invalid CSV: " + err.Error()}
	}

	if len(csvCards) == 0 {
		slog.Warn("CSV parsed successfully but contains no card rows")
		return &importError{statusCode: http.StatusBadRequest, message: "CSV contains no card rows"}
	}

	slog.Info("CSV parsed", "row_count", len(csvCards))

	// Track names seen in this request to avoid duplicate inserts.
	seen := make(map[string]bool, len(csvCards))

	// Track how many images have been downloaded in this request so that
	// the rate-limit sleep is applied correctly (only between downloads).
	downloadCount := 0

	insertedCount := 0
	skippedDBCount := 0
	skippedCSVCount := 0

	for _, csvCard := range csvCards {
		name := cardCSVToName(csvCard)

		if seen[name] {
			slog.Debug("skipping duplicate in CSV", "name", name)
			skippedCSVCount++
			continue
		}
		seen[name] = true

		exists, err := db.CardExistsByName(name)
		if err != nil {
			slog.Error("database error checking card existence", "name", name, "error", err)
			return &importError{statusCode: http.StatusInternalServerError, message: "database error"}
		}

		if exists {
			slog.Debug("skipping card already in database", "name", name)
			skippedDBCount++
			continue
		}

		imagePath := ""

		filePath, pathErr := buildImageFilePath(imagesDir, csvCard.Set, csvCard.CardNumber)
		if pathErr == nil {
			if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
				// Rate-limit: pause before every download after the first.
				if downloadCount > 0 {
					time.Sleep(imageDownloadInterval)
				}

				imageURL, urlErr := buildImageURL(imageBaseURL, csvCard.Set, csvCard.CardNumber)
				if urlErr == nil {
					slog.Info("downloading image", "name", name, "url", imageURL)
					if dlErr := downloadCardImage(httpClient, imageURL, filePath); dlErr == nil {
						slog.Info("image downloaded", "name", name, "path", filePath)
						imagePath = filePath
					} else {
						slog.Warn("image download failed, inserting card without image", "name", name, "error", dlErr)
					}
				} else {
					slog.Warn("could not build image URL", "name", name, "error", urlErr)
				}

				downloadCount++
			} else if statErr == nil {
				// Image already exists on disk; use its path directly.
				slog.Debug("image already on disk", "name", name, "path", filePath)
				imagePath = filePath
			}
		}

		slog.Info("inserting card", "name", name, "image_path", imagePath)
		if err := db.InsertCard(name, imagePath); err != nil {
			slog.Error("database error inserting card", "name", name, "error", err)
			return &importError{statusCode: http.StatusInternalServerError, message: "database error"}
		}
		insertedCount++
	}

	slog.Info("import complete",
		"inserted", insertedCount,
		"skipped_already_in_db", skippedDBCount,
		"skipped_duplicate_in_csv", skippedCSVCount,
	)

	return nil
}

// GetCardHandler returns an http.HandlerFunc that retrieves a single card by its
// integer id path parameter. Returns 200 OK with the card as JSON on success,
// 400 Bad Request for a missing or non-positive-integer id, 404 Not Found when
// no card with that id exists, and 500 Internal Server Error for database errors.
func GetCardHandler(db *database.Database) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		rawID := request.PathValue("id")
		if rawID == "" {
			http.Error(responseWriter, "id path parameter is required", http.StatusBadRequest)
			return
		}

		id, err := strconv.Atoi(rawID)
		if err != nil || id <= 0 {
			http.Error(responseWriter, "id must be a positive integer", http.StatusBadRequest)
			return
		}

		card, err := db.GetCardByID(id)
		if errors.Is(err, database.ErrCardNotFound) {
			http.Error(responseWriter, "card not found", http.StatusNotFound)
			return
		}
		if err != nil {
			slog.Error("database error fetching card", "id", id, "error", err)
			http.Error(responseWriter, "database error", http.StatusInternalServerError)
			return
		}

		responseWriter.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(responseWriter).Encode(card); err != nil {
			slog.Error("failed to encode card response", "id", id, "error", err)
			http.Error(responseWriter, "failed to encode response", http.StatusInternalServerError)
			return
		}
	}
}

// IncrementCardOwnedHandler returns an http.HandlerFunc that increments the
// owned count by 1 for the card identified by the id path parameter. Returns
// 204 No Content on success, 400 Bad Request for a missing or non-positive-integer
// id, 404 Not Found when no card with that id exists, and 500 Internal Server
// Error for database errors.
func IncrementCardOwnedHandler(db *database.Database) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		rawID := request.PathValue("id")
		if rawID == "" {
			http.Error(responseWriter, "id path parameter is required", http.StatusBadRequest)
			return
		}

		id, err := strconv.Atoi(rawID)
		if err != nil || id <= 0 {
			http.Error(responseWriter, "id must be a positive integer", http.StatusBadRequest)
			return
		}

		if err := db.IncrementCardOwned(id); errors.Is(err, database.ErrCardNotFound) {
			http.Error(responseWriter, "card not found", http.StatusNotFound)
			return
		} else if err != nil {
			slog.Error("database error incrementing owned count", "id", id, "error", err)
			http.Error(responseWriter, "database error", http.StatusInternalServerError)
			return
		}

		responseWriter.WriteHeader(http.StatusNoContent)
	}
}

// DecrementCardOwnedHandler returns an http.HandlerFunc that decrements the
// owned count by 1 for the card identified by the id path parameter, clamping
// at 0 so it never goes negative. Returns 204 No Content on success, 400 Bad
// Request for a missing or non-positive-integer id, 404 Not Found when no card
// with that id exists, and 500 Internal Server Error for database errors.
func DecrementCardOwnedHandler(db *database.Database) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		rawID := request.PathValue("id")
		if rawID == "" {
			http.Error(responseWriter, "id path parameter is required", http.StatusBadRequest)
			return
		}

		id, err := strconv.Atoi(rawID)
		if err != nil || id <= 0 {
			http.Error(responseWriter, "id must be a positive integer", http.StatusBadRequest)
			return
		}

		if err := db.DecrementCardOwned(id); errors.Is(err, database.ErrCardNotFound) {
			http.Error(responseWriter, "card not found", http.StatusNotFound)
			return
		} else if err != nil {
			slog.Error("database error decrementing owned count", "id", id, "error", err)
			http.Error(responseWriter, "database error", http.StatusInternalServerError)
			return
		}

		responseWriter.WriteHeader(http.StatusNoContent)
	}
}

// SearchCardsHandler returns an http.HandlerFunc that handles GET /cards/search.
// It reads the optional "q" query parameter and returns a JSON array of cards
// whose names contain the query as a case-insensitive substring. If "q" is
// absent or empty, all cards are returned. Always returns 200 OK with a JSON
// array (empty array when there are no results), or 500 Internal Server Error
// for database errors.
func SearchCardsHandler(db *database.Database) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		query := request.URL.Query().Get("q")

		matchedCards, err := db.SearchCards(query)
		if err != nil {
			slog.Error("database error searching cards", "query", query, "error", err)
			http.Error(responseWriter, "database error", http.StatusInternalServerError)
			return
		}

		responseWriter.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(responseWriter).Encode(matchedCards); err != nil {
			slog.Error("failed to encode search response", "query", query, "error", err)
			http.Error(responseWriter, "failed to encode response", http.StatusInternalServerError)
			return
		}
	}
}

// ImportCardsHandler returns an http.HandlerFunc that accepts a raw CSV body,
// parses it, and inserts any cards that do not already exist in the database.
// For each new card, the handler downloads its image from imageBaseURL and
// saves it to imagesDir/{Set}{CardNumber}.png. Downloads are rate-limited to
// 10 per second. If a download fails, the card is still inserted with an empty
// Image field. If an image file already exists on disk, the download is
// skipped. Cards that already exist (matched by name) are silently skipped.
// Cards that appear more than once in the same CSV are only inserted once.
// Returns 204 No Content on success, 400 Bad Request for invalid CSV, and
// 500 Internal Server Error for unexpected database errors.
func ImportCardsHandler(db *database.Database, httpClient *http.Client, imagesDir, imageBaseURL string) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		slog.Info("POST /cards/import received")

		if impErr := importCards(db, httpClient, imagesDir, imageBaseURL, request.Body); impErr != nil {
			slog.Error("import failed", "status", impErr.statusCode, "message", impErr.message)
			http.Error(responseWriter, impErr.message, impErr.statusCode)
			return
		}

		responseWriter.WriteHeader(http.StatusNoContent)
	}
}

// IndexHandler returns an http.HandlerFunc that serves the full index page at
// GET /. It loads all cards from the database and renders the index template.
// Returns 500 Internal Server Error if the database query or template
// rendering fails.
func IndexHandler(db *database.Database, tmpl *template.Template) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		slog.Info("GET / received")

		allCards, err := db.SearchCards("")
		if err != nil {
			slog.Error("database error loading cards for index", "error", err)
			http.Error(responseWriter, "database error", http.StatusInternalServerError)
			return
		}

		slog.Info("rendering index page", "card_count", len(allCards))

		responseWriter.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(responseWriter, "index", allCards); err != nil {
			slog.Error("failed to render index template", "error", err)
			http.Error(responseWriter, "template error", http.StatusInternalServerError)
			return
		}
	}
}

// SearchCardsHTMLHandler returns an http.HandlerFunc that handles
// GET /cards/search/html. It reads the optional "q" query parameter and
// renders the card grid partial template with matching cards. Used by htmx
// for live search updates. Returns 200 OK with HTML on success and 500
// Internal Server Error for database or template errors.
func SearchCardsHTMLHandler(db *database.Database, tmpl *template.Template) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		query := request.URL.Query().Get("q")

		matchedCards, err := db.SearchCards(query)
		if err != nil {
			slog.Error("database error searching cards for HTML response", "query", query, "error", err)
			http.Error(responseWriter, "database error", http.StatusInternalServerError)
			return
		}

		responseWriter.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(responseWriter, "cards", matchedCards); err != nil {
			slog.Error("failed to render cards template", "query", query, "error", err)
			http.Error(responseWriter, "template error", http.StatusInternalServerError)
			return
		}
	}
}

// ImportCardsHTMLHandler returns an http.HandlerFunc that accepts a
// multipart/form-data POST with a "file" field containing a CSV. It delegates
// to the shared importCards helper and, on success, responds with 200 OK and
// sets the HX-Trigger response header to "cardsImported" so htmx-listening
// elements can react. On failure it returns a human-readable error string for
// display in the UI.
func ImportCardsHTMLHandler(db *database.Database, httpClient *http.Client, imagesDir, imageBaseURL string) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		slog.Info("POST /cards/import/html received")

		if err := request.ParseMultipartForm(10 << 20); err != nil {
			slog.Error("failed to parse multipart form", "error", err)
			http.Error(responseWriter, "invalid form data", http.StatusBadRequest)
			return
		}

		file, fileHeader, err := request.FormFile("file")
		if err != nil {
			slog.Error("file field missing from import form", "error", err)
			http.Error(responseWriter, "file field is required", http.StatusBadRequest)
			return
		}
		defer file.Close()

		slog.Info("import file received", "filename", fileHeader.Filename, "size_bytes", fileHeader.Size)

		if impErr := importCards(db, httpClient, imagesDir, imageBaseURL, file); impErr != nil {
			slog.Error("import failed", "status", impErr.statusCode, "message", impErr.message)
			http.Error(responseWriter, impErr.message, impErr.statusCode)
			return
		}

		slog.Info("import succeeded, triggering cardsImported event")
		responseWriter.Header().Set("HX-Trigger", "cardsImported")
		responseWriter.WriteHeader(http.StatusOK)
	}
}

// IncrementCardOwnedHTMLHandler returns an http.HandlerFunc that increments
// the owned count by 1 for the card identified by the id path parameter and
// returns the updated owned-row fragment as HTML. Used by htmx for inline
// owned count updates. Returns 400 Bad Request for invalid id, 404 Not Found
// when no card exists, and 500 Internal Server Error for database or template
// errors.
func IncrementCardOwnedHTMLHandler(db *database.Database, tmpl *template.Template) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		rawID := request.PathValue("id")
		if rawID == "" {
			http.Error(responseWriter, "id path parameter is required", http.StatusBadRequest)
			return
		}

		id, err := strconv.Atoi(rawID)
		if err != nil || id <= 0 {
			http.Error(responseWriter, "id must be a positive integer", http.StatusBadRequest)
			return
		}

		slog.Info("incrementing owned count", "card_id", id)

		if err := db.IncrementCardOwned(id); errors.Is(err, database.ErrCardNotFound) {
			http.Error(responseWriter, "card not found", http.StatusNotFound)
			return
		} else if err != nil {
			slog.Error("database error incrementing owned count", "card_id", id, "error", err)
			http.Error(responseWriter, "database error", http.StatusInternalServerError)
			return
		}

		card, err := db.GetCardByID(id)
		if err != nil {
			slog.Error("database error fetching card after increment", "card_id", id, "error", err)
			http.Error(responseWriter, "database error", http.StatusInternalServerError)
			return
		}

		slog.Info("owned count incremented", "card_id", id, "owned", card.Owned)

		responseWriter.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(responseWriter, "card-owned-fragment", card); err != nil {
			slog.Error("failed to render card-owned-fragment template", "card_id", id, "error", err)
			http.Error(responseWriter, "template error", http.StatusInternalServerError)
			return
		}
	}
}

// DecrementCardOwnedHTMLHandler returns an http.HandlerFunc that decrements
// the owned count by 1 (clamped at 0) for the card identified by the id path
// parameter and returns the updated owned-row fragment as HTML. Used by htmx
// for inline owned count updates. Returns 400 Bad Request for invalid id,
// 404 Not Found when no card exists, and 500 Internal Server Error for
// database or template errors.
func DecrementCardOwnedHTMLHandler(db *database.Database, tmpl *template.Template) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		rawID := request.PathValue("id")
		if rawID == "" {
			http.Error(responseWriter, "id path parameter is required", http.StatusBadRequest)
			return
		}

		id, err := strconv.Atoi(rawID)
		if err != nil || id <= 0 {
			http.Error(responseWriter, "id must be a positive integer", http.StatusBadRequest)
			return
		}

		slog.Info("decrementing owned count", "card_id", id)

		if err := db.DecrementCardOwned(id); errors.Is(err, database.ErrCardNotFound) {
			http.Error(responseWriter, "card not found", http.StatusNotFound)
			return
		} else if err != nil {
			slog.Error("database error decrementing owned count", "card_id", id, "error", err)
			http.Error(responseWriter, "database error", http.StatusInternalServerError)
			return
		}

		card, err := db.GetCardByID(id)
		if err != nil {
			slog.Error("database error fetching card after decrement", "card_id", id, "error", err)
			http.Error(responseWriter, "database error", http.StatusInternalServerError)
			return
		}

		slog.Info("owned count decremented", "card_id", id, "owned", card.Owned)

		responseWriter.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(responseWriter, "card-owned-fragment", card); err != nil {
			slog.Error("failed to render card-owned-fragment template", "card_id", id, "error", err)
			http.Error(responseWriter, "template error", http.StatusInternalServerError)
			return
		}
	}
}
