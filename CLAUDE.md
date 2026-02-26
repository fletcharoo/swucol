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
- `Makefile`: Build and development automation tool.

### Project Structure
```text
.
├── README.md                 # Entry point for project, containing basic information and setup instructions.
├── LICENSE                   # The license file for project (MIT).
├── go.mod                    # Go module definition file.
├── go.sum                    # Go module dependencies lock file.
└── Makefile                  # Build and development automation commands.
```
