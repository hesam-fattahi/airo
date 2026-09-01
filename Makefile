BINARY_NAME=airo-operator
MAIN_PATH=cmd/main.go

.PHONY: help build run test clean

## Display available commands
help: 
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'


## Compile the operator binary
build: 
	@echo "Building binary..."
	@go build -o bin/$(BINARY_NAME) $(MAIN_PATH)


## Run the operator locally
run: 
	@go run $(MAIN_PATH)


## Run unit tests
test: 
	@go test -v ./...


## Remove build artifacts
clean: 
	@rm -rf bin/