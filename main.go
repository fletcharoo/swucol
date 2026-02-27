package main

import (
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"swucol/cards"
	"swucol/database"
)

// helloHandler responds with "hello world" for GET /hello requests.
func helloHandler(responseWriter http.ResponseWriter, request *http.Request) {
	slog.Info("GET /hello received")
	responseWriter.Write([]byte("hello world\n"))
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	slog.Info("starting SWU Collection Manager")

	db, err := database.New("./swucol.db")
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Shutdown()

	if err := db.RunMigrations(); err != nil {
		slog.Error("failed to run database migrations", "error", err)
		os.Exit(1)
	}

	slog.Info("database initialized")

	tmpl, err := template.ParseGlob("templates/*.html")
	if err != nil {
		slog.Error("failed to load templates", "error", err)
		os.Exit(1)
	}

	slog.Info("templates loaded")

	// Serve card images from the local images directory.
	http.Handle("/images/", http.StripPrefix("/images/", http.FileServer(http.Dir("images"))))

	// JSON API routes.
	http.HandleFunc("/hello", helloHandler)
	http.HandleFunc("POST /cards/import", cards.ImportCardsHandler(db, http.DefaultClient, "images", "https://swudb.com/cdn-cgi/image/width=300/images/cards"))
	http.HandleFunc("GET /cards/search", cards.SearchCardsHandler(db))
	http.HandleFunc("GET /cards/{id}", cards.GetCardHandler(db))
	http.HandleFunc("POST /cards/{id}/increment", cards.IncrementCardOwnedHandler(db))
	http.HandleFunc("POST /cards/{id}/decrement", cards.DecrementCardOwnedHandler(db))

	// HTML / htmx routes.
	http.HandleFunc("GET /{$}", cards.IndexHandler(db, tmpl))
	http.HandleFunc("GET /cards/search/html", cards.SearchCardsHTMLHandler(db, tmpl))
	http.HandleFunc("POST /cards/import/html", cards.ImportCardsHTMLHandler(db, http.DefaultClient, "images", "https://swudb.com/cdn-cgi/image/width=300/images/cards"))
	http.HandleFunc("POST /cards/{id}/increment/html", cards.IncrementCardOwnedHTMLHandler(db, tmpl))
	http.HandleFunc("POST /cards/{id}/decrement/html", cards.DecrementCardOwnedHTMLHandler(db, tmpl))
	http.HandleFunc("GET /wishlist", cards.WishlistHandler(db, tmpl))
	http.HandleFunc("GET /wishlist/search/html", cards.SearchWishlistHTMLHandler(db, tmpl))

	slog.Info("server listening", "addr", ":8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
