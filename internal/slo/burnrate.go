package slo

import (
	"fmt"
	"math"

	"github.com/hesam-fattahi/airo/api/v1alpha1"
)

// WindowBurnRateResult holds calculated burn rates and confirmation
// information for a single short/long window pair.
type WindowBurnRateResult struct {
	ShortSLI      float64
	LongSLI       float64
	ShortBurnRate float64
	LongBurnRate  float64
	IsConfirmed   bool
}

// EvaluateWindowPair calculates error budget burn rates for a short/long
// window pair and determines whether both windows breach the configured
// threshold.
//
// A violation is confirmed only when BOTH windows are at or above the
// configured burn-rate threshold.
func EvaluateWindowPair(
	targetSLO float64,
	shortSLI float64,
	longSLI float64,
	window v1alpha1.BurnRateWindow,
) (WindowBurnRateResult, error) {
	if targetSLO <= 0.0 || targetSLO >= 1.0 {
		return WindowBurnRateResult{}, fmt.Errorf(
			"target SLO must be between 0.0 and 1.0 (exclusive), got %f",
			targetSLO,
		)
	}

	if shortSLI < 0.0 || shortSLI > 1.0 {
		return WindowBurnRateResult{}, fmt.Errorf(
			"short SLI must be between 0.0 and 1.0, got %f",
			shortSLI,
		)
	}

	if longSLI < 0.0 || longSLI > 1.0 {
		return WindowBurnRateResult{}, fmt.Errorf(
			"long SLI must be between 0.0 and 1.0, got %f",
			longSLI,
		)
	}

	if window.Threshold <= 0.0 {
		return WindowBurnRateResult{}, fmt.Errorf(
			"burn rate threshold must be positive, got %f",
			window.Threshold,
		)
	}

	allowedErrorBudget := 1.0 - targetSLO

	shortErrorRatio := 1.0 - shortSLI
	longErrorRatio := 1.0 - longSLI

	// Calculate raw burn rates
	shortBurnRate := shortErrorRatio / allowedErrorBudget
	longBurnRate := longErrorRatio / allowedErrorBudget

	// Clean IEEE 754 floating-point precision noise by rounding to 6 decimal places
	shortBurnRate = math.Round(shortBurnRate*1e6) / 1e6
	longBurnRate = math.Round(longBurnRate*1e6) / 1e6

	// Multi-window confirmation:
	// both the short and long windows must breach the threshold.
	isConfirmed :=
		shortBurnRate >= window.Threshold &&
			longBurnRate >= window.Threshold

	return WindowBurnRateResult{
		ShortSLI:      shortSLI,
		LongSLI:       longSLI,
		ShortBurnRate: shortBurnRate,
		LongBurnRate:  longBurnRate,
		IsConfirmed:   isConfirmed,
	}, nil
}
