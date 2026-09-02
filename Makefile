BINARY_NAME=airo
MAIN_PATH=cmd/main.go

.PHONY: help build run test clean fmt fmt-check verify vet ci

## Display available commands
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

## Compile the operator binary
build: 
	@echo "Building binary..."
	@go build -o bin/$(BINARY_NAME) $(MAIN_PATH)

## Verify dependencies
verify:
	@echo "Verifying dependencies..."
	@go mod tidy
	@go mod verify

## Format Go source files
fmt: 
	@echo "Formatting Go source files..."
	@gofmt -w .

## Check Go source formatting
fmt-check:
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

## Run Go static analysis (go vet)
vet: 
	@echo "Running go static analysis..."
	@go vet ./...

## Run unit tests
test: 
	@echo "Running unit tests..."
	@go test -v ./... -race -count=1

## Run all CI validation checks
ci: fmt-check verify vet test build
	@echo "All CI checks passed."

## Run the operator locally
run: 
	@go run $(MAIN_PATH)

## Remove build artifacts
clean: 
	@echo "Cleaning..."
	@rm -rf bin/
	@echo "Done"

## TODO: Build example payment service
