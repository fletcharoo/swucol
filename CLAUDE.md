## Project Context

### Project Overview
SWU Collection Manager (swucol) is a tool being created for the user to help him manage his Star Wars: Unlimited collection. It is designed to be run locally on the user's machine and not be deployed or used by other people.

### Core Features
**Import Cards**
- Query the APIs of https://swudb.com/ to update the internal database with new cards.

**Collection Management**
- Keep track how many copies of each card the user has.

**Export Wishlist**
- Export the list of cards the user is missing.

### Decision Making
When implementing features, prioritize:
1. User experience and core functionality
2. Data consistency and security
3. Performance and scalability
4. Code maintainability

## Development Environment

### Developer Commands
When doing any of the following tasks, you **MUST** use the appropriate command:
- Build the service: `make build`
- Build and start the service: `make run`
- Run all tests `make test`
- Check test coverage: `make test/coverage`
- Format all Go code: `make fmt`

### Technology Stack
- **Backend:** Golang
- **Frontend:** htmx with Golang templating
- **Database:** sqlite
- **Testing:** `testify/assert` and `testify/require`

### Local Development Environment
- Application runs on port 8080

### Important Files
- `Makefile`: Build and development automation commands.
- `main.go`: Application entry point; configures structured logging (`slog`), initializes the SQLite database, loads HTML templates, registers all HTTP routes (JSON API and HTML/htmx), and serves card images as static files from the `images/` directory.
- `models/models.go`: Shared data models used across packages (`Card` for database records with `id`, `name`, `image`, `owned`, and `mainboard` fields; `WishlistCard` wrapping `Card` with a pre-computed `Deficit` field; `CardCSV` for CSV import rows).
- `database/database.go`: SQLite wrapper providing connection management, idempotent schema migrations (using `addColumnIfNotExists` to support incremental column additions), minimum owned constants (`MainboardMinimumOwned = 6`, `NonMainboardMinimumOwned = 3`), and card operations (insert with image path and mainboard flag, existence check, lookup by ID, case-insensitive name search, wishlist query filtered below minimum threshold, and increment/decrement owned count).
- `cards/handler.go`: All HTTP handlers. JSON API handlers (`POST /cards/import`, `GET /cards/search`, `GET /cards/{id}`, `POST /cards/{id}/increment`, `POST /cards/{id}/decrement`) and HTML/htmx handlers (`GET /`, `GET /cards/search/html`, `POST /cards/import/html`, `POST /cards/{id}/increment/html`, `POST /cards/{id}/decrement/html`, `GET /wishlist`, `GET /wishlist/search/html`). Helpers include `importCards` (CSV parsing with BOM stripping, deduplication, rate-limited image downloading, mainboard flag derivation), `cardCSVToMainboard`, and `computeWishlistCards` (converts `Card` slices to `WishlistCard` slices with pre-computed deficits). All handlers emit structured logs via `slog`.
- `templates/index.html`: Full page HTML shell (`{{define "index"}}`); renders the dark-themed UI with a sticky search bar, Import button, Wishlist nav link, server-side card grid, and CSV import `<dialog>`.
- `templates/cards.html`: Card grid partial (`{{define "cards"}}`); renders a list of card tiles or an empty-state message; used by htmx for live search responses on the collection page.
- `templates/card.html`: Card tile (`{{define "card-tile"}}`) and owned-count row fragment (`{{define "card-owned-fragment"}}`); the fragment is the htmx swap target for inline `+`/`-` owned count updates.
- `templates/wishlist.html`: Full page HTML shell (`{{define "wishlist"}}`); renders the dark-themed wishlist UI with a sticky search bar, Export button (copies filtered `{deficit}x {name}` lines to clipboard via JS), Collection nav link, and server-side wishlist card grid.
- `templates/wishlist-cards.html`: Wishlist card grid partial (`{{define "wishlist-cards"}}`); renders a list of read-only wishlist card tiles or an empty-state message; used by htmx for live search responses on the wishlist page.
- `templates/wishlist-card-tile.html`: Read-only wishlist card tile (`{{define "wishlist-card-tile"}}`); displays the card image, name, and deficit count ("Need: N more") with `data-wishlist-card`, `data-name`, and `data-deficit` attributes used by the export JS.
- `example_csv.csv`: Sample CSV in the format exported from swudb.com, used for manual import testing.

### Project Structure
```text
.
├── README.md                    # Project overview and setup instructions.
├── LICENSE                      # MIT license.
├── CLAUDE.md                    # Project-specific AI assistant instructions.
├── Makefile                     # Build and development automation commands.
├── go.mod                       # Go module definition.
├── go.sum                       # Go module dependency lock file.
├── main.go                      # Application entry point: configures slog, initializes the database, loads templates, registers routes, and serves static images.
├── example_csv.csv              # Sample card CSV in swudb.com export format for manual import testing.
├── images/                      # Downloaded card images stored as {Set}{CardNumber}.png; served at GET /images/.
├── models/
│   └── models.go                # Shared data models: Card (database record with id, name, image, owned, mainboard), WishlistCard (Card with pre-computed Deficit), and CardCSV (CSV import row).
├── database/
│   ├── database.go              # SQLite wrapper: connection, idempotent migrations (addColumnIfNotExists), minimum owned constants, InsertCard, CardExistsByName, SearchCards, GetWishlistCards, GetCardByID, and increment/decrement owned count.
│   └── database_test.go         # Behavioral tests for database initialization, migrations, and all card operations including image path storage, mainboard flag, search, wishlist threshold filtering, and owned count adjustments.
├── cards/
│   ├── handler.go               # All HTTP handlers (JSON API and HTML/htmx) plus helpers: importCards (CSV parsing with BOM stripping, deduplication, rate-limited image downloading, mainboard derivation), cardCSVToName, cardCSVToMainboard, and computeWishlistCards.
│   └── handler_test.go          # Behavioral tests for all card endpoints: CSV import (including BOM-prefixed files, duplicate skipping, image download/fallback, mainboard flag by card type), JSON API, HTML/htmx handlers (search, owned count fragments, import trigger), and wishlist handlers (threshold filtering, deficit computation, search).
└── templates/
    ├── index.html               # {{define "index"}}: full page shell with dark theme, search bar, Import dialog, Wishlist nav link, and server-rendered card grid.
    ├── cards.html               # {{define "cards"}}: card grid partial for htmx search swap responses on the collection page.
    ├── card.html                # {{define "card-tile"}} and {{define "card-owned-fragment"}}: card tile and inline owned-count row fragment for htmx +/- updates.
    ├── wishlist.html            # {{define "wishlist"}}: full page shell with dark theme, search bar, clipboard Export button, Collection nav link, and server-rendered wishlist card grid.
    ├── wishlist-cards.html      # {{define "wishlist-cards"}}: wishlist card grid partial for htmx search swap responses on the wishlist page.
    └── wishlist-card-tile.html  # {{define "wishlist-card-tile"}}: read-only card tile showing image, name, and deficit count with data attributes used by the export JS.
```
