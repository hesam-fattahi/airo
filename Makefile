# ==============================================================================
# Shell & Make Configuration
# ==============================================================================
SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c

# ==============================================================================
# Project Configuration
# ==============================================================================
BINARY_NAME := airo
MAIN_PATH := ./cmd/main.go
LOCALBIN ?= $(CURDIR)/bin

# Executables
GO ?= go
DOCKER ?= docker
KUBECTL ?= kubectl
KIND ?= kind
HEY ?= hey

# Cluster & Deploy Configs
KIND_CLUSTER_NAME := airo-cluster
KIND_CONFIG := deploy/kind-config.yaml
PAYMENT_SERVICE_IMAGE := payment-service:v1.0
PAYMENT_SERVICE_DOCKERFILE := example/Dockerfile
PROMETHEUS_IMAGE := prom/prometheus:v2.51.0
GRAFANA_IMAGE := grafana/grafana:10.4.1

# Kubernetes Manifests & Namespaces
MONITORING_NAMESPACE := monitoring
CRD_DIR := config/crd/bases
MONITORING_DIR := deploy/monitoring
WORKLOAD_MANIFEST := example/payment-api.yaml
POLICY_MANIFEST := config/samples/payment_api_policy.yaml

# Code Generation Tools
CONTROLLER_TOOLS_VERSION ?= v0.16.5
CONTROLLER_GEN := $(LOCALBIN)/controller-gen

# Load and Chaos Test
TESTS_DIR := tests
LOAD_TEST := $(TESTS_DIR)/load-test.sh
CHAOS_TEST := $(TESTS_DIR)/chaos-test.sh
LOAD_DURATION ?= 300
LOAD_CONCURRENCY ?= 10
CHAOS_LATENCY_MS ?= 500
CHAOS_DURATION ?= 120


# ==============================================================================
# Help Menu
# ==============================================================================
.PHONY: help

help: ## Display available commands
	@grep -E '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
		sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-24s\033[0m %s\n", $$1, $$2}'


# ==============================================================================
# Tooling & Code Generation
# ==============================================================================
.PHONY: generate manifests generate-check

$(LOCALBIN):
	@mkdir -p "$(LOCALBIN)"

$(CONTROLLER_GEN): $(LOCALBIN)
	@test -s "$(CONTROLLER_GEN)" || \
		GOBIN="$(LOCALBIN)" $(GO) install \
		sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

generate: $(CONTROLLER_GEN) ## Generate Go DeepCopy implementation code
	@echo "Generating DeepCopy code..."
	@"$(CONTROLLER_GEN)" object paths="./..."

manifests: $(CONTROLLER_GEN) ## Generate CustomResourceDefinition manifests
	@echo "Generating CRD manifests..."
	@mkdir -p "$(CRD_DIR)"
	@"$(CONTROLLER_GEN)" crd:allowDangerousTypes=true paths="./..." output:crd:artifacts:config="$(CRD_DIR)"

generate-check: generate manifests ## Verify generated code and CRDs are up to date (CI step)
	@echo "Checking generated artifacts for drift..."
	@git diff --exit-code -- api "$(CRD_DIR)" || \
		(echo "Error: Generated files are out of date. Run 'make generate manifests' and commit." && exit 1)


# ==============================================================================
# Formatting & Static Checks
# ==============================================================================
.PHONY: fmt fmt-check tidy verify vet check

fmt: ## Format Go source files
	@echo "Formatting Go source files..."
	@gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

fmt-check: ## Verify Go source code formatting (CI step)
	@echo "Checking Go code formatting..."
	@files=$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*')); \
	if [ -n "$$files" ]; then \
		echo "Error: Code is not formatted. Run 'make fmt'."; \
		echo "$$files"; \
		exit 1; \
	fi

tidy: ## Tidy Go module dependencies
	@echo "Tidying Go modules..."
	@"$(GO)" mod tidy

verify: ## Verify Go module dependencies match go.mod / go.sum (CI step)
	@echo "Verifying Go module integrity..."
	@"$(GO)" mod verify
	@git diff --exit-code -- go.mod go.sum || \
		(echo "Error: go.mod or go.sum modified. Run 'make tidy' and commit." && exit 1)

vet: ## Run Go static analysis (CI step)
	@echo "Running go vet..."
	@"$(GO)" vet ./...

check: fmt-check generate-check verify vet ## Run all static checks and validations without mutating repository


# ==============================================================================
# Unit Testing
# ==============================================================================
.PHONY: unit-test test

unit-test: ## Run unit tests with race detection (CI step)
	@echo "Running unit tests..."
	@"$(GO)" test -race -count=1 ./...

test: unit-test ## Alias for unit-test


# ==============================================================================
# Build
# ==============================================================================
.PHONY: build

build: ## Build the AIRO operator binary without mutating source artifacts (CI step)
	@echo "Building $(BINARY_NAME) binary..."
	@mkdir -p bin
	@"$(GO)" build -o "bin/$(BINARY_NAME)" $(MAIN_PATH)


# ==============================================================================
# CI Pipeline
# ==============================================================================
.PHONY: ci

ci: check unit-test build ## Run full validation pipeline
	@echo "All CI pipeline checks passed successfully."


# ==============================================================================
# KinD Cluster Lifecycle
# ==============================================================================
.PHONY: cluster-up cluster-down cluster-status

cluster-up: ## Create local KinD cluster if it does not exist
	@if "$(KIND)" get clusters | grep -qx "$(KIND_CLUSTER_NAME)"; then \
		echo "KinD cluster '$(KIND_CLUSTER_NAME)' already exists."; \
	else \
		echo "Creating KinD cluster '$(KIND_CLUSTER_NAME)'..."; \
		"$(KIND)" create cluster --name "$(KIND_CLUSTER_NAME)" --config "$(KIND_CONFIG)"; \
	fi
	@"$(KUBECTL)" get nodes

cluster-down: ## Delete local KinD cluster
	@echo "Deleting KinD cluster '$(KIND_CLUSTER_NAME)'..."
	@"$(KIND)" delete cluster --name "$(KIND_CLUSTER_NAME)"

cluster-status: ## Display cluster node status
	@"$(KUBECTL)" get nodes -o wide


# ==============================================================================
# Container Images
# ==============================================================================
.PHONY: image-build image-load-app image-load-prometheus image-load-grafana image-load

image-build: ## Build payment-service container image
	@echo "Building payment-service image..."
	@"$(DOCKER)" build -t "$(PAYMENT_SERVICE_IMAGE)" -f "$(PAYMENT_SERVICE_DOCKERFILE)" .

image-load-app: ## Load images into KinD cluster,except Grafana (fails fast on missing images)
	@echo "Loading payment-service image into KinD..."
	@"$(KIND)" load docker-image "$(PAYMENT_SERVICE_IMAGE)" --name "$(KIND_CLUSTER_NAME)"

image-load-prometheus: ## Load Prometheus image into KinD cluster
	@echo "Loading Prometheus image into KinD..."
	@docker image save --platform linux/amd64 --output /tmp/prometheus.tar "$(PROMETHEUS_IMAGE)"
	@"$(KIND)" load image-archive /tmp/prometheus.tar --name "$(KIND_CLUSTER_NAME)"

image-load-grafana: ## Load Grafana image into KinD cluster
	@echo "Loading Grafana image into KinD..."
	@docker image save --platform linux/amd64 --output /tmp/grafana.tar "$(GRAFANA_IMAGE)"
	@"$(KIND)" load image-archive /tmp/grafana.tar --name "$(KIND_CLUSTER_NAME)"

## Separated Grafana image load from others, because the image-load is used in CI pipeline,
## Grafana adds unnecessary overhead to the CI pipeline. 
## 
image-load: image-load-app image-load-prometheus ## Load all required images into KinD cluster

# ==============================================================================
# Kubernetes Deployment
# ==============================================================================
.PHONY: namespace-create deploy-crds deploy-monitoring deploy-workload deploy-policy deploy wait-workload

namespace-create: ## Ensure monitoring namespace exists
	@echo "Ensuring monitoring namespace exists..."
	@"$(KUBECTL)" create namespace "$(MONITORING_NAMESPACE)" --dry-run=client -o yaml | "$(KUBECTL)" apply -f -

deploy-crds: manifests ## Deploy AIRO CRDs
	@echo "Applying CRDs..."
	@"$(KUBECTL)" apply -f "$(CRD_DIR)"

deploy-monitoring: namespace-create ## Deploy monitoring stack
	@echo "Applying monitoring manifests..."
	@"$(KUBECTL)" apply -f "$(MONITORING_DIR)"

deploy-workload: ## Deploy payment-api workload
	@echo "Applying workload manifests..."
	@"$(KUBECTL)" apply -f "$(WORKLOAD_MANIFEST)"

deploy-policy: deploy-crds deploy-workload ## Deploy sample RemediationPolicy
	@echo "Applying remediation policy manifests..."
	@"$(KUBECTL)" apply -f "$(POLICY_MANIFEST)"

deploy: deploy-crds deploy-monitoring deploy-workload deploy-policy ## Deploy full application stack

wait-workload: ## Wait for payment-api deployment to become available
	@echo "Waiting for payment-api deployment readiness..."
	@"$(KUBECTL)" wait --for=condition=Available deployment/payment-api --timeout=120s


# ==============================================================================
# Local Development Runtime
# ==============================================================================
.PHONY: dev-setup run-monitoring run-operator

dev-setup: cluster-up image-build image-load deploy wait-workload ## Provision full local development environment
	@echo "Development cluster environment ready."

run-monitoring: ## Port-forward Prometheus (9090) and Grafana (3000)
	@echo "Starting monitoring port-forwards (Prometheus: 9090, Grafana: 3000)..."
	@"$(KUBECTL)" port-forward -n "$(MONITORING_NAMESPACE)" svc/prometheus 9090:9090 & \
	"$(KUBECTL)" port-forward -n "$(MONITORING_NAMESPACE)" svc/grafana 3000:3000

run-operator: ## Run the AIRO operator locally against cluster
	@echo "Starting AIRO operator..."
	@"$(GO)" run "$(MAIN_PATH)" --prometheus-url=http://localhost:9090


# ==============================================================================
# Integration / E2E Verification
# ==============================================================================
.PHONY: e2e-test e2e

e2e-test: ## Verify active Kubernetes environment health
	@echo "Running Kubernetes E2E checks..."
	@"$(KUBECTL)" get crd remediationpolicies.reliability.airo.io
	@"$(KUBECTL)" rollout status deployment/payment-api --timeout=120s
	@"$(KUBECTL)" get pods -l app=payment-api
	@"$(KUBECTL)" get remediationpolicies
	@echo "E2E verification succeeded."

e2e: dev-setup e2e-test ## Provision fresh environment and run E2E verification


# ==============================================================================
# Load and Chaos Testing
# ==============================================================================
.PHONY: load-test chaos-test

load-test: ## Generate sustained traffic against payment-api using hey
	@echo "Generating traffic against payment-api..."
	@"$(HEY)" -z 10m -q 1000 -c 20 -m POST \
		"$(PAYMENT_SERVICE_URL)/api/v1/pay"

chaos-test: ## Inject latency into one payment-api pod
	@echo "Injecting latency to pod x..."


# ==============================================================================
# Cleanup
# ==============================================================================
.PHONY: clean

clean: ## Remove local build artifacts
	@echo "Cleaning local build artifacts..."
	@rm -rf bin
	@echo "Clean completed."