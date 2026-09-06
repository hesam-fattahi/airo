# ==============================================================================
# Project Configuration
# ==============================================================================

BINARY_NAME := airo
MAIN_PATH := ./cmd/main.go

LOCALBIN ?= $(shell pwd)/bin

KIND_CLUSTER_NAME := airo-cluster
KIND_CONFIG := deploy/kind-config.yaml

PAYMENT_SERVICE_IMAGE := payment-service:v1.0
PAYMENT_SERVICE_DOCKERFILE := examples/payment-service/Dockerfile

PROMETHEUS_IMAGE := prom/prometheus:v2.51.0
GRAFANA_IMAGE := grafana/grafana:10.4.0

MONITORING_NAMESPACE := monitoring

CRD_DIR := config/crd/bases
MONITORING_DIR := deploy/monitoring
WORKLOAD_MANIFEST := examples/payment-service/k8s-deployment.yaml
POLICY_MANIFEST := config/samples/payment_api_policy.yaml

CONTROLLER_TOOLS_VERSION ?= v0.16.5
CONTROLLER_GEN := $(LOCALBIN)/controller-gen


# ==============================================================================
# Tooling
# ==============================================================================

$(LOCALBIN):
	mkdir -p $(LOCALBIN)

$(CONTROLLER_GEN): $(LOCALBIN)
	@test -s $(CONTROLLER_GEN) || \
		GOBIN=$(LOCALBIN) go install \
		sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)


# ==============================================================================
# Phony Targets
# ==============================================================================

.PHONY: \
	help \
	generate manifests \
	build run clean \
	fmt fmt-check verify vet test ci \
	cluster-up cluster-down cluster-status \
	image-build image-load \
	namespace-create deploy deploy-crds deploy-monitoring deploy-workload deploy-policy \
	wait-workload \
	dev-setup dev-run dev-demo \
	e2e-test \
	port-forward-prom port-forward-grafana


# ==============================================================================
# Help
# ==============================================================================

help: ## Display available commands
	@grep -E '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
		sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-24s\033[0m %s\n", $$1, $$2}'


# ==============================================================================
# Code Generation
# ==============================================================================

generate: $(CONTROLLER_GEN) ## Generate DeepCopy implementations
	@echo "Generating Go DeepCopy code..."
	@$(CONTROLLER_GEN) object paths="./..."

manifests: $(CONTROLLER_GEN) ## Generate CustomResourceDefinition manifests
	@echo "Generating CRD manifests..."
	@mkdir -p $(CRD_DIR)
	@$(CONTROLLER_GEN) \
		crd:allowDangerousTypes=true \
		paths="./..." \
		output:crd:artifacts:config=$(CRD_DIR)


# ==============================================================================
# Build
# ==============================================================================

build: generate manifests ## Build the AIRO operator binary
	@echo "Building AIRO..."
	@mkdir -p bin
	@go build -o bin/$(BINARY_NAME) $(MAIN_PATH)

run: generate ## Run the AIRO operator locally
	@go run $(MAIN_PATH)

clean: ## Remove generated local build artifacts and tools
	@echo "Cleaning local artifacts..."
	@rm -rf bin
	@echo "Done."


# ==============================================================================
# Code Quality
# ==============================================================================

fmt: ## Format Go source files
	@echo "Formatting Go source files..."
	@gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

fmt-check: ## Check Go source formatting
	@echo "Checking Go source formatting..."
	@files=$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*')); \
	if [ -n "$$files" ]; then \
		echo "Code is not formatted. Run 'make fmt'."; \
		echo "$$files"; \
		exit 1; \
	fi; \
	echo "Code is clean."

verify: ## Verify Go module dependencies and module cleanliness
	@echo "Verifying Go module dependencies..."
	@go mod tidy
	@go mod verify
	@git diff --exit-code -- go.mod go.sum || \
		(echo "go.mod or go.sum changed. Run 'go mod tidy' and commit the result." && exit 1)

vet: ## Run Go static analysis
	@echo "Running go vet..."
	@go vet ./...

test: ## Run unit tests with race detection
	@echo "Running unit tests..."
	@go test -race -count=1 ./...

ci: fmt-check verify vet test build ## Run all local CI validation checks
	@echo "All CI checks passed."


# ==============================================================================
# KinD Cluster Lifecycle
# ==============================================================================

cluster-up: ## Create the local KinD cluster
	@echo "Creating KinD cluster '$(KIND_CLUSTER_NAME)'..."
	@kind create cluster \
		--name $(KIND_CLUSTER_NAME) \
		--config $(KIND_CONFIG)
	@kubectl get nodes

cluster-down: ## Delete the local KinD cluster
	@echo "Deleting KinD cluster '$(KIND_CLUSTER_NAME)'..."
	@kind delete cluster --name $(KIND_CLUSTER_NAME)

cluster-status: ## Display KinD cluster node status
	@kubectl get nodes -o wide


# ==============================================================================
# Container Images
# ==============================================================================

image-build: ## Build the payment-service container image
	@echo "Building payment-service image..."
	@docker build \
		-t $(PAYMENT_SERVICE_IMAGE) \
		-f $(PAYMENT_SERVICE_DOCKERFILE) \
		.

image-load: ## Load required images into the KinD cluster
	@echo "Loading payment-service image into KinD..."
	@kind load docker-image \
		$(PAYMENT_SERVICE_IMAGE) \
		--name $(KIND_CLUSTER_NAME)

	@echo "Loading monitoring images into KinD..."
	@kind load docker-image \
		$(PROMETHEUS_IMAGE) \
		--name $(KIND_CLUSTER_NAME) || true

	@kind load docker-image \
		$(GRAFANA_IMAGE) \
		--name $(KIND_CLUSTER_NAME) || true


# ==============================================================================
# Kubernetes Deployment
# ==============================================================================

namespace-create: ## Create required Kubernetes namespaces
	@echo "Ensuring monitoring namespace exists..."
	@kubectl create namespace $(MONITORING_NAMESPACE) \
		--dry-run=client \
		-o yaml | \
		kubectl apply -f -

deploy-crds: manifests ## Deploy AIRO CustomResourceDefinitions
	@kubectl apply -f $(CRD_DIR)

deploy-monitoring: namespace-create ## Deploy monitoring stack
	@kubectl apply -f $(MONITORING_DIR)

deploy-workload: ## Deploy payment-api workload
	@kubectl apply -f $(WORKLOAD_MANIFEST)

deploy-policy: ## Deploy sample RemediationPolicy
	@kubectl apply -f $(POLICY_MANIFEST)

deploy: deploy-crds deploy-monitoring deploy-workload deploy-policy ## Deploy complete AIRO test environment

wait-workload: ## Wait for payment-api deployment readiness
	@echo "Waiting for payment-api deployment..."
	@kubectl wait \
		--for=condition=Available \
		deployment/payment-api \
		--timeout=120s


# ==============================================================================
# Local Development Environment
# ==============================================================================

dev-setup: cluster-up image-build image-load deploy wait-workload ## Create and deploy complete local development environment
	@echo "AIRO development environment is ready."

dev-run: generate ## Start port-forwards and run AIRO locally
	@echo "Starting Prometheus port-forward..."
	@kubectl port-forward \
		-n $(MONITORING_NAMESPACE) \
		svc/prometheus 9090:9090 >/dev/null 2>&1 &

	@echo "Starting Grafana port-forward..."
	@kubectl port-forward \
		-n $(MONITORING_NAMESPACE) \
		svc/grafana 3000:3000 >/dev/null 2>&1 &

	@echo "Starting payment-api port-forward..."
	@kubectl port-forward \
		svc/payment-api 8080:8080 >/dev/null 2>&1 &

	@echo "Starting AIRO operator..."
	@go run $(MAIN_PATH) \
		--prometheus-url=http://localhost:9090

dev-demo: ## Run the payment-service chaos demonstration
	@./examples/payment-service/chaos-test.sh


# ==============================================================================
# Integration / E2E Testing
# ==============================================================================

e2e-test: image-build image-load deploy wait-workload ## Run Kubernetes integration verification
	@echo "Running Kubernetes integration verification..."

	@kubectl get crd remediationpolicies.reliability.airo.io

	@kubectl rollout status \
		deployment/payment-api \
		--timeout=120s

	@kubectl get pods \
		-l app=payment-api

	@kubectl get remediationpolicies

	@echo "KinD integration test passed."


# ==============================================================================
# Observability
# ==============================================================================

port-forward-prom: ## Port-forward Prometheus to localhost:9090
	@kubectl port-forward \
		-n $(MONITORING_NAMESPACE) \
		svc/prometheus 9090:9090

port-forward-grafana: ## Port-forward Grafana to localhost:3000
	@kubectl port-forward \
		-n $(MONITORING_NAMESPACE) \
		svc/grafana 3000:3000