# SRE Math

AIRO's current reliability model is based on a latency SLI evaluated against a configured SLO and monitored through multi-window, multi-burn-rate detection. The overall approach and example burn-rate values are inspired by the SRE practices described in the [Google SRE Book](https://sre.google/sre-book/service-level-objectives/) and [SRE Workbook](https://sre.google/workbook/alerting-on-slos/).

The values used by AIRO and explained in this documentation are arbitrary and can be configured in the Remediation Policy defined for a certain deployment. A sample policy could be found at [`RemediationPolicy`](../config/samples/remediationpolicy.yaml).

## Histogram Input

The workload exposes `http_request_duration_seconds` as a Prometheus histogram with latency buckets ranging from `0.0005s` to `2.5s`.

The current SLO threshold is **50 ms**, so the relevant histogram bucket is `le="0.05"`.

The request metric also carries `path` and `status` labels, allowing AIRO to select the relevant traffic and evaluate it at Pod level.

## SLI

AIRO's current SLI is the proportion of requests that complete within the configured latency threshold.

For the current policy, that means:

> **requests with latency `le="0.05"` divided by all requests**

The same calculation can be evaluated over different Prometheus time windows and grouped by Pod for attribution.

## SLO

The current policy defines:

    slo:
      target: 0.99
      latencyThresholdMs: 50

This means:

> **99% of requests must finish within 50 ms (`le="0.05"`).**

## Error Budget

With a 99% SLO, the allowed error budget is **1%**.

The observed error rate is:

> **the fraction of requests whose latency exceeds 50 ms.**

The relationship is:

`error_rate = 1 - SLI`

`error_budget = 1 - SLO`

## Burn Rate

Burn rate expresses how quickly the workload is consuming its available error budget:

`burn_rate = error_rate / error_budget`

or equivalently:

`burn_rate = (1 - SLI) / (1 - SLO)`

A burn rate of `1x` means the error budget is being consumed at the sustainable rate. Higher values indicate faster budget consumption.

## Multi-Window, Multi-Burn-Rate Detection

AIRO uses two burn-rate conditions:

| Condition | Short window | Long window | Burn-rate threshold |
|---|---:|---:|---:|
| Fast burn | 5m | 1h | 14.4x |
| Slow burn | 30m | 6h | 6x |

A **fast-burn** condition is met when both the 5-minute and 1-hour burn rates reach `14.4x`.

A **slow-burn** condition is met when both the 30-minute and 6-hour burn rates reach `6x`.

### Why Multiple Windows?

A single long window can remain elevated after a degradation has already recovered because it still contains historical bad requests.

The short window confirms that the high burn rate is still active.

Using both windows therefore lets AIRO detect significant budget consumption while reducing the chance of treating an already-resolved incident as an active one.

The Google SRE Workbook recommends using a short window approximately **1/12 of the long window** for multi-window alerting. AIRO follows that relationship:

- `5m / 1h = 1/12`
- `30m / 6h = 1/12`

The `14.4x` and `6x` values are configurable policy values, not fixed SRE requirements.

## What the Two Conditions Represent

### Fast Burn

The workload is consuming its error budget very quickly.

The 5-minute and 1-hour windows must both show a `14.4x` burn rate, indicating that the degradation is both severe and currently active.

### Slow Burn

The workload is consuming its error budget more gradually but persistently.

The 30-minute and 6-hour windows must both show a `6x` burn rate, allowing AIRO to detect sustained degradation that may not reach the fast-burn threshold.

## Detection Is Not Remediation

A burn-rate violation only indicates that the workload requires further evaluation. It does not directly authorize Pod remediation.

After burn detection, the Controller proceeds with attribution and safety evaluation before taking action.

See [Architecture](architecture.md) for the overall controller flow and [Remediation](remediation.md) for attribution, safety, isolation, execution, and recovery.

## References

- [Google SRE Book — Service Level Objectives](https://sre.google/sre-book/service-level-objectives/)
- [Google SRE Workbook — Alerting on SLOs](https://sre.google/workbook/alerting-on-slos/)
- [Prometheus — Metric Types](https://prometheus.io/docs/concepts/metric_types/)