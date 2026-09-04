package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RemediationPhase represents the current state machine phase.
// Each phase transition is persisted atomically in CR status so reconciliation
// can safely resume after controller restart or reconciliation interruption.
type RemediationPhase string

const (
	PhaseHealthy              RemediationPhase = "Healthy"
	PhaseViolationConfirmed   RemediationPhase = "ViolationConfirmed"
	PhaseAttributionCompleted RemediationPhase = "AttributionCompleted"
	PhaseSystemicHalt         RemediationPhase = "SystemicHalt" // Non-destructive observation state when failures are uniform across all pods
	PhaseSafetyApproved       RemediationPhase = "SafetyApproved"
	PhaseIsolationRequested   RemediationPhase = "IsolationRequested"
	PhaseIsolationVerified    RemediationPhase = "IsolationVerified"
	PhaseRecoveryRequested    RemediationPhase = "RecoveryRequested"
	PhaseEvaluationHoldoff    RemediationPhase = "EvaluationHoldoff"
	PhaseRemediationFailed    RemediationPhase = "RemediationFailed"
)

// TargetRef defines the Kubernetes Deployment object to monitor in the policy's namespace
type TargetRef struct {
	// Name of the target Deployment
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// SLOConfig defines the target SLO and latency thresholds
type SLOConfig struct {
	// Target ratio (e.g. 0.99 for 99.0% SLO)
	// +kubebuilder:validation:Minimum=0.0
	// +kubebuilder:validation:Maximum=1.0
	Target float64 `json:"target"`

	// Latency threshold in milliseconds (e.g. 50 for 50ms)
	// +kubebuilder:validation:Minimum=1
	LatencyThresholdMs int64 `json:"latencyThresholdMs"`
}

// BurnRateWindow configures short/long evaluation windows and burn rate threshold for a policy
type BurnRateWindow struct {
	// Short evaluation window (e.g. "5m" or "30m")
	// +kubebuilder:validation:Pattern=`^([0-9]+(s|m|h))+$`
	ShortWindow string `json:"shortWindow"`

	// Long evaluation window (e.g. "1h" or "6h")
	// +kubebuilder:validation:Pattern=`^([0-9]+(s|m|h))+$`
	LongWindow string `json:"longWindow"`

	// Burn rate threshold multiplier (e.g. 14.4 or 6.0)
	// +kubebuilder:validation:Minimum=0.0
	Threshold float64 `json:"threshold"`
}

// DetectionConfig defines independent multi-window burn rate policies
type DetectionConfig struct {
	FastBurn BurnRateWindow `json:"fastBurn"`
	SlowBurn BurnRateWindow `json:"slowBurn"`
}

// SafetyConfig defines quorum checks and bounded remediation budgets
type SafetyConfig struct {
	// Minimum number of healthy replicas required to allow remediation
	// +kubebuilder:validation:Minimum=1
	MinHealthyReplicas int32 `json:"minHealthyReplicas"`

	// Cooldown duration between remediations (e.g. "5m")
	// +kubebuilder:validation:Pattern=`^([0-9]+(s|m|h))+$`
	CooldownDuration string `json:"cooldownDuration"`

	// Maximum allowed remediations within the remediation window
	// +kubebuilder:validation:Minimum=1
	MaxRemediationsPerWindow int32 `json:"maxRemediationsPerWindow"`

	// Time window for maximum remediation budget tracking (e.g. "1h")
	// +kubebuilder:validation:Pattern=`^([0-9]+(s|m|h))+$`
	RemediationWindow string `json:"remediationWindow"`
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
	Phase RemediationPhase `json:"phase,omitempty"`

	ObservedSLI          float64 `json:"observedSLI,omitempty"`
	ObservedP99LatencyMs float64 `json:"observedP99LatencyMs,omitempty"`

	FastBurnRate float64 `json:"fastBurnRate,omitempty"`
	SlowBurnRate float64 `json:"slowBurnRate,omitempty"`

	TargetPodUID  string `json:"targetPodUID,omitempty"`
	TargetPodName string `json:"targetPodName,omitempty"`

	AttributionConfidence float64 `json:"attributionConfidence,omitempty"`
	AttributionReason     string  `json:"attributionReason,omitempty"`

	LastRemediationTime    *metav1.Time `json:"lastRemediationTime,omitempty"`
	EvaluationHoldoffUntil *metav1.Time `json:"evaluationHoldoffUntil,omitempty"`

	RemediationWindowStart   *metav1.Time `json:"remediationWindowStart,omitempty"`
	RemediationCountInWindow int32        `json:"remediationCountInWindow,omitempty"`

	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RemediationPolicy is the Schema for the remediationpolicies API
type RemediationPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemediationPolicySpec   `json:"spec"`
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
