SHELL := /bin/bash

PROJECT_NAME := airo
BINARY_NAME := airo
MAIN_PATH := ./cmd/main.go

GO ?= go
GOFMT ?= gofmt
HELM ?= helm
KUBECTL ?= kubectl
KIND ?= kind
DOCKER ?= docker
TRIVY ?= trivy
GOVULNCHECK ?= govulncheck

KIND_CLUSTER_NAME := airo-cluster
KIND_CONFIG := deploy/kind/kind-config.yaml

AIRO_IMAGE := airo:latest
DEMO_API_IMAGE := demo-api:v1.0

HELM_CHART := ./charts/airo
HELM_RELEASE := airo
HELM_NAMESPACE := airo

PROMETHEUS_URL := http://prometheus.monitoring.svc:9090

PROMETHEUS_DIR := deploy/prometheus
GRAFANA_DIR := deploy/grafana
DEMO_API_DIR := examples/demo-api

CRD_DIR := config/crd/bases
HELM_CRD_DIR := $(HELM_CHART)/crds

LOCALBIN := $(shell pwd)/bin
CONTROLLER_TOOLS_VERSION := v0.16.5
CONTROLLER_GEN := $(LOCALBIN)/controller-gen

.PHONY: all
all: check

.PHONY: help
help: ## Show available Make targets
	@echo "AIRO development commands:"
	@echo
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make <target>\n\nTargets:\n"} \
		/^[a-zA-Z0-9_-]+:.*##/ {printf "  %-28s %s\n", $$1, $$2}' \
		$(MAKEFILE_LIST)

# --------------------------------------------------------------------
# Code quality and validation
# --------------------------------------------------------------------

.PHONY: fmt fmt-check vet lint helm-lint helm-template
fmt: ## Format all Go source files
	$(GOFMT) -w $$(find . -name '*.go' -not -path './vendor/*')

fmt-check: ## Check that all Go source files are formatted
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*'))" \
		|| (echo "Go files are not formatted:" && \
		    gofmt -l $$(find . -name '*.go' -not -path './vendor/*') && \
		    exit 1)

vet: ## Run go vet
	$(GO) vet ./...

lint: ## Run golangci-lint
	golangci-lint run ./...

helm-lint: ## Validate the AIRO Helm chart
	$(HELM) lint $(HELM_CHART)

helm-template: ## Render the AIRO Helm chart locally
	$(HELM) template \
		$(HELM_RELEASE) \
		$(HELM_CHART) \
		--namespace $(HELM_NAMESPACE) \
		--set prometheus.url=$(PROMETHEUS_URL)

# --------------------------------------------------------------------
# Generated manifests
# --------------------------------------------------------------------

.PHONY: manifests generate-check
manifests: controller-gen ## Generate CRD manifests and sync them to the Helm chart
	$(CONTROLLER_GEN) crd:allowDangerousTypes=true paths="./api/..." output:crd:artifacts:config=$(CRD_DIR)
	@mkdir -p $(HELM_CRD_DIR)
	@cp $(CRD_DIR)/*.yaml $(HELM_CRD_DIR)/

generate-check: ## Verify generated CRD manifests are up to date
	$(MAKE) manifests
	@git diff --exit-code -- \
		$(CRD_DIR) \
		$(HELM_CRD_DIR)

# --------------------------------------------------------------------
# Tests and security
# --------------------------------------------------------------------

.PHONY: unit-test vuln-check image-scan
unit-test: ## Run unit tests with the race detector
	$(GO) test -race -count=1 -coverprofile=coverage.out ./...

vuln-check: ## Scan Go dependencies for known vulnerabilities
	$(GOVULNCHECK) ./...

image-scan: ## Scan the AIRO container image with Trivy
	$(TRIVY) image $(AIRO_IMAGE)

# --------------------------------------------------------------------
# Verification
# --------------------------------------------------------------------

.PHONY: verify check
verify: ## Run static analysis and Helm validation
	$(MAKE) vet
	$(MAKE) helm-lint
	$(MAKE) helm-template

check: ## Run formatting, generation, lint, and verification checks
	$(MAKE) fmt-check
	$(MAKE) generate-check
	$(MAKE) lint
	$(MAKE) verify

# --------------------------------------------------------------------
# Local builds
# --------------------------------------------------------------------

.PHONY: build image-build demo-api-image-build
build: ## Build the AIRO executable locally
	$(GO) build -o $(BINARY_NAME) $(MAIN_PATH)

image-build: ## Build the AIRO container image
	$(DOCKER) build -t $(AIRO_IMAGE) .

demo-api-image-build: ## Build the demo-api container image
	$(DOCKER) build \
		-f $(DEMO_API_DIR)/Dockerfile \
		-t $(DEMO_API_IMAGE) \
		.

# --------------------------------------------------------------------
# KinD
# --------------------------------------------------------------------

.PHONY: cluster-up cluster-down cluster-recreate image-load demo-api-image-load
cluster-up: ## Create the local KinD cluster
	$(KIND) create cluster \
		--name $(KIND_CLUSTER_NAME) \
		--config $(KIND_CONFIG)

cluster-down: ## Delete the local KinD cluster
	$(KIND) delete cluster --name $(KIND_CLUSTER_NAME)

cluster-recreate: ## Delete and recreate the local KinD cluster
	$(MAKE) cluster-down
	$(MAKE) cluster-up

image-load: ## Load the AIRO image into the KinD cluster
	$(KIND) load docker-image $(AIRO_IMAGE) \
		--name $(KIND_CLUSTER_NAME)

demo-api-image-load: ## Load the demo-api image into the KinD cluster
	$(KIND) load docker-image $(DEMO_API_IMAGE) \
		--name $(KIND_CLUSTER_NAME)

# --------------------------------------------------------------------
# Local monitoring
# --------------------------------------------------------------------

.PHONY: deploy-monitoring
deploy-monitoring: ## Deploy Prometheus and Grafana to the cluster
	$(KUBECTL) create namespace monitoring --dry-run=client -o yaml | \
		$(KUBECTL) apply -f -
	$(KUBECTL) apply -f $(PROMETHEUS_DIR)
	$(KUBECTL) apply -f $(GRAFANA_DIR)

# --------------------------------------------------------------------
# AIRO deployment
# --------------------------------------------------------------------

.PHONY: deploy-airo
deploy-airo: helm-lint ## Install the AIRO CRD and deploy AIRO with Helm
	$(KUBECTL) apply -f $(CRD_DIR)
	$(HELM) upgrade --install \
		$(HELM_RELEASE) \
		$(HELM_CHART) \
		--namespace $(HELM_NAMESPACE) \
		--create-namespace \
		--set image.repository=airo \
		--set image.tag=latest \
		--set image.pullPolicy=IfNotPresent \
		--set prometheus.url=$(PROMETHEUS_URL)

# --------------------------------------------------------------------
# Demo workload
# --------------------------------------------------------------------

.PHONY: deploy-workload
deploy-workload: ## Deploy demo-api, Fortio load generation, and remediation policy
	$(KUBECTL) apply -f $(DEMO_API_DIR)/deployment.yaml
	$(KUBECTL) apply -f $(DEMO_API_DIR)/service.yaml
	$(KUBECTL) apply -f $(DEMO_API_DIR)/load-generator.yaml
	$(KUBECTL) apply -f $(DEMO_API_DIR)/remediation-policy.yaml

# --------------------------------------------------------------------
# Local development environment
# --------------------------------------------------------------------

.PHONY: dev-up dev-down up down
dev-up: ## Build and deploy the complete local AIRO environment
	$(MAKE) cluster-up
	$(MAKE) image-build
	$(MAKE) image-load
	$(MAKE) demo-api-image-build
	$(MAKE) demo-api-image-load
	$(MAKE) deploy-monitoring
	$(MAKE) deploy-airo
	$(MAKE) deploy-workload

dev-down: ## Delete the local AIRO environment
	$(MAKE) cluster-down

up: ## Alias for dev-up
	$(MAKE) dev-up

down: ## Alias for dev-down
	$(MAKE) dev-down

# --------------------------------------------------------------------
# Runtime inspection
# --------------------------------------------------------------------

.PHONY: run-monitoring run-airo status airo-status airo-logs demo-api-status
run-monitoring: ## Port-forward Prometheus and Grafana for browser access
	@echo "Prometheus: http://localhost:9090"
	@echo "Grafana:    http://localhost:3000"
	@echo
	@echo "Press Ctrl+C to stop port-forwarding."
	@trap 'kill 0' EXIT; \
		$(KUBECTL) -n monitoring port-forward service/prometheus 9090:9090 & \
		$(KUBECTL) -n monitoring port-forward service/grafana 3000:3000 & \
		wait

run-airo: ## Follow logs from the AIRO controller running in Kubernetes
	@echo "AIRO is running inside Kubernetes."
	$(KUBECTL) -n $(HELM_NAMESPACE) logs \
		deployment/$(HELM_RELEASE) \
		--follow

status: ## Show the status of AIRO, monitoring, and the demo workload
	@echo "=== AIRO ==="
	$(KUBECTL) -n $(HELM_NAMESPACE) get deployment,pods
	@echo
	@echo "=== Monitoring ==="
	$(KUBECTL) -n monitoring get deployment,pods
	@echo
	@echo "=== Workload ==="
	$(KUBECTL) -n default get deployment,pods,service \
		-l app=demo-api

airo-status: ## Show the AIRO deployment and pods
	$(KUBECTL) -n $(HELM_NAMESPACE) get deployment,pods

airo-logs: ## Follow AIRO controller logs
	$(KUBECTL) -n $(HELM_NAMESPACE) logs \
		deployment/$(HELM_RELEASE) \
		--follow

demo-api-status: ## Show the demo-api deployment, pods, and service
	$(KUBECTL) -n default get deployment,pods,service \
		-l app=demo-api

# --------------------------------------------------------------------
# Smoke test
# --------------------------------------------------------------------

.PHONY: smoke-test
smoke-test: ## Verify that AIRO installs and starts successfully
	@echo "Checking AIRO deployment..."
	$(KUBECTL) -n $(HELM_NAMESPACE) rollout status \
		deployment/$(HELM_RELEASE) \
		--timeout=120s

	@echo "Checking RemediationPolicy CRD..."
	$(KUBECTL) get crd remediationpolicies.reliability.airo.io

	@echo "Checking AIRO pods..."
	$(KUBECTL) -n $(HELM_NAMESPACE) get pods

	@echo "Smoke test passed."

# --------------------------------------------------------------------
# Helm release packaging
# --------------------------------------------------------------------

.PHONY: helm-package
helm-package: helm-lint ## Package the AIRO Helm chart
	@mkdir -p dist
	$(HELM) package $(HELM_CHART) \
		--destination dist

# --------------------------------------------------------------------
# Controller tooling
# --------------------------------------------------------------------

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Install controller-gen locally

$(CONTROLLER_GEN):
	mkdir -p $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install \
		sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

# --------------------------------------------------------------------
# CI
# --------------------------------------------------------------------

.PHONY: ci
ci: ## Run the CI validation and unit test suite
	$(MAKE) check
	$(MAKE) unit-test

# --------------------------------------------------------------------
# Cleanup
# --------------------------------------------------------------------

.PHONY: clean
clean: ## Remove local build artifacts and installed tooling
	rm -f $(BINARY_NAME)
	rm -rf $(LOCALBIN)
	rm -rf dist
	rm -f coverage.out