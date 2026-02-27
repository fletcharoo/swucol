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
- `main.go`: Application entry point; initializes the SQLite database and registers HTTP routes.
- `models/models.go`: Shared data models used across packages (`Card` for database records, `CardCSV` for CSV import rows).
- `database/database.go`: SQLite wrapper providing connection management, schema migrations, and card query/insert operations.
- `cards/handler.go`: HTTP handler for `POST /cards/import` that parses a CSV body and persists new cards to the database.

### Project Structure
```text
.
├── README.md                    # Project overview and setup instructions.
├── LICENSE                      # MIT license.
├── CLAUDE.md                    # Project-specific AI assistant instructions.
├── Makefile                     # Build and development automation commands.
├── go.mod                       # Go module definition.
├── go.sum                       # Go module dependency lock file.
├── main.go                      # Application entry point; starts the HTTP server on :8080.
├── models/
│   └── models.go                # Shared data models: Card (database) and CardCSV (CSV import).
├── database/
│   ├── database.go              # SQLite wrapper: connection, migrations, and card operations.
│   └── database_test.go         # Tests for database initialization, migrations, and card operations.
└── cards/
    ├── handler.go               # HTTP handler for POST /cards/import; parses CSV and inserts cards.
    └── handler_test.go          # Behavioral tests for the card import endpoint.
```
