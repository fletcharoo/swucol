package store

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"swucol/models"

	"github.com/gocarina/gocsv"
	"github.com/lithammer/fuzzysearch/fuzzy"
)

func New(filepath string) (*Store, error) {
	store := new(Store)
	store.filepath = filepath

	data, err := os.ReadFile(filepath)
	if err != nil {
		// If the file doesn't exist, create an empty collection
		if os.IsNotExist(err) {
			store.collection = []models.Card{}
			return store, nil
		}
		return nil, fmt.Errorf("failed to read file %q: %w", filepath, err)
	}

	if err = json.Unmarshal(data, &(store.collection)); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data into collection: %w", err)
	}

	// Populate names slice for fuzzy search
	store.refreshNames()

	return store, nil
}

type Store struct {
	filepath   string
	collection []models.Card
	names      []string
}

func (s *Store) Save() error {
	collectionJSON, err := json.Marshal(s.collection)
	if err != nil {
		return fmt.Errorf("failed to marshal collection: %w", err)
	}

	if err := os.WriteFile(s.filepath, collectionJSON, 0644); err != nil {
		return fmt.Errorf("failed to write file %q: %w", s.filepath, err)
	}

	s.refreshNames()

	return nil
}

func (s *Store) refreshNames() {
	collectionLen := len(s.collection)
	s.names = make([]string, collectionLen)
	for i, c := range s.collection {
		s.names[i] = c.Name
	}
}

func (s *Store) getCardByName(name string) (int, models.Card, error) {
	for i, c := range s.collection {
		if c.Name == name {
			return i, c, nil
		}
	}

	return 0, models.Card{}, fmt.Errorf("card not found: %q", name)
}

func (s *Store) Search(search string) ([]models.Card, error) {
	results := make([]models.Card, 0, len(s.names))
	matches := fuzzy.RankFindNormalizedFold(search, s.names)
	sort.Sort(matches)
	for _, m := range matches {
		_, card, err := s.getCardByName(m.Target)
		if err != nil {
			return nil, err
		}
		results = append(results, card)
	}

	return results, nil
}

func (s *Store) InsertSWUDBCard(swudbCard models.SWUDBCard) error {
	if swudbCard.CardName == "" {
		return fmt.Errorf("card name is empty")
	}
	if swudbCard.Set == "" {
		return fmt.Errorf("card set is empty")
	}
	if swudbCard.CardNumber == "" {
		return fmt.Errorf("card number is empty")
	}
	if swudbCard.CardType == "" {
		return fmt.Errorf("card rarity is empty")
	}
	// I have no need for keeping track of common bases in my collection.
	if swudbCard.CardType == models.SWUDBType_Base && swudbCard.Rarity == models.SWUDBRarity_Common {
		return nil
	}

	// Check if this card already exists in the collection.
	cardName := swudbCard.CardName
	if len(swudbCard.CardTitle) != 0 {
		cardName = fmt.Sprintf("%s, %s", swudbCard.CardName, swudbCard.CardTitle)
	}
	for _, c := range s.collection {
		if c.Name == cardName {
			return nil
		}
	}

	// Create card model and add it to collection.
	card := models.Card{
		Name:     cardName,
		Owned:    0, // If it's not in the database it's assumed we don't have any.
		Type:     swudbCard.CardType,
		SWUDBURL: fmt.Sprintf("https://swudb.com/card/%s/%s", swudbCard.Set, swudbCard.CardNumber),
	}

	s.collection = append(s.collection, card)
	return nil
}

func (s *Store) InsertSWUDBCSV(filepath string) error {
	// Read file.
	file, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("failed to open file %q: %w", filepath, err)
	}
	defer file.Close()

	// Unmarshal file.
	var swudbCollection []models.SWUDBCard
	if err = gocsv.UnmarshalFile(file, &swudbCollection); err != nil {
		err = fmt.Errorf("failed to unmarshal file: %w", err)
	}

	// Check if slice is still nil.
	if swudbCollection == nil {
		return fmt.Errorf("swudbCollection is nil")
	}

	// Insert each card individually.
	for _, c := range swudbCollection {
		if err = s.InsertSWUDBCard(c); err != nil {
			return fmt.Errorf("failed to insert card %q: %w", c.CardNumber, err)
		}
	}

	// Save the collection.
	if err = s.Save(); err != nil {
		return fmt.Errorf("failed to save: %w", err)
	}

	return nil
}

func (s *Store) IncrementCardOwned(name string) error {
	i, _, err := s.getCardByName(name)
	if err != nil {
		return fmt.Errorf("failed to get card %q: %w", name, err)
	}

	s.collection[i].Owned += 1
	s.Save()
	return nil
}

func (s *Store) DecrementCardOwned(name string) error {
	i, card, err := s.getCardByName(name)
	if err != nil {
		return fmt.Errorf("failed to get card %q: %w", name, err)
	}

	if card.Owned == 0 {
		return nil
	}

	s.collection[i].Owned -= 1
	s.Save()
	return nil
}
