package attribution

import "testing"

func TestEvaluatePods(t *testing.T) {
	engine := NewAttributionEngine(
		10.0,
		0.30,
	)

	t.Run(
		"Localized Outlier Pod Identified",
		func(t *testing.T) {
			pods := []PodSLIStat{
				{
					PodName:       "pod-1",
					PodUID:        "uid-1",
					GoodRequests:  99,
					TotalRequests: 100,
				},
				{
					PodName:       "pod-2",
					PodUID:        "uid-2",
					GoodRequests:  98,
					TotalRequests: 100,
				},
				{
					PodName:       "pod-3",
					PodUID:        "uid-3",
					GoodRequests:  10,
					TotalRequests: 100,
				},
				{
					PodName:       "pod-4",
					PodUID:        "uid-4",
					GoodRequests:  99,
					TotalRequests: 100,
				},
			}

			result := engine.EvaluatePods(pods)

			if !result.IsAttributable {
				t.Fatalf(
					"expected attributable result, got false: %s",
					result.Reason,
				)
			}

			if result.TargetPodName != "pod-3" {
				t.Errorf(
					"expected target pod pod-3, got %s",
					result.TargetPodName,
				)
			}

			if result.TargetPodUID != "uid-3" {
				t.Errorf(
					"expected target UID uid-3, got %s",
					result.TargetPodUID,
				)
			}

			if result.IsSystemicFailure {
				t.Error(
					"expected localized failure, got systemic failure",
				)
			}
		},
	)

	t.Run(
		"Systemic Failure",
		func(t *testing.T) {
			pods := []PodSLIStat{
				{
					PodName:       "pod-1",
					PodUID:        "uid-1",
					GoodRequests:  50,
					TotalRequests: 100,
				},
				{
					PodName:       "pod-2",
					PodUID:        "uid-2",
					GoodRequests:  51,
					TotalRequests: 100,
				},
				{
					PodName:       "pod-3",
					PodUID:        "uid-3",
					GoodRequests:  49,
					TotalRequests: 100,
				},
				{
					PodName:       "pod-4",
					PodUID:        "uid-4",
					GoodRequests:  50,
					TotalRequests: 100,
				},
			}

			result := engine.EvaluatePods(pods)

			if result.IsAttributable {
				t.Error(
					"expected systemic failure to be non-attributable",
				)
			}

			if !result.IsSystemicFailure {
				t.Errorf(
					"expected systemic failure, got false: %s",
					result.Reason,
				)
			}
		},
	)

	t.Run(
		"Low Traffic Pod Is Ignored",
		func(t *testing.T) {
			pods := []PodSLIStat{
				{
					PodName:       "pod-1",
					PodUID:        "uid-1",
					GoodRequests:  99,
					TotalRequests: 100,
				},
				{
					PodName:       "pod-2",
					PodUID:        "uid-2",
					GoodRequests:  98,
					TotalRequests: 100,
				},
				{
					PodName:       "pod-3",
					PodUID:        "uid-3",
					GoodRequests:  0,
					TotalRequests: 2,
				},
			}

			result := engine.EvaluatePods(pods)

			if result.IsAttributable {
				t.Errorf(
					"low traffic pod should not be attributed: %s",
					result.Reason,
				)
			}
		},
	)

	t.Run(
		"Insufficient Pods For Peer Comparison",
		func(t *testing.T) {
			pods := []PodSLIStat{
				{
					PodName:       "pod-1",
					PodUID:        "uid-1",
					GoodRequests:  10,
					TotalRequests: 100,
				},
			}

			result := engine.EvaluatePods(pods)

			if result.IsAttributable {
				t.Error(
					"single pod must not be attributed without peers",
				)
			}

			if result.IsSystemicFailure {
				t.Error(
					"insufficient data should not automatically be systemic failure",
				)
			}
		},
	)

	t.Run(
		"Deterministic Tie Breaker",
		func(t *testing.T) {
			pods := []PodSLIStat{
				{
					PodName:       "pod-a",
					PodUID:        "uid-2",
					GoodRequests:  10,
					TotalRequests: 100,
				},
				{
					PodName:       "pod-b",
					PodUID:        "uid-1",
					GoodRequests:  10,
					TotalRequests: 100,
				},
				{
					PodName:       "pod-c",
					PodUID:        "uid-3",
					GoodRequests:  99,
					TotalRequests: 100,
				},
				{
					PodName:       "pod-d",
					PodUID:        "uid-4",
					GoodRequests:  99,
					TotalRequests: 100,
				},
			}

			result := engine.EvaluatePods(pods)

			if !result.IsAttributable {
				t.Fatalf(
					"expected attributable result: %s",
					result.Reason,
				)
			}

			if result.TargetPodUID != "uid-1" {
				t.Errorf(
					"expected deterministic UID tie breaker uid-1, got %s",
					result.TargetPodUID,
				)
			}
		},
	)
}

func TestPodSLI(t *testing.T) {
	tests := []struct {
		name     string
		pod      PodSLIStat
		expected float64
	}{
		{
			name: "Normal SLI",
			pod: PodSLIStat{
				GoodRequests:  90,
				TotalRequests: 100,
			},
			expected: 0.9,
		},
		{
			name: "Zero Traffic",
			pod: PodSLIStat{
				GoodRequests:  0,
				TotalRequests: 0,
			},
			expected: 0,
		},
		{
			name: "Clamp Above One",
			pod: PodSLIStat{
				GoodRequests:  200,
				TotalRequests: 100,
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.pod.SLI()

			if result != tt.expected {
				t.Errorf(
					"expected SLI %f, got %f",
					tt.expected,
					result,
				)
			}
		})
	}
}
