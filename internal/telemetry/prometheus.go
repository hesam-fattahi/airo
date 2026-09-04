package telemetry

import (
	"context"
	"fmt"
	"time"

	prometheusapi "github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// SLIResult represents the result of a deployment-level SLI query.
type SLIResult struct {
	Value      float64
	HasTraffic bool
}

// PodSLIMetric represents per-pod SLI telemetry.
//
// GoodRequestRate and TotalRequestRate are calculated using Prometheus
// rate(), so they represent average requests per second over the requested
// evaluation window rather than absolute request counts.
type PodSLIMetric struct {
	PodName          string
	PodUID           string
	GoodRequestRate  float64
	TotalRequestRate float64
}

// Client defines the interface for querying Prometheus SLI metrics.
type Client interface {
	QueryDeploymentSLI(
		ctx context.Context,
		appName string,
		window string,
		latencyThresholdMs int64,
	) (SLIResult, error)

	QueryPodSLIs(
		ctx context.Context,
		appName string,
		window string,
		latencyThresholdMs int64,
	) ([]PodSLIMetric, error)
}

type prometheusClient struct {
	api prometheusv1.API
}

// NewPrometheusClient initializes a Prometheus API client.
func NewPrometheusClient(address string) (Client, error) {
	client, err := prometheusapi.NewClient(prometheusapi.Config{
		Address: address,
	})
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create prometheus client: %w",
			err,
		)
	}

	return &prometheusClient{
		api: prometheusv1.NewAPI(client),
	}, nil
}

// QueryDeploymentSLI calculates:
//
// good events / total events
//
// for the entire deployment.
//
// HasTraffic is false when Prometheus has no matching time series or the
// total request rate is zero.
func (p *prometheusClient) QueryDeploymentSLI(
	ctx context.Context,
	appName string,
	window string,
	latencyThresholdMs int64,
) (SLIResult, error) {
	if latencyThresholdMs <= 0 {
		latencyThresholdMs = 50
	}

	thresholdSeconds :=
		float64(latencyThresholdMs) / 1000.0

	query := fmt.Sprintf(
		`sum(rate(http_request_duration_seconds_bucket{app="%s",le="%g"}[%s]))
/
sum(rate(http_request_duration_seconds_bucket{app="%s",le="+Inf"}[%s]))`,
		appName,
		thresholdSeconds,
		window,
		appName,
		window,
	)

	result, _, err := p.api.Query(
		ctx,
		query,
		time.Now(),
	)
	if err != nil {
		return SLIResult{}, fmt.Errorf(
			"promql deployment SLI query error: %w",
			err,
		)
	}

	vector, ok := result.(model.Vector)
	if !ok || len(vector) == 0 {
		return SLIResult{
			Value:      0,
			HasTraffic: false,
		}, nil
	}

	if vector[0] == nil {
		return SLIResult{
			Value:      0,
			HasTraffic: false,
		}, nil
	}

	sli := float64(vector[0].Value)

	if sli < 0 {
		sli = 0
	}

	if sli > 1 {
		sli = 1
	}

	return SLIResult{
		Value:      sli,
		HasTraffic: true,
	}, nil
}

// QueryPodSLIs returns per-pod good and total request rates for
// fault attribution.
//
// The metrics are grouped by pod. pod_uid is included when available in
// Prometheus labels.
func (p *prometheusClient) QueryPodSLIs(
	ctx context.Context,
	appName string,
	window string,
	latencyThresholdMs int64,
) ([]PodSLIMetric, error) {
	if latencyThresholdMs <= 0 {
		latencyThresholdMs = 50
	}

	thresholdSeconds :=
		float64(latencyThresholdMs) / 1000.0

	goodQuery := fmt.Sprintf(
		`sum by (pod, pod_uid) (
  rate(http_request_duration_seconds_bucket{
    app="%s",
    le="%g"
  }[%s])
)`,
		appName,
		thresholdSeconds,
		window,
	)

	totalQuery := fmt.Sprintf(
		`sum by (pod, pod_uid) (
  rate(http_request_duration_seconds_bucket{
    app="%s",
    le="+Inf"
  }[%s])
)`,
		appName,
		window,
	)

	goodResult, _, err := p.api.Query(
		ctx,
		goodQuery,
		time.Now(),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"promql good request rate query error: %w",
			err,
		)
	}

	totalResult, _, err := p.api.Query(
		ctx,
		totalQuery,
		time.Now(),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"promql total request rate query error: %w",
			err,
		)
	}

	goodVector, ok := goodResult.(model.Vector)
	if !ok {
		return nil, fmt.Errorf(
			"unexpected Prometheus result type for good request query",
		)
	}

	totalVector, ok := totalResult.(model.Vector)
	if !ok {
		return nil, fmt.Errorf(
			"unexpected Prometheus result type for total request query",
		)
	}

	podMap := make(map[string]*PodSLIMetric)

	for _, sample := range totalVector {
		podName := string(sample.Metric["pod"])

		if podName == "" {
			continue
		}

		podUID := string(sample.Metric["pod_uid"])

		podMap[podName] = &PodSLIMetric{
			PodName:          podName,
			PodUID:           podUID,
			TotalRequestRate: float64(sample.Value),
		}
	}

	for _, sample := range goodVector {
		podName := string(sample.Metric["pod"])

		if podName == "" {
			continue
		}

		metric, exists := podMap[podName]
		if !exists {
			continue
		}

		metric.GoodRequestRate =
			float64(sample.Value)
	}

	metrics := make(
		[]PodSLIMetric,
		0,
		len(podMap),
	)

	for _, metric := range podMap {
		metrics = append(metrics, *metric)
	}

	return metrics, nil
}
