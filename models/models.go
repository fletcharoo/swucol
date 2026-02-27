package models

// Card represents a card record stored in the database.
type Card struct {
	Name  string
	Image string
	Owned int
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
