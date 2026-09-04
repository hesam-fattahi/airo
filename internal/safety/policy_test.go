package safety

import (
	"testing"
	"time"

	"github.com/hesam-fattahi/airo/api/v1alpha1"
)

func TestEvaluateSafety(t *testing.T) {
	now := time.Now()

	config := v1alpha1.SafetyConfig{
		MinHealthyReplicas:       3,
		CooldownDuration:         "5m",
		MaxRemediationsPerWindow: 2,
		RemediationWindow:        "1h",
	}

	engine, err := NewSafetyEngineFromConfig(config)
	if err != nil {
		t.Fatalf("Failed to initialize safety engine: %v", err)
	}

	t.Run("Approved - Remediation Leaves Exactly Minimum Healthy Replicas", func(t *testing.T) {
		lastRemediation := now.Add(-10 * time.Minute)
		windowStart := now.Add(-20 * time.Minute)

		// 4 healthy replicas - 1 removed = 3 remaining (equals minHealthy 3) -> APPROVED!
		result := engine.EvaluateSafety(
			4,
			&lastRemediation,
			&windowStart,
			0,
			now,
		)

		if !result.IsApproved {
			t.Fatalf("Expected approval because 3 healthy replicas would remain, got rejected: %s", result.Reason)
		}
	})

	t.Run("Rejected - Remediation Would Violate Quorum", func(t *testing.T) {
		lastRemediation := now.Add(-10 * time.Minute)
		windowStart := now.Add(-20 * time.Minute)

		// 3 healthy replicas - 1 removed = 2 remaining (below minHealthy 3) -> REJECTED!
		result := engine.EvaluateSafety(
			3,
			&lastRemediation,
			&windowStart,
			0,
			now,
		)

		if result.IsApproved {
			t.Fatalf("Expected rejection because remediation would leave 2 healthy replicas (< 3)")
		}
	})

	t.Run("Rejected - Cooldown Active", func(t *testing.T) {
		lastRemediation := now.Add(-2 * time.Minute) // Only 2m ago (Cooldown is 5m)
		windowStart := now.Add(-20 * time.Minute)

		result := engine.EvaluateSafety(
			4,
			&lastRemediation,
			&windowStart,
			0,
			now,
		)

		if result.IsApproved {
			t.Fatalf("Expected rejection due to active cooldown")
		}
	})

	t.Run("Rejected - Budget Exhausted", func(t *testing.T) {
		lastRemediation := now.Add(-10 * time.Minute)
		windowStart := now.Add(-20 * time.Minute)

		result := engine.EvaluateSafety(
			4,
			&lastRemediation,
			&windowStart,
			2, // Reached max 2 remediations in current window!
			now,
		)

		if result.IsApproved {
			t.Fatalf("Expected rejection due to budget exhaustion")
		}
	})

	t.Run("Approved - Window Expired and Budget Reset", func(t *testing.T) {
		lastRemediation := now.Add(-10 * time.Minute)
		windowStart := now.Add(-70 * time.Minute) // 70m ago (Window is 1h -> EXPIRED!)

		result := engine.EvaluateSafety(
			4,
			&lastRemediation,
			&windowStart,
			2,
			now,
		)

		if !result.IsApproved {
			t.Fatalf("Expected budget reset and approval after window expiration, got rejected: %s", result.Reason)
		}

		if result.RemediationCountInWindow != 0 {
			t.Errorf("Expected count reset to 0, got %d", result.RemediationCountInWindow)
		}
	})

	t.Run("Validation Error - minHealthyReplicas <= 0", func(t *testing.T) {
		badConfig := config
		badConfig.MinHealthyReplicas = 0
		_, err := NewSafetyEngineFromConfig(badConfig)
		if err == nil {
			t.Fatalf("Expected error for minHealthyReplicas <= 0, got nil")
		}
	})

	t.Run("Validation Error - maxRemediationsPerWindow <= 0", func(t *testing.T) {
		badConfig := config
		badConfig.MaxRemediationsPerWindow = 0
		_, err := NewSafetyEngineFromConfig(badConfig)
		if err == nil {
			t.Fatalf("Expected error for maxRemediationsPerWindow <= 0, got nil")
		}
	})

	t.Run("Validation Error - Invalid Cooldown Duration String", func(t *testing.T) {
		badConfig := config
		badConfig.CooldownDuration = "invalid-duration"
		_, err := NewSafetyEngineFromConfig(badConfig)
		if err == nil {
			t.Fatalf("Expected error for invalid CooldownDuration string, got nil")
		}
	})
}
