package attribution

import (
	"fmt"
)

// PodSLIStat holds telemetry performance data for a single Pod replica
type PodSLIStat struct {
	PodName       string
	PodUID        string
	GoodRequests  float64
	TotalRequests float64
	SLI           float64
}

// AttributionResult holds the decision of the fault attribution engine
type AttributionResult struct {
	IsAttributable    bool
	TargetPodName     string
	TargetPodUID      string
	ConfidenceScore   float64
	PeerBaselineSLI   float64
	IsSystemicFailure bool
	Reason            string
}

// AttributionEngine evaluates per-pod metrics to identify localized outliers
type AttributionEngine struct {
	MinTrafficVolume   float64 // Minimum requests required for statistical significance (e.g. 10)
	MinConfidenceRatio float64 // Minimum outlier ratio to confirm localization (e.g. 0.30)
}

// NewAttributionEngine initializes an engine with safety thresholds
func NewAttributionEngine(minTraffic, minConfidence float64) *AttributionEngine {
	if minTraffic <= 0 {
		minTraffic = 10.0
	}
	if minConfidence <= 0 {
		minConfidence = 0.30
	}
	return &AttributionEngine{
		MinTrafficVolume:   minTraffic,
		MinConfidenceRatio: minConfidence,
	}
}

// EvaluatePods identifies if a single pod is a localized outlier vs a systemic failure
func (e *AttributionEngine) EvaluatePods(pods []PodSLIStat) AttributionResult {
	if len(pods) == 0 {
		return AttributionResult{
			IsAttributable:    false,
			IsSystemicFailure: true,
			Reason:            "No pod metrics available for evaluation",
		}
	}

	var eligiblePods []PodSLIStat
	var totalGood, totalRequests float64

	// Step 1: Filter out low-traffic pods (Statistical Significance Guard)
	for _, p := range pods {
		if p.TotalRequests >= e.MinTrafficVolume {
			eligiblePods = append(eligiblePods, p)
			totalGood += p.GoodRequests
			totalRequests += p.TotalRequests
		}
	}

	if len(eligiblePods) == 0 {
		return AttributionResult{
			IsAttributable:    false,
			IsSystemicFailure: false,
			Reason:            "Insufficient traffic volume across pods for statistical attribution",
		}
	}

	// Step 2: Calculate Peer Baseline SLI
	peerBaselineSLI := totalGood / totalRequests

	// Step 3: Identify worst-performing pod
	worstPod := eligiblePods[0]
	for _, p := range eligiblePods {
		if p.SLI < worstPod.SLI {
			worstPod = p
		}
	}

	// Step 4: Calculate Relative Outlier Confidence Score
	if peerBaselineSLI <= 0 {
		return AttributionResult{
			IsAttributable:    false,
			IsSystemicFailure: true,
			Reason:            "Peer baseline SLI is 0%; failure is systemic across all replicas",
		}
	}

	confidenceScore := (peerBaselineSLI - worstPod.SLI) / peerBaselineSLI

	// Step 5: Decision Boundary Check
	if confidenceScore >= e.MinConfidenceRatio {
		return AttributionResult{
			IsAttributable:    true,
			TargetPodName:     worstPod.PodName,
			TargetPodUID:      worstPod.PodUID,
			ConfidenceScore:   confidenceScore,
			PeerBaselineSLI:   peerBaselineSLI,
			IsSystemicFailure: false,
			Reason:            fmt.Sprintf("Pod %s is a localized outlier (Confidence: %.2f)", worstPod.PodName, confidenceScore),
		}
	}

	// If confidence score is low, all pods are failing equally -> Systemic Failure
	return AttributionResult{
		IsAttributable:    false,
		ConfidenceScore:   confidenceScore,
		PeerBaselineSLI:   peerBaselineSLI,
		IsSystemicFailure: true,
		Reason:            fmt.Sprintf("Degradation is uniform across pods (Peer Baseline: %.2f); systemic backend failure detected", peerBaselineSLI),
	}
}