SHELL := /bin/bash

# =============================================================================
# Project
# =============================================================================

PROJECT_NAME := airo
BINARY_NAME := airo
MAIN_PATH := ./cmd/main.go

GO ?= go
GOFMT ?= gofmt
HELM ?= helm
KUBECTL ?= kubectl
KIND ?= kind

# =============================================================================
# Local Kubernetes environment
# =============================================================================

KIND_CLUSTER_NAME := airo-cluster
KIND_CONFIG := deploy/kind/kind-config.yaml

# =============================================================================
# Container images
# =============================================================================

AIRO_IMAGE := airo:latest
DEMO_API_IMAGE := demo-api:v1.0

# =============================================================================
# Helm
# =============================================================================

HELM_CHART := ./charts/airo
HELM_RELEASE := airo
HELM_NAMESPACE := airo

PROMETHEUS_URL := http://prometheus.monitoring.svc:9090

# =============================================================================
# Deployment manifests
# =============================================================================

PROMETHEUS_DIR := deploy/prometheus
GRAFANA_DIR := deploy/grafana

DEMO_API_DIR := examples/demo-api

# =============================================================================
# Generated Kubernetes artifacts
# =============================================================================

CRD_DIR := config/crd/bases
HELM_CRD_DIR := $(HELM_CHART)/crds

# =============================================================================
# Tooling
# =============================================================================

LOCALBIN := $(shell pwd)/bin
CONTROLLER_TOOLS_VERSION := v0.16.5
CONTROLLER_GEN := $(LOCALBIN)/controller-gen


# =============================================================================
# Default
# =============================================================================

.PHONY: all

all: check


# =============================================================================
# Help
# =============================================================================

.PHONY: help

help: ## Show available Make targets
	@echo "AIRO development commands:"
	@echo
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make <target>\n\nTargets:\n"} \
		/^[a-zA-Z0-9_-]+:.*##/ {printf "  %-28s %s\n", $$1, $$2}' \
		$(MAKEFILE_LIST)


# =============================================================================
# Code quality
# =============================================================================

.PHONY: fmt fmt-check vet unit-test generate-check verify check

fmt: ## Format all Go source files
	$(GOFMT) -w $$(find . -name '*.go' -not -path './vendor/*')

fmt-check: ## Check that all Go source files are formatted
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*'))" \
		|| (echo "Go files are not formatted:" && \
		    gofmt -l $$(find . -name '*.go' -not -path './vendor/*') && \
		    exit 1)

vet: ## Run go vet
	$(GO) vet ./...

unit-test: ## Run unit tests with the race detector
	$(GO) test -race -count=1 ./...

generate-check: ## Verify generated CRD manifests are up to date
	$(MAKE) manifests
	@git diff --exit-code -- \
		$(CRD_DIR) \
		$(HELM_CRD_DIR)

verify: ## Run static analysis and Helm validation
	$(MAKE) vet
	$(MAKE) helm-lint
	$(MAKE) helm-template

check: ## Run formatting, generated manifest, and verification checks
	$(MAKE) fmt-check
	$(MAKE) generate-check
	$(MAKE) verify


# =============================================================================
# Build
# =============================================================================

.PHONY: build image-build demo-api-image-build

build: ## Build the AIRO executable locally
	$(GO) build -o $(BINARY_NAME) $(MAIN_PATH)

image-build: ## Build the AIRO container image
	docker build -t $(AIRO_IMAGE) .

demo-api-image-build: ## Build the demo-api container image
	docker build \
		-f $(DEMO_API_DIR)/Dockerfile \
		-t $(DEMO_API_IMAGE) \
		.


# =============================================================================
# Kubernetes cluster
# =============================================================================

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


# =============================================================================
# Monitoring
# =============================================================================

.PHONY: deploy-monitoring

deploy-monitoring: ## Deploy Prometheus and Grafana to the cluster
	$(KUBECTL) apply -f $(PROMETHEUS_DIR)
	$(KUBECTL) apply -f $(GRAFANA_DIR)


# =============================================================================
# AIRO
# =============================================================================

.PHONY: manifests helm-lint helm-template deploy-airo

manifests: controller-gen ## Generate CRD manifests and sync them to the Helm chart 
	$(CONTROLLER_GEN) \ 
	crd:allowDangerousTypes=true \ 
	paths="./api/..." \ 
	output:crd:artifacts:config=$(CRD_DIR) 
	@mkdir -p $(HELM_CRD_DIR) 
	@cp $(CRD_DIR)/*.yaml $(HELM_CRD_DIR)/

helm-lint: ## Validate the AIRO Helm chart
	$(HELM) lint $(HELM_CHART)

helm-template: ## Render the AIRO Helm chart locally
	$(HELM) template \
		$(HELM_RELEASE) \
		$(HELM_CHART) \
		--namespace $(HELM_NAMESPACE) \
		--set prometheus.url=$(PROMETHEUS_URL)

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


# =============================================================================
# Example workload
# =============================================================================

.PHONY: deploy-workload

deploy-workload: ## Deploy demo-api, Fortio load generation, and its remediation policy
	$(KUBECTL) apply -f $(DEMO_API_DIR)/deployment.yaml
	$(KUBECTL) apply -f $(DEMO_API_DIR)/service.yaml
	$(KUBECTL) apply -f $(DEMO_API_DIR)/load-generator.yaml
	$(KUBECTL) apply -f $(DEMO_API_DIR)/remediation-policy.yaml


# =============================================================================
# Runtime
# =============================================================================

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


# =============================================================================
# Development environment
# =============================================================================

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


# =============================================================================
# Smoke tests
# =============================================================================

.PHONY: smoke-test

smoke-test: ## Verify that the deployed AIRO environment is healthy
	@echo "Checking AIRO deployment..."
	$(KUBECTL) -n $(HELM_NAMESPACE) rollout status \
		deployment/$(HELM_RELEASE) \
		--timeout=120s

	@echo "Checking RemediationPolicy CRD..."
	$(KUBECTL) get crd remediationpolicies.reliability.airo.io

	@echo "Checking demo-api deployment..."
	$(KUBECTL) -n default rollout status \
		deployment/demo-api \
		--timeout=120s

	@echo "Checking demo-api pods..."
	$(KUBECTL) -n default get pods \
		-l app=demo-api

	@echo "Smoke test passed."


# =============================================================================
# End-to-end remediation
# =============================================================================

.PHONY: e2e-remediation

e2e-remediation: ## Run the full SLO-driven AIRO remediation scenario
	@echo "Running full AIRO remediation test..."
	@echo "This test intentionally waits for Prometheus detection windows."
	@echo
	@echo "Full remediation scenario:"
	@echo "  1. Verify healthy baseline"
	@echo "  2. Inject latency into one demo-api pod"
	@echo "  3. Wait for SLO/burn-rate detection"
	@echo "  4. Verify AIRO identifies the offending pod"
	@echo "  5. Verify traffic isolation"
	@echo "  6. Verify pod deletion/replacement"
	@echo "  7. Wait for recovery holdoff"
	@echo "  8. Verify recovery state"


# =============================================================================
# Controller tooling
# =============================================================================

.PHONY: controller-gen

controller-gen: $(CONTROLLER_GEN) ## Install controller-gen locally

$(CONTROLLER_GEN):
	mkdir -p $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install \
		sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)


# =============================================================================
# CI
# =============================================================================

.PHONY: ci

ci: ## Run the fast CI validation suite
	$(MAKE) fmt-check
	$(MAKE) vet
	$(MAKE) unit-test
	$(MAKE) helm-lint
	$(MAKE) helm-template


# =============================================================================
# Cleanup
# =============================================================================

.PHONY: clean

clean: ## Remove local build artifacts and installed tooling
	rm -f $(BINARY_NAME)
	rm -rf $(LOCALBIN)