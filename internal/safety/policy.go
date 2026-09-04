package safety

import (
	"fmt"
	"time"

	"github.com/hesam-fattahi/airo/api/v1alpha1"
)

// SafetyResult contains the decision and normalized remediation budget state.
type SafetyResult struct {
	IsApproved               bool
	Reason                   string
	WindowStart              time.Time
	RemediationCountInWindow int32
}

// SafetyEngine enforces quorum protection, cooldown periods,
// and bounded remediation budgets.
type SafetyEngine struct {
	MinHealthyReplicas       int32
	CooldownDuration         time.Duration
	MaxRemediationsPerWindow int32
	RemediationWindow        time.Duration
}

// NewSafetyEngineFromConfig validates SafetyConfig and creates a SafetyEngine.
func NewSafetyEngineFromConfig(config v1alpha1.SafetyConfig) (*SafetyEngine, error) {
	if config.MinHealthyReplicas <= 0 {
		return nil, fmt.Errorf(
			"minHealthyReplicas must be greater than 0, got %d",
			config.MinHealthyReplicas,
		)
	}

	if config.MaxRemediationsPerWindow <= 0 {
		return nil, fmt.Errorf(
			"maxRemediationsPerWindow must be greater than 0, got %d",
			config.MaxRemediationsPerWindow,
		)
	}

	cooldown, err := time.ParseDuration(config.CooldownDuration)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid cooldownDuration %q: %w",
			config.CooldownDuration,
			err,
		)
	}

	if cooldown <= 0 {
		return nil, fmt.Errorf(
			"cooldownDuration must be greater than 0, got %q",
			config.CooldownDuration,
		)
	}

	window, err := time.ParseDuration(config.RemediationWindow)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid remediationWindow %q: %w",
			config.RemediationWindow,
			err,
		)
	}

	if window <= 0 {
		return nil, fmt.Errorf(
			"remediationWindow must be greater than 0, got %q",
			config.RemediationWindow,
		)
	}

	return &SafetyEngine{
		MinHealthyReplicas:       config.MinHealthyReplicas,
		CooldownDuration:         cooldown,
		MaxRemediationsPerWindow: config.MaxRemediationsPerWindow,
		RemediationWindow:        window,
	}, nil
}

// EvaluateSafety determines whether AIRO may perform one destructive
// remediation action.
//
// This method does not consume remediation budget. The controller must update
// remediation accounting only after the remediation action is successfully issued.
func (e *SafetyEngine) EvaluateSafety(
	healthyReplicas int32,
	lastRemediationTime *time.Time,
	windowStart *time.Time,
	remediationsInWindow int32,
	currentTime time.Time,
) SafetyResult {

	// Pillar 1: Quorum protection.
	remainingHealthyReplicas := healthyReplicas - 1

	if remainingHealthyReplicas < e.MinHealthyReplicas {
		return SafetyResult{
			IsApproved: false,
			Reason: fmt.Sprintf(
				"quorum protection blocked remediation: %d healthy replicas would remain after remediation, below minimum required %d",
				remainingHealthyReplicas,
				e.MinHealthyReplicas,
			),
		}
	}

	// Pillar 2: Cooldown protection.
	if lastRemediationTime != nil && !lastRemediationTime.IsZero() {
		elapsed := currentTime.Sub(*lastRemediationTime)

		if elapsed < e.CooldownDuration {
			remaining := e.CooldownDuration - elapsed

			return SafetyResult{
				IsApproved: false,
				Reason: fmt.Sprintf(
					"cooldown active: %s remaining before another remediation may be attempted",
					remaining.Round(time.Second),
				),
			}
		}
	}

	// Pillar 3: Bounded remediation budget.
	normalizedWindowStart := currentTime
	normalizedCount := remediationsInWindow

	if windowStart == nil || windowStart.IsZero() {
		normalizedWindowStart = currentTime
		normalizedCount = 0
	} else {
		elapsed := currentTime.Sub(*windowStart)

		if elapsed >= e.RemediationWindow {
			normalizedWindowStart = currentTime
			normalizedCount = 0
		} else {
			normalizedWindowStart = *windowStart
		}
	}

	if normalizedCount >= e.MaxRemediationsPerWindow {
		return SafetyResult{
			IsApproved: false,
			Reason: fmt.Sprintf(
				"remediation budget exhausted: %d of %d remediations already used in the current window",
				normalizedCount,
				e.MaxRemediationsPerWindow,
			),
			WindowStart:              normalizedWindowStart,
			RemediationCountInWindow: normalizedCount,
		}
	}

	return SafetyResult{
		IsApproved:               true,
		Reason:                   "all safety checks passed",
		WindowStart:              normalizedWindowStart,
		RemediationCountInWindow: normalizedCount,
	}
}
