# Makefile

default: help

.PHONY: help
help: ## Show this help.
	@egrep '^(.+)\:\ .*##\ (.+)' ${MAKEFILE_LIST} | sed 's/:.*##/#/' | column -t -c 2 -s '#'

.PHONY: test
test: ## Run all tests.
	go test -count 1 ./...

.PHONY: test/coverage
test/coverage: ## Generate test coverage report.
	@if [ -f temp/coverage.html ]; then rm temp/coverage.html; fi
	go test -v -coverprofile=temp/coverage.out ./... && go tool cover -html=temp/coverage.out -o temp/coverage.html

.PHONY: build
build: ## Builds the executable
	go build -o swucol

.PHONY: run
run: ## Builds and runs the executable
	go build -o swucol
	./swucol

.PHONY: fmt
fmt: ## Run go formatter.
	go fmt ./...
