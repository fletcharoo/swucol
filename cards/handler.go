// Package cards provides HTTP handlers for card-related API endpoints.
package cards

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"swucol/database"
	"swucol/models"
)

// csvColumnCount is the expected number of columns in a valid card CSV.
const csvColumnCount = 13

// csvHeaderSet is the value expected in the first column of the header row.
const csvHeaderSet = "Set"

// imageDownloadInterval is the minimum duration between image downloads to
// stay within the rate limit of 10 images per second.
const imageDownloadInterval = 100 * time.Millisecond

// parseCardsCSV reads a CSV from reader and returns a slice of CardCSV records.
// The first row must be the header row. Returns an error if the CSV is empty,
// malformed, or has an unexpected number of columns.
func parseCardsCSV(reader io.Reader) ([]models.CardCSV, error) {
	if reader == nil {
		return nil, errors.New("reader must not be nil")
	}

	csvReader := csv.NewReader(reader)

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
			http.Error(responseWriter, "database error", http.StatusInternalServerError)
			return
		}

		responseWriter.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(responseWriter).Encode(card); err != nil {
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
			http.Error(responseWriter, "database error", http.StatusInternalServerError)
			return
		}

		responseWriter.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(responseWriter).Encode(matchedCards); err != nil {
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
		csvCards, err := parseCardsCSV(request.Body)
		if err != nil {
			http.Error(responseWriter, "invalid CSV: "+err.Error(), http.StatusBadRequest)
			return
		}

		if len(csvCards) == 0 {
			http.Error(responseWriter, "CSV contains no card rows", http.StatusBadRequest)
			return
		}

		// Track names seen in this request to avoid duplicate inserts.
		seen := make(map[string]bool, len(csvCards))

		// Track how many images have been downloaded in this request so that
		// the rate-limit sleep is applied correctly (only between downloads).
		downloadCount := 0

		for _, csvCard := range csvCards {
			name := cardCSVToName(csvCard)

			if seen[name] {
				continue
			}
			seen[name] = true

			exists, err := db.CardExistsByName(name)
			if err != nil {
				http.Error(responseWriter, "database error", http.StatusInternalServerError)
				return
			}

			if exists {
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
						if dlErr := downloadCardImage(httpClient, imageURL, filePath); dlErr == nil {
							imagePath = filePath
						}
					}

					downloadCount++
				} else if statErr == nil {
					// Image already exists on disk; use its path directly.
					imagePath = filePath
				}
			}

			if err := db.InsertCard(name, imagePath); err != nil {
				http.Error(responseWriter, "database error", http.StatusInternalServerError)
				return
			}
		}

		responseWriter.WriteHeader(http.StatusNoContent)
	}
}
