BINARY_NAME=airo
MAIN_PATH=cmd/main.go
KIND_CLUSTER_NAME=airo-cluster
KIND_CONFIG=deploy/kind-config.yaml

# Local bin directory for tools
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

# Upgraded Controller-gen version for modern Go compatibility
CONTROLLER_TOOLS_VERSION ?= v0.16.2
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen

.PHONY: help build run test clean fmt fmt-check verify vet ci \
	cluster-up cluster-down cluster-status \
	port-forward-prom port-forward-grafana \
	controller-gen generate manifests

help: ## Display available commands
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary
$(CONTROLLER_GEN): $(LOCALBIN)
	@test -s $(LOCALBIN)/controller-gen || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject implementations
	@echo "Generating Go DeepCopy code..."
	@$(CONTROLLER_GEN) object paths="./..."

manifests: controller-gen ## Generate CustomResourceDefinition YAML objects
	@echo "Generating CRD manifests..."
	@mkdir -p config/crd/bases
	@$(CONTROLLER_GEN) crd:allowDangerousTypes=true paths="./..." output:crd:artifacts:config=config/crd/bases

build: generate manifests ## Compile the operator binary
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary
$(CONTROLLER_GEN): $(LOCALBIN)
	@test -s $(LOCALBIN)/controller-gen || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject implementations
	@echo "Generating Go DeepCopy code..."
	@$(CONTROLLER_GEN) object paths="./..."

manifests: controller-gen ## Generate CustomResourceDefinition YAML objects
	@echo "Generating CRD manifests..."
	@mkdir -p config/crd/bases
	@$(CONTROLLER_GEN) crd:allowDangerousTypes=true paths="./..." output:crd:artifacts:config=config/crd/bases

build: generate manifests ## Compile the operator binary
	@echo "Building binary..."
	@go build -o bin/$(BINARY_NAME) $(MAIN_PATH)

verify: ## Verify Go module dependencies and ensure go.mod is tidy
	@echo "Verifying dependencies..."
	@go mod tidy
	@go mod verify
	@git diff --exit-code go.mod go.sum || (echo "go.mod or go.sum is dirty! Run 'go mod tidy' locally." && exit 1)
	@git diff --exit-code go.mod go.sum || (echo "go.mod or go.sum is dirty! Run 'go mod tidy' locally." && exit 1)

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

test: generate ## Run unit tests with race detection and no caching
test: generate ## Run unit tests with race detection and no caching
	@echo "Running unit tests..."
	@go test -v ./... -race -count=1

ci: fmt-check verify vet test build ## Run all CI validation checks
	@echo "All CI checks passed."

run: generate ## Run the operator locally
run: generate ## Run the operator locally
	@go run $(MAIN_PATH)

cluster-up: ## Spin up local KinD multi-node cluster
	@echo "Spinning up KinD cluster '$(KIND_CLUSTER_NAME)'..."
	@kind create cluster --config $(KIND_CONFIG)
	@echo "Cluster is ready! Current nodes:"
	@kind create cluster --config $(KIND_CONFIG)
	@echo "Cluster is ready! Current nodes:"
	@kubectl get nodes

cluster-down: ## Destroy local KinD cluster
	@echo "Destroying KinD cluster '$(KIND_CLUSTER_NAME)'..."
	@kind delete cluster --name $(KIND_CLUSTER_NAME)

cluster-status: ## Check local KinD cluster status
	@kubectl get nodes -o wide

port-forward-prom: ## Port-forward Prometheus UI to http://localhost:9090
	@echo "Port-forwarding Prometheus UI to http://localhost:9090..."
	@kubectl port-forward -n monitoring svc/prometheus 9090:9090

port-forward-grafana: ## Port-forward Grafana UI to http://localhost:3000
	@echo "Port-forwarding Grafana UI to http://localhost:3000..."
	@kubectl port-forward -n monitoring svc/grafana 3000:3000

clean: ## Remove local binaries and tools
	@echo "Cleaning..."
	@rm -rf bin/
	@echo "Done."