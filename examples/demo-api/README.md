# Demo API

A minimal HTTP workload used to demonstrate AIRO's SLO-based incident
detection and autonomous remediation.

It provides:

- a simple HTTP endpoint
- `/healthz` for Kubernetes health checks
- `/metrics` for Prometheus
- deterministic per-pod latency injection for testing

The application does not contain AIRO code or depend on the AIRO controller.
The Kubernetes workload uses the `airo.io/traffic` label so AIRO can isolate
a pod from Service traffic during remediation.

## Endpoints

```text
GET /api/v1/message
GET /healthz
GET /metrics
```

Example:

```bash
curl http://localhost:8080/api/v1/message
```

## Build

```bash
docker build -t demo-api:v1.0 -f examples/demo-api/Dockerfile .
```

Load the image into KinD:

```bash
kind load docker-image demo-api:v1.0
```

## Deploy

```bash
kubectl apply -f examples/demo-api/deployment.yaml
kubectl apply -f examples/demo-api/service.yaml
```

Check the pods:

```bash
kubectl get pods -l app=demo-api
```

## Load generation

Deploy the Fortio load generator:

```bash
kubectl apply -f examples/demo-api/load-generator.yaml
```

It continuously sends traffic to the `demo-api` Service.

## Latency injection

Find a pod:

```bash
kubectl get pods -l app=demo-api
```

Inject 200 ms latency:

```bash
kubectl exec <pod-name> -- sh -c 'echo 200 > /tmp/latency_ms'
```

Clear the latency:

```bash
kubectl exec <pod-name> -- rm -f /tmp/latency_ms
```

## AIRO policy

Apply the example remediation policy:

```bash
kubectl apply -f examples/demo-api/remediation-policy.yaml
```

AIRO monitors the workload's Prometheus metrics, detects SLO violations,
identifies the affected pod, isolates it from Service traffic, and deletes
it so Kubernetes can replace it.

## Demo flow

```text
normal traffic
    ↓
inject latency into one pod
    ↓
SLI / burn rate degrades
    ↓
AIRO detects the violation
    ↓
AIRO isolates and deletes the pod
    ↓
Kubernetes creates a replacement
    ↓
service recovers
```