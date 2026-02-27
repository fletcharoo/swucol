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
- `models/models.go`: Shared data models used across packages (`Card` for database records with `id`, `name`, `image`, `owned`, and `mainboard` fields; `CardCSV` for CSV import rows).
- `database/database.go`: SQLite wrapper providing connection management, idempotent schema migrations (using `addColumnIfNotExists` to support incremental column additions), and card operations (insert with image path and mainboard flag, existence check, lookup by ID, case-insensitive name search, and increment/decrement owned count).
- `cards/handler.go`: All HTTP handlers. JSON API handlers (`POST /cards/import`, `GET /cards/search`, `GET /cards/{id}`, `POST /cards/{id}/increment`, `POST /cards/{id}/decrement`) and HTML/htmx handlers (`GET /`, `GET /cards/search/html`, `POST /cards/import/html`, `POST /cards/{id}/increment/html`, `POST /cards/{id}/decrement/html`). The shared `importCards` helper handles CSV parsing (with UTF-8 BOM stripping), deduplication, rate-limited image downloading, mainboard flag derivation via `cardCSVToMainboard`, and database insertion. All handlers emit structured logs via `slog`.
- `templates/index.html`: Full page HTML shell (`{{define "index"}}`); renders the dark-themed UI with a sticky search bar, Import button, server-side card grid, and CSV import `<dialog>`.
- `templates/cards.html`: Card grid partial (`{{define "cards"}}`); renders a list of card tiles or an empty-state message; used by htmx for live search responses.
- `templates/card.html`: Card tile (`{{define "card-tile"}}`) and owned-count row fragment (`{{define "card-owned-fragment"}}`); the fragment is the htmx swap target for inline `+`/`-` owned count updates.
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
│   └── models.go                # Shared data models: Card (database record with id, name, image, owned, mainboard) and CardCSV (CSV import row).
├── database/
│   ├── database.go              # SQLite wrapper: connection, idempotent migrations (addColumnIfNotExists), InsertCard (with image path and mainboard flag), CardExistsByName, SearchCards, GetCardByID, and increment/decrement owned count.
│   └── database_test.go         # Behavioral tests for database initialization, migrations, and all card operations including image path storage, mainboard flag, search, and owned count adjustments.
├── cards/
│   ├── handler.go               # All HTTP handlers (JSON API and HTML/htmx) plus helpers: importCards (CSV parsing with BOM stripping, deduplication, rate-limited image downloading, mainboard derivation, slog logging), cardCSVToName, and cardCSVToMainboard.
│   └── handler_test.go          # Behavioral tests for all card endpoints: CSV import (including BOM-prefixed files, duplicate skipping, image download/fallback, mainboard flag by card type), JSON API, and HTML/htmx handlers (search, owned count fragments, import trigger).
└── templates/
    ├── index.html               # {{define "index"}}: full page shell with dark theme, search bar, Import dialog, and server-rendered card grid.
    ├── cards.html               # {{define "cards"}}: card grid partial for htmx search swap responses.
    └── card.html                # {{define "card-tile"}} and {{define "card-owned-fragment"}}: card tile and inline owned-count row fragment for htmx +/- updates.
```
