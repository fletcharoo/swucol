package main

import (
	"fmt"
	"net/http"
	"swucol/cards"
	"swucol/database"
)

// helloHandler responds with "hello world" for GET /hello requests.
func helloHandler(responseWriter http.ResponseWriter, request *http.Request) {
	fmt.Fprintln(responseWriter, "hello world")
}

func main() {
	db, err := database.New("./swucol.db")
	if err != nil {
		fmt.Printf("Failed to open database: %v\n", err)
		return
	}
	defer db.Shutdown()

	if err := db.RunMigrations(); err != nil {
		fmt.Printf("Failed to run database migrations: %v\n", err)
		return
	}

	fmt.Println("Database initialized successfully")

	http.HandleFunc("/hello", helloHandler)
	http.HandleFunc("/cards/import", cards.ImportCardsHandler(db))

	fmt.Println("Server listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}
