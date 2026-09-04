package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RemediationPhase represents the current state machine phase
type RemediationPhase string

const (
	PhaseHealthy              RemediationPhase = "Healthy"
	PhaseViolationConfirmed   RemediationPhase = "ViolationConfirmed"
	PhaseAttributionCompleted RemediationPhase = "AttributionCompleted"
	PhaseSystemicHalt         RemediationPhase = "SystemicHalt"
	PhaseSafetyApproved       RemediationPhase = "SafetyApproved"
	PhaseIsolationRequested   RemediationPhase = "IsolationRequested"
	PhaseIsolationVerified    RemediationPhase = "IsolationVerified"
	PhaseRecoveryRequested    RemediationPhase = "RecoveryRequested"
	PhaseEvaluationHoldoff    RemediationPhase = "EvaluationHoldoff"
	PhaseRemediationFailed    RemediationPhase = "RemediationFailed"
)

// TargetRef defines the Kubernetes Deployment object to monitor
type TargetRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// SLOConfig defines the target SLO and latency thresholds
type SLOConfig struct {
	Target             float64 `json:"target"`             // e.g. 0.99 (99.0%)
	LatencyThresholdMs int64   `json:"latencyThresholdMs"` // e.g. 50 (50ms)
}

// DetectionConfig defines multi-window burn rate thresholds
type DetectionConfig struct {
	ShortWindow       string  `json:"shortWindow"`       // e.g. "5m"
	LongWindow        string  `json:"longWindow"`        // e.g. "1h"
	FastBurnThreshold float64 `json:"fastBurnThreshold"` // e.g. 14.4
	SlowBurnThreshold float64 `json:"slowBurnThreshold"` // e.g. 6.0
}

// SafetyConfig defines quorum checks and bounded budgets
type SafetyConfig struct {
	MinHealthyReplicas       int32  `json:"minHealthyReplicas"`       // e.g. 3
	CooldownDuration         string `json:"cooldownDuration"`         // e.g. "5m"
	MaxRemediationsPerWindow int32  `json:"maxRemediationsPerWindow"` // e.g. 2
	RemediationWindow        string `json:"remediationWindow"`        // e.g. "1h"
}

// RemediationPolicySpec defines the desired state of RemediationPolicy
type RemediationPolicySpec struct {
	TargetRef TargetRef       `json:"targetRef"`
	SLO       SLOConfig       `json:"slo"`
	Detection DetectionConfig `json:"detection"`
	Safety    SafetyConfig    `json:"safety"`
}

// RemediationPolicyStatus defines the observed state of RemediationPolicy
type RemediationPolicyStatus struct {
	Phase                    RemediationPhase `json:"phase,omitempty"`
	ObservedSLI              float64          `json:"observedSLI,omitempty"`
	ShortWindowBurnRate      float64          `json:"shortWindowBurnRate,omitempty"`
	LongWindowBurnRate       float64          `json:"longWindowBurnRate,omitempty"`
	TargetPodUID             string           `json:"targetPodUID,omitempty"`
	TargetPodName            string           `json:"targetPodName,omitempty"`
	LastRemediationTime      *metav1.Time     `json:"lastRemediationTime,omitempty"`
	EvaluationHoldoffUntil   *metav1.Time     `json:"evaluationHoldoffUntil,omitempty"`
	RemediationCountInWindow int32            `json:"remediationCountInWindow,omitempty"`
	Message                  string           `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RemediationPolicy is the Schema for the remediationpolicies API
type RemediationPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemediationPolicySpec   `json:"spec,omitempty"`
	Status RemediationPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RemediationPolicyList contains a list of RemediationPolicy
type RemediationPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemediationPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RemediationPolicy{}, &RemediationPolicyList{})
}
