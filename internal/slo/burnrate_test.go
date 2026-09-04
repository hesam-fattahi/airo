package slo

import (
	"math"
	"testing"

	"github.com/hesam-fattahi/airo/api/v1alpha1"
)

func TestEvaluateWindowPair(t *testing.T) {
	fastWindow := v1alpha1.BurnRateWindow{
		ShortWindow: "5m",
		LongWindow:  "1h",
		Threshold:   14.4,
	}

	slowWindow := v1alpha1.BurnRateWindow{
		ShortWindow: "30m",
		LongWindow:  "6h",
		Threshold:   6.0,
	}

	const epsilon = 0.0001

	tests := []struct {
		name            string
		targetSLO       float64
		shortSLI        float64
		longSLI         float64
		window          v1alpha1.BurnRateWindow
		expectError     bool
		expectedBurn    bool
		expectedShortBR float64
		expectedLongBR  float64
	}{
		// FAST BURN TESTS
		{
			name:            "Fast Burn - Healthy State",
			targetSLO:       0.99,
			shortSLI:        1.00,
			longSLI:         1.00,
			window:          fastWindow,
			expectedBurn:    false,
			expectedShortBR: 0.0,
			expectedLongBR:  0.0,
		},
		{
			name:            "Fast Burn - Confirmed",
			targetSLO:       0.99,
			shortSLI:        0.85,
			longSLI:         0.85,
			window:          fastWindow,
			expectedBurn:    true,
			expectedShortBR: 15.0,
			expectedLongBR:  15.0,
		},
		{
			name:            "Fast Burn - Transient Spike Guard",
			targetSLO:       0.99,
			shortSLI:        0.80,
			longSLI:         0.99,
			window:          fastWindow,
			expectedBurn:    false,
			expectedShortBR: 20.0,
			expectedLongBR:  1.0,
		},

		// SLOW BURN TESTS
		{
			name:            "Slow Burn - Healthy",
			targetSLO:       0.99,
			shortSLI:        0.995,
			longSLI:         0.995,
			window:          slowWindow,
			expectedBurn:    false,
			expectedShortBR: 0.5,
			expectedLongBR:  0.5,
		},
		{
			name:            "Slow Burn - Confirmed",
			targetSLO:       0.99,
			shortSLI:        0.92,
			longSLI:         0.92,
			window:          slowWindow,
			expectedBurn:    true,
			expectedShortBR: 8.0,
			expectedLongBR:  8.0,
		},
		{
			name:            "Slow Burn - Long Window Does Not Confirm",
			targetSLO:       0.99,
			shortSLI:        0.92,
			longSLI:         0.98,
			window:          slowWindow,
			expectedBurn:    false,
			expectedShortBR: 8.0,
			expectedLongBR:  2.0,
		},

		// OUTAGE AND BOUNDARY TESTS
		{
			name:            "Catastrophic Outage",
			targetSLO:       0.99,
			shortSLI:        0.00,
			longSLI:         0.00,
			window:          fastWindow,
			expectedBurn:    true,
			expectedShortBR: 100.0,
			expectedLongBR:  100.0,
		},
		{
			name:            "Exact Threshold Boundary",
			targetSLO:       0.99,
			shortSLI:        0.856,
			longSLI:         0.856,
			window:          fastWindow,
			expectedBurn:    true,
			expectedShortBR: 14.4,
			expectedLongBR:  14.4,
		},
		{
			name:            "Just Below Threshold",
			targetSLO:       0.99,
			shortSLI:        0.857,
			longSLI:         0.857,
			window:          fastWindow,
			expectedBurn:    false,
			expectedShortBR: 14.3,
			expectedLongBR:  14.3,
		},

		// VALIDATION TESTS
		{
			name:        "Invalid Target SLO Zero",
			targetSLO:   0.0,
			shortSLI:    0.99,
			longSLI:     0.99,
			window:      fastWindow,
			expectError: true,
		},
		{
			name:        "Invalid Target SLO One",
			targetSLO:   1.0,
			shortSLI:    0.99,
			longSLI:     0.99,
			window:      fastWindow,
			expectError: true,
		},
		{
			name:        "Invalid Short SLI Below Zero",
			targetSLO:   0.99,
			shortSLI:    -0.1,
			longSLI:     0.99,
			window:      fastWindow,
			expectError: true,
		},
		{
			name:        "Invalid Short SLI Above One",
			targetSLO:   0.99,
			shortSLI:    1.1,
			longSLI:     0.99,
			window:      fastWindow,
			expectError: true,
		},
		{
			name:        "Invalid Long SLI Below Zero",
			targetSLO:   0.99,
			shortSLI:    0.99,
			longSLI:     -0.1,
			window:      fastWindow,
			expectError: true,
		},
		{
			name:      "Invalid Threshold",
			targetSLO: 0.99,
			shortSLI:  0.99,
			longSLI:   0.99,
			window: v1alpha1.BurnRateWindow{
				ShortWindow: "5m",
				LongWindow:  "1h",
				Threshold:   0,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := EvaluateWindowPair(
				tt.targetSLO,
				tt.shortSLI,
				tt.longSLI,
				tt.window,
			)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected validation error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.IsConfirmed != tt.expectedBurn {
				t.Errorf(
					"expected IsConfirmed=%v, got %v",
					tt.expectedBurn,
					result.IsConfirmed,
				)
			}

			if math.Abs(
				result.ShortBurnRate-tt.expectedShortBR,
			) > epsilon {
				t.Errorf(
					"expected ShortBurnRate=%f, got %f",
					tt.expectedShortBR,
					result.ShortBurnRate,
				)
			}

			if math.Abs(
				result.LongBurnRate-tt.expectedLongBR,
			) > epsilon {
				t.Errorf(
					"expected LongBurnRate=%f, got %f",
					tt.expectedLongBR,
					result.LongBurnRate,
				)
			}
		})
	}
}
