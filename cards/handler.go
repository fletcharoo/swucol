// Package cards provides HTTP handlers for card-related API endpoints.
package cards

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"swucol/database"
	"swucol/models"
)

// csvColumnCount is the expected number of columns in a valid card CSV.
const csvColumnCount = 13

// csvHeaderSet is the value expected in the first column of the header row.
const csvHeaderSet = "Set"

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

// ImportCardsHandler returns an http.HandlerFunc that accepts a raw CSV body,
// parses it, and inserts any cards that do not already exist in the database.
// Cards that already exist (matched by name) are silently skipped. Cards that
// appear more than once in the same CSV are only inserted once.
// Returns 204 No Content on success, 400 Bad Request for invalid CSV, and
// 500 Internal Server Error for unexpected database errors.
func ImportCardsHandler(db *database.Database) http.HandlerFunc {
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

			if err := db.InsertCard(name); err != nil {
				http.Error(responseWriter, "database error", http.StatusInternalServerError)
				return
			}
		}

		responseWriter.WriteHeader(http.StatusNoContent)
	}
}
