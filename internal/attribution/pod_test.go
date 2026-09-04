package attribution

import (
	"testing"
)

func TestEvaluatePods(t *testing.T) {
	engine := NewAttributionEngine(10.0, 0.30)

	t.Run("Localized Outlier Pod Identified", func(t *testing.T) {
		pods := []PodSLIStat{
			{PodName: "pod-1", PodUID: "uid-1", GoodRequests: 99, TotalRequests: 100, SLI: 0.99},
			{PodName: "pod-2", PodUID: "uid-2", GoodRequests: 98, TotalRequests: 100, SLI: 0.98},
			{PodName: "pod-3", PodUID: "uid-3", GoodRequests: 10, TotalRequests: 100, SLI: 0.10}, // Sick Pod!
			{PodName: "pod-4", PodUID: "uid-4", GoodRequests: 99, TotalRequests: 100, SLI: 0.99},
		}

		result := engine.EvaluatePods(pods)

		if !result.IsAttributable {
			t.Fatalf("Expected IsAttributable=true, got false. Reason: %s", result.Reason)
		}
		if result.TargetPodName != "pod-3" {
			t.Errorf("Expected TargetPodName=pod-3, got %s", result.TargetPodName)
		}
		if result.TargetPodUID != "uid-3" {
			t.Errorf("Expected TargetPodUID=uid-3, got %s", result.TargetPodUID)
		}
		if result.IsSystemicFailure {
			t.Errorf("Expected IsSystemicFailure=false, got true")
		}
	})

	t.Run("Systemic Failure - All Pods Degrading Equally", func(t *testing.T) {
		pods := []PodSLIStat{
			{PodName: "pod-1", PodUID: "uid-1", GoodRequests: 50, TotalRequests: 100, SLI: 0.50},
			{PodName: "pod-2", PodUID: "uid-2", GoodRequests: 51, TotalRequests: 100, SLI: 0.51},
			{PodName: "pod-3", PodUID: "uid-3", GoodRequests: 49, TotalRequests: 100, SLI: 0.49},
			{PodName: "pod-4", PodUID: "uid-4", GoodRequests: 50, TotalRequests: 100, SLI: 0.50},
		}

		result := engine.EvaluatePods(pods)

		if result.IsAttributable {
			t.Errorf("Expected IsAttributable=false for systemic failure, got true")
		}
		if !result.IsSystemicFailure {
			t.Errorf("Expected IsSystemicFailure=true, got false")
		}
	})

	t.Run("Low Traffic Ignored - Statistical Noise Guard", func(t *testing.T) {
		pods := []PodSLIStat{
			{PodName: "pod-1", PodUID: "uid-1", GoodRequests: 99, TotalRequests: 100, SLI: 0.99},
			{PodName: "pod-2", PodUID: "uid-2", GoodRequests: 0, TotalRequests: 2, SLI: 0.00}, // Only 2 requests! Excluded!
		}

		result := engine.EvaluatePods(pods)

		if result.IsAttributable {
			t.Errorf("Expected IsAttributable=false because pod-2 had insufficient volume")
		}
	})
}
