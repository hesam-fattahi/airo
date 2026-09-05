BINARY_NAME=airo
MAIN_PATH=cmd/main.go
KIND_CLUSTER_NAME=airo-cluster
KIND_CONFIG=deploy/kind-config.yaml
OPERATOR_IMAGE=airo:latest
PAYMENT_IMAGE=payment-service:v1.0

# Local bin directory for tools
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

# Controller-gen version for modern Go toolchain compatibility
CONTROLLER_TOOLS_VERSION ?= v0.16.5
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen

.PHONY: help build run test clean fmt fmt-check verify vet ci \
	cluster-up cluster-down cluster-status \
	port-forward-prom port-forward-grafana \
	controller-gen generate manifests \
	docker-build kind-load deploy-workload e2e-test

help: ## Display available commands
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary
$(CONTROLLER_GEN): $(LOCALBIN)
	@test -s $(LOCALBIN)/controller-gen || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

generate: controller-gen ## Generate code containing DeepCopy implementations
	@echo "Generating Go DeepCopy code..."
	@$(CONTROLLER_GEN) object paths="./..."

manifests: controller-gen ## Generate CustomResourceDefinition YAML objects
	@echo "Generating CRD manifests..."
	@mkdir -p config/crd/bases
	@$(CONTROLLER_GEN) crd:allowDangerousTypes=true paths="./..." output:crd:artifacts:config=config/crd/bases

build: ## Compile the operator binary
	@echo "Building binary..."
	@go build -o bin/$(BINARY_NAME) $(MAIN_PATH)

docker-build: ## Build Docker images for AIRO operator and payment service
	@echo "Building Docker image '$(OPERATOR_IMAGE)'..."
	@docker build -t $(OPERATOR_IMAGE) .
	@echo "Building Docker image '$(PAYMENT_IMAGE)'..."
	@docker build -t $(PAYMENT_IMAGE) -f examples/payment-service/Dockerfile .

kind-load: ## Load local Docker images into KinD cluster
	@echo "Loading images into KinD cluster '$(KIND_CLUSTER_NAME)'..."
	@kind load docker-image $(PAYMENT_IMAGE) --name $(KIND_CLUSTER_NAME)
	@kind load docker-image $(OPERATOR_IMAGE) --name $(KIND_CLUSTER_NAME)

deploy-workload: ## Apply CRDs, RBAC, monitoring, target workload, and AIRO operator
	@echo "Applying CRDs and RBAC manifests..."
	@kubectl apply -f config/crd/bases/
	@kubectl apply -f config/rbac/
	@echo "Applying monitoring stack..."
	@kubectl apply -f deploy/monitoring/
	@echo "Deploying payment microservice and AIRO operator..."
	@kubectl apply -f examples/payment-service/k8s-deployment.yaml
	@kubectl apply -f config/manager/deployment.yaml

e2e-test: docker-build kind-load deploy-workload ## Execute full E2E integration test sequence
	@echo "Applying sample RemediationPolicy..."
	@kubectl apply -f config/samples/payment_api_policy.yaml
	@echo "Waiting for Target Workload readiness..."
	@kubectl wait --for=condition=Available deployment/payment-api --timeout=120s
	@echo "Waiting for AIRO Operator rollout..."
	@kubectl rollout status deployment/airo-operator --timeout=60s
	@echo "Waiting for RemediationPolicy status phase to reach 'Healthy'..."
	@kubectl wait --for=jsonpath='{.status.phase}'=Healthy remediationpolicy/payment-api-policy --timeout=60s
	@echo "E2E Integration Test Completed Successfully!"

verify: ## Verify Go module dependencies and ensure go.mod is tidy
	@echo "Verifying dependencies..."
	@go mod tidy
	@go mod verify
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

test: ## Run unit tests with race detection and no caching
	@echo "Running unit tests..."
	@go test -v ./... -race -count=1

ci: fmt-check verify vet test build e2e-test ## Run all CI validation checks
	@echo "All CI checks passed."

run: ## Run the operator locally
	@go run $(MAIN_PATH)

cluster-up: ## Spin up local KinD multi-node cluster
	@echo "Spinning up KinD cluster '$(KIND_CLUSTER_NAME)'..."
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