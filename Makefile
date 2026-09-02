BINARY_NAME=airo
MAIN_PATH=cmd/main.go

.PHONY: help build run test clean fmt fmt-check verify vet ci

help: ## Display available commands
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Compile the operator binary
	@echo "Building binary..."
	@go build -o bin/$(BINARY_NAME) $(MAIN_PATH)

verify: ## Verify Go module dependencies and ensure go.mod is tidy
	@echo "Verifying dependencies..."
	@go mod tidy
	@go mod verify

fmt: ## Format Go source files
	@echo "Formatting Go source files..."
	@gofmt -w .

fmt-check: ## Check Go source formatting
	@echo "Checking Go source formatting..."
	@files=$$(gofmt -l .); \
	if [ -n "$$files" ]; then \
		echo "Code is not formatted. The following files need formatting:"; \
		echo "$$files"; \
		gofmt -d $$(echo "$$files" | head -n 1); \
		exit 1; \
	else \
		echo "Code is clean."; \
		exit 0; \
	fi

vet: ## Run Go static analysis (go vet)
	@echo "Running go static analysis..."
	@go vet ./...

test: ## Run unit tests with race detection and no caching
	@echo "Running unit tests..."
	@go test -v ./... -race -count=1

ci: fmt-check verify vet test build ## Run all CI validation checks
	@echo "All CI checks passed."

run: ## Run the operator locally
	@go run $(MAIN_PATH)

clean: ## Remove build artifacts
	@echo "Cleaning..."
	@rm -rf bin/
	@echo "Done."