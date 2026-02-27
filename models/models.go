// Package models defines the shared data structures used across the application.
package models

// Card represents a card record stored in the database.
type Card struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Image string `json:"image"`
	Owned int    `json:"owned"`
}

// CardCSV represents a single row from a card collection CSV export.
// The fields map directly to the CSV column headers.
type CardCSV struct {
	Set             string
	CardNumber      string
	CardName        string
	CardTitle       string
	CardType        string
	Aspects         string
	VariantType     string
	Rarity          string
	Foil            string
	Stamp           string
	Artist          string
	OwnedCount      string
	GroupOwnedCount string
}
