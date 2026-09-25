<div align="center">

# AIRO

[![GitHub Release](https://img.shields.io/github/v/release/hesam-fattahi/airo?include_prereleases&style=flat-square&color=blue)](https://github.com/hesam-fattahi/airo/releases)
[![Build Status](https://img.shields.io/github/actions/workflow/status/hesam-fattahi/airo/ci.yaml?branch=main&style=flat-square&label=ci)](https://github.com/hesam-fattahi/airo/actions)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg?style=flat-square)](LICENSE)
[![GHCR Container Image](https://img.shields.io/badge/container-ghcr.io-green?style=flat-square&logo=docker)](https://github.com/hesam-fattahi/airo/pkgs/container/airo)
[![Helm Chart](https://img.shields.io/badge/helm-oci%2Fcharts-blue?style=flat-square&logo=helm)](https://github.com/hesam-fattahi/airo/pkgs/container/charts%2Fairo)

</div>

AIRO is a Kubernetes operator that turns sustained SLO burn-rate violations into safe, bounded, and verifiable remediation actions.

## The Problem

Kubernetes liveness and readiness probes are useful for detecting basic application health, but they do not express higher-level SLOs such as request latency or error-budget consumption. A workload can remain healthy from Kubernetes' perspective while its user-facing reliability is already degrading.

Alerting can detect these conditions, but turning an alert into an automated remediation decision introduces another problem: blindly restarting a replica can lead to cascading failures and make a broader incident worse.

## What AIRO does

AIRO uses Prometheus telemetry to evaluate workload SLOs and detect sustained burn-rate violations.

When degradation can be attributed to an individual Pod, AIRO evaluates whether remediation is safe. It can isolate the Pod from Service traffic before removing it, allowing Kubernetes to replace it through the workload controller.

The controller then observes the workload again and verifies recovery before considering the incident resolved.

## Architecture
![high level architecture of airo](docs/images/architecture.jpg)

AIRO runs as a Kubernetes controller and interacts with Prometheus and
the Kubernetes API to evaluate workload health and execute bounded remediation.

Check [architecture.md](docs/architecture.md) for the detailed controller and
component design.


## Quick Start

### 0. Prerequisites
Before deploying AIRO, ensure your environment meets the following requirements:

* **Kubernetes Cluster:** `v1.26+` with administrative access (`kubectl` configured).
* **Helm CLI:** `v3.8.0+` (required for OCI chart support).
* **Prometheus Instance:** Accessible inside the cluster (e.g., `kube-prometheus-stack`) to serve application latency/error metrics to AIRO.

### 1. Install AIRO Operator

Install directly from GitHub Container Registry (GHCR):

``` bash
helm install airo-operator oci://ghcr.io/hesam-fattahi/charts/airo \
  --version 0.1.0 \
  --namespace airo-system \
  --create-namespace
```

### 2. Opt-In Workloads for Traffic Isolation

Add `airo.io/traffic: "enabled"` **specifically to your Pod template metadata** (`.spec.template.metadata.labels`)

``` yaml
spec:
  template:
    metadata:
      labels:
        airo.io/traffic: "enabled"
```

Patch an existing Deployment and trigger a rollout restart. Replace `<app-name>` with your application name:

``` bash
kubectl patch deployment <app-name> --type='json' \
  -p='[{"op": "add", "path": "/spec/template/metadata/labels/airo.io~1traffic", "value": "enabled"}]'

kubectl rollout restart deployment/<app-name>
```

> **Note:** Right now AIRO uses `airo.io/traffic: enabled` to modify EndpointSlices and safely drain traffic from degraded Pod UIDs prior to remediation. This coupling to the app's deployment will be removed in a future update.

### 3. Apply Remediation Policy

Create a remediation policy in your workload directory. You can find a sample in `config/samples/remediationpolicy.yaml`. And then apply the policy:

``` bash
kubectl apply -f config/samples/remediationpolicy.yaml
```

### 4. Verify Installation

``` bash
kubectl get pods -n airo-system
kubectl get remediationpolicies
```

## Further Documentation

- **[Architecture](docs/architecture.md)** — High-level system architecture, controller responsibilities, internal modules, Kubernetes interaction, and workload metrics contract.
- **[SRE Math](docs/sre-math.md)** — SLI/SLO calculation, error budgets, burn rates, and multi-window burn-rate detection.
- **[Remediation](docs/remediation.md)** — Attribution, safety gates, Pod isolation, execution, recovery, and remediation state transitions.
- **[Development](docs/development.md)** — Repository structure, local KinD environment, observability stack, development commands, and local workflow.


## Contributing
Read [CONTRIBUTING.md](CONTRIBUTING.md) for the basic workflow and links to area-specific development guides.

## License
AIRO is licensed under the Apache License 2.0. See [LICENSE](LICENSE).