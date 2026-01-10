package models

// The following are constant values used by SWUDB and found in SWUDB export CSVs.
const (
	SWUDBRarity_Special   = "Special"
	SWUDBRarity_Legendary = "Legendary"
	SWUDBRarity_Rare      = "Rare"
	SWUDBRarity_Uncommon  = "Uncommon"
	SWUDBRarity_Common    = "Common"

	SWUDBType_Leader  = "Leader"
	SWUDBType_Base    = "Base"
	SWUDBType_Unit    = "Unit"
	SWUDBType_Event   = "Event"
	SWUDBType_Upgrade = "Upgrade"

	SWUDBAspect_Vigilance  = "Vigilance"
	SWUDBAspect_Command    = "Command"
	SWUDBAspect_Aggression = "Aggression"
	SWUDBAspect_Cunning    = "Cunning"
	SWUDBAspect_Villainy   = "Villainy"
	SWUDBAspect_Heroism    = "Heroism"
)

// SWUDBCard matches the schema in CSV files when you click "Export CSV" in swudb.
type SWUDBCard struct {
	Set             string `csv:"Set"`
	CardNumber      string `csv:"Card Number"`
	CardName        string `csv:"Card Name"`
	CardTitle       string `csv:"Card Title"`
	CardType        string `csv:"Card Type"`
	Aspects         string `csv:"Aspects"`
	VariantType     string `csv:"Variant Type"`
	Rarity          string `csv:"Rarity"`
	Foil            string `csv:"Foil"`
	Stamp           string `csv:"Stamp"`
	Artist          string `csv:"Artist"`
	OwnedCount      string `csv:"Owned Count"`
	GroupOwnedCount string `csv:"Group Owned Count"`
}

// Card is the internal structure stored in the swucol database.
type Card struct {
	Name     string `json:"name"`
	Owned    int    `json:"owned"`
	Type     string `json:"type"`
	SWUDBURL string `json:"swudbURL"`
}

// SaveJSON is how swucol saves data to the file system
type SaveJSON struct {
	Collection []Card `json:"collection"`
}
