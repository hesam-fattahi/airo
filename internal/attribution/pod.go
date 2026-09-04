package attribution

import (
	"fmt"
	"sort"
)

// PodSLIStat holds telemetry performance data for a single pod.
//
// GoodRequests and TotalRequests may represent rates rather than absolute
// counts, as long as both values use the same unit. Attribution only depends
// on their ratio and relative traffic volume.
type PodSLIStat struct {
	PodName       string
	PodUID        string
	GoodRequests  float64
	TotalRequests float64
}

// SLI returns the pod's calculated service level indicator.
func (p PodSLIStat) SLI() float64 {
	if p.TotalRequests <= 0 {
		return 0
	}

	sli := p.GoodRequests / p.TotalRequests

	if sli < 0 {
		return 0
	}

	if sli > 1 {
		return 1
	}

	return sli
}

// AttributionResult holds the decision of the fault attribution engine.
type AttributionResult struct {
	IsAttributable    bool
	TargetPodName     string
	TargetPodUID      string
	ConfidenceScore   float64
	PeerBaselineSLI   float64
	IsSystemicFailure bool
	Reason            string
}

// AttributionEngine evaluates per-pod metrics to identify localized outliers.
type AttributionEngine struct {
	MinTrafficVolume   float64
	MinConfidenceRatio float64
}

// NewAttributionEngine initializes an engine with safety thresholds.
func NewAttributionEngine(
	minTraffic float64,
	minConfidence float64,
) *AttributionEngine {
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

// EvaluatePods determines whether one pod is a sufficiently degraded outlier
// compared with its peers.
//
// Each state decision is deterministic so reconciliation remains safe and
// repeatable across controller restarts.
func (e *AttributionEngine) EvaluatePods(
	pods []PodSLIStat,
) AttributionResult {
	if len(pods) == 0 {
		return AttributionResult{
			IsAttributable:    false,
			IsSystemicFailure: false,
			Reason:            "No pod metrics available for evaluation",
		}
	}

	eligiblePods := make(
		[]PodSLIStat,
		0,
		len(pods),
	)

	for _, pod := range pods {
		if pod.TotalRequests >= e.MinTrafficVolume {
			eligiblePods = append(
				eligiblePods,
				pod,
			)
		}
	}

	if len(eligiblePods) < 2 {
		return AttributionResult{
			IsAttributable:    false,
			IsSystemicFailure: false,
			Reason:            "Insufficient eligible pod traffic for peer comparison",
		}
	}

	// Sort deterministically.
	//
	// Primary order: lowest SLI first.
	// Tie-breaker: oldest/stable lexical UID order.
	sort.Slice(
		eligiblePods,
		func(i, j int) bool {
			leftSLI := eligiblePods[i].SLI()
			rightSLI := eligiblePods[j].SLI()

			if leftSLI != rightSLI {
				return leftSLI < rightSLI
			}

			return eligiblePods[i].PodUID <
				eligiblePods[j].PodUID
		},
	)

	worstPod := eligiblePods[0]
	worstPodSLI := worstPod.SLI()

	// Calculate the peer baseline EXCLUDING the suspected worst pod.
	var peerGood float64
	var peerTotal float64

	for _, pod := range eligiblePods[1:] {
		peerGood += pod.GoodRequests
		peerTotal += pod.TotalRequests
	}

	if peerTotal <= 0 {
		return AttributionResult{
			IsAttributable:    false,
			IsSystemicFailure: false,
			Reason:            "Insufficient peer traffic for attribution baseline",
		}
	}

	peerBaselineSLI := peerGood / peerTotal

	if peerBaselineSLI < 0 {
		peerBaselineSLI = 0
	}

	if peerBaselineSLI > 1 {
		peerBaselineSLI = 1
	}

	// If peers are also heavily degraded, we should not blame one pod.
	if peerBaselineSLI <= 0 {
		return AttributionResult{
			IsAttributable:    false,
			IsSystemicFailure: true,
			PeerBaselineSLI:   peerBaselineSLI,
			Reason:            "Peer baseline SLI is 0; degradation is systemic across replicas",
		}
	}

	confidenceScore :=
		(peerBaselineSLI - worstPodSLI) /
			peerBaselineSLI

	if confidenceScore < 0 {
		confidenceScore = 0
	}

	if confidenceScore >= e.MinConfidenceRatio {
		return AttributionResult{
			IsAttributable:    true,
			TargetPodName:     worstPod.PodName,
			TargetPodUID:      worstPod.PodUID,
			ConfidenceScore:   confidenceScore,
			PeerBaselineSLI:   peerBaselineSLI,
			IsSystemicFailure: false,
			Reason: fmt.Sprintf(
				"pod %s is a localized outlier: pod SLI %.4f, peer baseline %.4f, confidence %.4f",
				worstPod.PodName,
				worstPodSLI,
				peerBaselineSLI,
				confidenceScore,
			),
		}
	}

	return AttributionResult{
		IsAttributable:    false,
		ConfidenceScore:   confidenceScore,
		PeerBaselineSLI:   peerBaselineSLI,
		IsSystemicFailure: true,
		Reason: fmt.Sprintf(
			"degradation is not sufficiently localized: worst pod SLI %.4f, peer baseline %.4f, confidence %.4f",
			worstPodSLI,
			peerBaselineSLI,
			confidenceScore,
		),
	}
}
