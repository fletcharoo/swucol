package main

import (
	"flag"
	"fmt"
	"log"
	"swucol/store"
)

func main() {
	storeFilepath := "swucol.db.json"
	var importFile string
	flag.StringVar(&importFile, "import", "", "Import cards from SWUDB CSV file")
	flag.Parse()

	if importFile != "" {
		// Initialize a new store with a default path
		// If the file doesn't exist, it will create an empty store
		s, err := store.New(storeFilepath)
		if err != nil {
			log.Fatalf("Failed to initialize store: %v", err)
		}

		// Run the InsertSWUDBCSV function with the provided filename
		err = s.InsertSWUDBCSV(importFile)
		if err != nil {
			log.Fatalf("Failed to import CSV: %v", err)
		}

		fmt.Printf("Successfully imported cards from %s\n", importFile)
		return
	}

	fmt.Println("hello world")
}
