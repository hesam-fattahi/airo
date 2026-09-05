package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/hesam-fattahi/airo/api/v1alpha1"
	"github.com/hesam-fattahi/airo/internal/attribution"
	"github.com/hesam-fattahi/airo/internal/remediation"
	"github.com/hesam-fattahi/airo/internal/safety"
	"github.com/hesam-fattahi/airo/internal/slo"
	"github.com/hesam-fattahi/airo/internal/telemetry"
)

// RemediationPolicyReconciler reconciles a RemediationPolicy object
type RemediationPolicyReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	Recorder          record.EventRecorder
	TelemetryClient   telemetry.Client
	Isolator          remediation.Isolator
	Executor          remediation.Executor
	AttributionEngine *attribution.AttributionEngine
}

// Reconcile implements the level-triggered state machine control loop
func (r *RemediationPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Step 1: Fetch the RemediationPolicy custom resource
	policy := &v1alpha1.RemediationPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil // Policy was deleted
		}
		return ctrl.Result{}, fmt.Errorf("failed to fetch RemediationPolicy %s: %w", req.NamespacedName, err)
	}

	// Initialize Phase if empty
	if policy.Status.Phase == "" {
		err := r.updateStatus(ctx, req.NamespacedName, func(p *v1alpha1.RemediationPolicy) {
			p.Status.Phase = v1alpha1.PhaseHealthy
			p.Status.Message = "Policy initialized in Healthy state"
		})
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Step 2: Execute State Machine Phase Switch
	switch policy.Status.Phase {

	case v1alpha1.PhaseHealthy:
		return r.reconcileHealthy(ctx, req.NamespacedName, policy)

	case v1alpha1.PhaseViolationConfirmed:
		return r.reconcileViolationConfirmed(ctx, req.NamespacedName, policy)

	case v1alpha1.PhaseAttributionCompleted:
		return r.reconcileAttributionCompleted(ctx, req.NamespacedName, policy)

	case v1alpha1.PhaseSafetyApproved:
		return r.reconcileSafetyApproved(ctx, req.NamespacedName, policy)

	case v1alpha1.PhaseIsolationRequested:
		return r.reconcileIsolationRequested(ctx, req.NamespacedName, policy)

	case v1alpha1.PhaseIsolationVerified:
		return r.reconcileIsolationVerified(ctx, req.NamespacedName, policy)

	case v1alpha1.PhaseRecoveryRequested:
		return r.reconcileRecoveryRequested(ctx, req.NamespacedName, policy)

	case v1alpha1.PhaseEvaluationHoldoff:
		return r.reconcileEvaluationHoldoff(ctx, req.NamespacedName, policy)

	case v1alpha1.PhaseSystemicHalt:
		return r.reconcileSystemicHalt(ctx, req.NamespacedName, policy)

	default:
		logger.Error(nil, "Unknown remediation phase encountered; halting reconciliation", "phase", policy.Status.Phase)
		return ctrl.Result{}, fmt.Errorf("unknown remediation phase %q", policy.Status.Phase)
	}
}

// reconcileHealthy checks current telemetry against SLO burn rate thresholds
func (r *RemediationPolicyReconciler) reconcileHealthy(ctx context.Context, key types.NamespacedName, policy *v1alpha1.RemediationPolicy) (ctrl.Result, error) {
	if r.TelemetryClient == nil {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	// Query Fast Burn SLIs
	fastShortSLI, err := r.TelemetryClient.QueryDeploymentSLI(ctx, policy.Spec.TargetRef.Name, policy.Spec.Detection.FastBurn.ShortWindow, policy.Spec.SLO.LatencyThresholdMs)
	if err != nil || !fastShortSLI.HasTraffic {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	fastLongSLI, err := r.TelemetryClient.QueryDeploymentSLI(ctx, policy.Spec.TargetRef.Name, policy.Spec.Detection.FastBurn.LongWindow, policy.Spec.SLO.LatencyThresholdMs)
	if err != nil || !fastLongSLI.HasTraffic {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	fastResult, err := slo.EvaluateWindowPair(policy.Spec.SLO.Target, fastShortSLI.Value, fastLongSLI.Value, policy.Spec.Detection.FastBurn)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Query Slow Burn SLIs
	slowShortSLI, err := r.TelemetryClient.QueryDeploymentSLI(ctx, policy.Spec.TargetRef.Name, policy.Spec.Detection.SlowBurn.ShortWindow, policy.Spec.SLO.LatencyThresholdMs)
	if err != nil || !slowShortSLI.HasTraffic {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	slowLongSLI, err := r.TelemetryClient.QueryDeploymentSLI(ctx, policy.Spec.TargetRef.Name, policy.Spec.Detection.SlowBurn.LongWindow, policy.Spec.SLO.LatencyThresholdMs)
	if err != nil || !slowLongSLI.HasTraffic {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	slowResult, err := slo.EvaluateWindowPair(policy.Spec.SLO.Target, slowShortSLI.Value, slowLongSLI.Value, policy.Spec.Detection.SlowBurn)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update Status Metrics
	if err := r.updateStatus(ctx, key, func(p *v1alpha1.RemediationPolicy) {
		p.Status.ObservedSLI = fastShortSLI.Value
		p.Status.FastBurnRate = fastResult.ShortBurnRate
		p.Status.SlowBurnRate = slowResult.ShortBurnRate
	}); err != nil {
		return ctrl.Result{}, err
	}

	if fastResult.IsConfirmed || slowResult.IsConfirmed {
		r.eventf(policy, corev1.EventTypeWarning, "SLOBudgetExhausted",
			"SLO burn rate threshold breached: FastBurn=%.2fx, SlowBurn=%.2fx", fastResult.ShortBurnRate, slowResult.ShortBurnRate)

		err := r.updateStatus(ctx, key, func(p *v1alpha1.RemediationPolicy) {
			p.Status.Phase = v1alpha1.PhaseViolationConfirmed
			p.Status.Message = fmt.Sprintf("SLO burn rate breached: FastBurn=%.2fx, SlowBurn=%.2fx", fastResult.ShortBurnRate, slowResult.ShortBurnRate)
		})
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

// reconcileViolationConfirmed executes per-pod fault attribution
func (r *RemediationPolicyReconciler) reconcileViolationConfirmed(ctx context.Context, key types.NamespacedName, policy *v1alpha1.RemediationPolicy) (ctrl.Result, error) {
	if r.TelemetryClient == nil || r.AttributionEngine == nil {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	podMetrics, err := r.TelemetryClient.QueryPodSLIs(ctx, policy.Spec.TargetRef.Name, policy.Spec.Detection.FastBurn.ShortWindow, policy.Spec.SLO.LatencyThresholdMs)
	if err != nil {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	var podStats []attribution.PodSLIStat
	for _, m := range podMetrics {
		podStats = append(podStats, attribution.PodSLIStat{
			PodName:       m.PodName,
			PodUID:        m.PodUID,
			GoodRequests:  m.GoodRequestRate,
			TotalRequests: m.TotalRequestRate,
		})
	}

	attrResult := r.AttributionEngine.EvaluatePods(podStats)

	if attrResult.IsSystemicFailure {
		r.eventf(policy, corev1.EventTypeWarning, "SystemicFailureDetected",
			"Degradation is uniform across replicas; halting remediation to protect workload")

		err := r.updateStatus(ctx, key, func(p *v1alpha1.RemediationPolicy) {
			p.Status.Phase = v1alpha1.PhaseSystemicHalt
			p.Status.Message = attrResult.Reason
		})
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if attrResult.IsAttributable {
		err := r.updateStatus(ctx, key, func(p *v1alpha1.RemediationPolicy) {
			p.Status.Phase = v1alpha1.PhaseAttributionCompleted
			p.Status.TargetPodName = attrResult.TargetPodName
			p.Status.TargetPodUID = attrResult.TargetPodUID
			p.Status.AttributionConfidence = attrResult.ConfidenceScore
			p.Status.AttributionReason = attrResult.Reason
			p.Status.Message = fmt.Sprintf("Attributed fault to Pod %s (UID: %s)", attrResult.TargetPodName, attrResult.TargetPodUID)
		})
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

// reconcileAttributionCompleted checks safety policies (quorum, cooldown, budget)
func (r *RemediationPolicyReconciler) reconcileAttributionCompleted(ctx context.Context, key types.NamespacedName, policy *v1alpha1.RemediationPolicy) (ctrl.Result, error) {
	healthyCount, err := r.countHealthyReplicas(ctx, policy.Namespace, policy.Spec.TargetRef.Name)
	if err != nil {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	safetyEngine, err := safety.NewSafetyEngineFromConfig(policy.Spec.Safety)
	if err != nil {
		return ctrl.Result{}, err
	}

	var lastRemTime, windowStart *time.Time
	if policy.Status.LastRemediationTime != nil {
		lastRemTime = &policy.Status.LastRemediationTime.Time
	}
	if policy.Status.RemediationWindowStart != nil {
		windowStart = &policy.Status.RemediationWindowStart.Time
	}

	safetyResult := safetyEngine.EvaluateSafety(healthyCount, lastRemTime, windowStart, policy.Status.RemediationCountInWindow, time.Now())

	if !safetyResult.IsApproved {
		r.eventf(policy, corev1.EventTypeWarning, "SafetyCheckBlocked",
			"Safety policy blocked remediation: %s", safetyResult.Reason)

		_ = r.updateStatus(ctx, key, func(p *v1alpha1.RemediationPolicy) {
			p.Status.Message = fmt.Sprintf("Safety blocked: %s", safetyResult.Reason)
		})
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	err = r.updateStatus(ctx, key, func(p *v1alpha1.RemediationPolicy) {
		p.Status.Phase = v1alpha1.PhaseSafetyApproved
		p.Status.RemediationWindowStart = &metav1.Time{Time: safetyResult.WindowStart}
		p.Status.RemediationCountInWindow = safetyResult.RemediationCountInWindow
		p.Status.Message = "Safety checks passed; approving isolation"
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

// reconcileSafetyApproved mutates pod labels to isolate traffic
func (r *RemediationPolicyReconciler) reconcileSafetyApproved(ctx context.Context, key types.NamespacedName, policy *v1alpha1.RemediationPolicy) (ctrl.Result, error) {
	if r.Isolator == nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	err := r.Isolator.IsolatePod(ctx, policy.Namespace, policy.Status.TargetPodName, policy.Status.TargetPodUID)
	if err != nil {
		r.eventf(policy, corev1.EventTypeWarning, "IsolationFailed", "Failed to isolate pod: %v", err)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}

	r.eventf(policy, corev1.EventTypeNormal, "PodIsolationRequested",
		"Mutated traffic label on Pod %s to disabled", policy.Status.TargetPodName)

	err = r.updateStatus(ctx, key, func(p *v1alpha1.RemediationPolicy) {
		p.Status.Phase = v1alpha1.PhaseIsolationRequested
		p.Status.Message = fmt.Sprintf("Requested isolation for Pod %s", policy.Status.TargetPodName)
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
}

// reconcileIsolationRequested verifies EndpointSlice traffic removal
func (r *RemediationPolicyReconciler) reconcileIsolationRequested(ctx context.Context, key types.NamespacedName, policy *v1alpha1.RemediationPolicy) (ctrl.Result, error) {
	if r.Isolator == nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	pod := &corev1.Pod{}
	podKey := types.NamespacedName{Namespace: policy.Namespace, Name: policy.Status.TargetPodName}
	err := r.Get(ctx, podKey, pod)

	if apierrors.IsNotFound(err) {
		// Target pod disappeared; isolation verified implicitly
		err := r.updateStatus(ctx, key, func(p *v1alpha1.RemediationPolicy) {
			p.Status.Phase = v1alpha1.PhaseIsolationVerified
			p.Status.Message = "Target pod disappeared before isolation verification"
		})
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to fetch target pod during isolation verification: %w", err)
	}

	if string(pod.UID) != policy.Status.TargetPodUID {
		return ctrl.Result{}, fmt.Errorf("target pod UID changed during isolation verification: expected %s, got %s", policy.Status.TargetPodUID, pod.UID)
	}

	verified, err := r.Isolator.IsIsolationVerified(ctx, policy.Namespace, policy.Spec.TargetRef.Name, pod.Status.PodIP)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !verified {
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	}

	r.eventf(policy, corev1.EventTypeNormal, "PodIsolationVerified",
		"Verified Pod %s IP (%s) removed from active EndpointSlices", policy.Status.TargetPodName, pod.Status.PodIP)

	err = r.updateStatus(ctx, key, func(p *v1alpha1.RemediationPolicy) {
		p.Status.Phase = v1alpha1.PhaseIsolationVerified
		p.Status.Message = fmt.Sprintf("Verified traffic isolation for Pod %s", policy.Status.TargetPodName)
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

// reconcileIsolationVerified issues graceful pod deletion
func (r *RemediationPolicyReconciler) reconcileIsolationVerified(ctx context.Context, key types.NamespacedName, policy *v1alpha1.RemediationPolicy) (ctrl.Result, error) {
	if r.Executor == nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	err := r.Executor.RecoverPod(ctx, policy.Namespace, policy.Status.TargetPodName, policy.Status.TargetPodUID)
	if err != nil {
		r.eventf(policy, corev1.EventTypeWarning, "PodRecoveryFailed", "Failed to issue pod recovery: %v", err)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}

	now := metav1.Now()
	r.eventf(policy, corev1.EventTypeNormal, "PodRecoveryIssued",
		"Issued graceful deletion for Pod %s (UID: %s)", policy.Status.TargetPodName, policy.Status.TargetPodUID)

	err = r.updateStatus(ctx, key, func(p *v1alpha1.RemediationPolicy) {
		p.Status.Phase = v1alpha1.PhaseRecoveryRequested
		p.Status.LastRemediationTime = &now
		p.Status.RemediationCountInWindow++
		p.Status.Message = fmt.Sprintf("Issued graceful recovery for Pod %s", policy.Status.TargetPodName)
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

// reconcileRecoveryRequested sets evaluation holdoff timer
func (r *RemediationPolicyReconciler) reconcileRecoveryRequested(ctx context.Context, key types.NamespacedName, policy *v1alpha1.RemediationPolicy) (ctrl.Result, error) {
	holdoffUntil := metav1.NewTime(time.Now().Add(5 * time.Minute))

	err := r.updateStatus(ctx, key, func(p *v1alpha1.RemediationPolicy) {
		p.Status.Phase = v1alpha1.PhaseEvaluationHoldoff
		p.Status.EvaluationHoldoffUntil = &holdoffUntil
		p.Status.Message = "Entered 5-minute telemetry evaluation holdoff period"
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// reconcileEvaluationHoldoff waits for telemetry to clear and verifies recovery
func (r *RemediationPolicyReconciler) reconcileEvaluationHoldoff(ctx context.Context, key types.NamespacedName, policy *v1alpha1.RemediationPolicy) (ctrl.Result, error) {
	if r.TelemetryClient == nil {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	if policy.Status.EvaluationHoldoffUntil != nil && time.Now().Before(policy.Status.EvaluationHoldoffUntil.Time) {
		remaining := time.Until(policy.Status.EvaluationHoldoffUntil.Time)
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	// Holdoff expired; check if SLI recovered
	sliResult, err := r.TelemetryClient.QueryDeploymentSLI(ctx, policy.Spec.TargetRef.Name, policy.Spec.Detection.FastBurn.ShortWindow, policy.Spec.SLO.LatencyThresholdMs)
	if err == nil && sliResult.HasTraffic && sliResult.Value >= policy.Spec.SLO.Target {
		r.eventf(policy, corev1.EventTypeNormal, "SLORecovered",
			"SLO recovered to target threshold (SLI: %.4f)", sliResult.Value)

		err := r.updateStatus(ctx, key, func(p *v1alpha1.RemediationPolicy) {
			p.Status.Phase = v1alpha1.PhaseHealthy
			p.Status.TargetPodName = ""
			p.Status.TargetPodUID = ""
			p.Status.AttributionReason = ""
			p.Status.Message = fmt.Sprintf("SLO recovered successfully (SLI: %.4f)", sliResult.Value)
		})
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	// Still degraded; re-evaluate
	err = r.updateStatus(ctx, key, func(p *v1alpha1.RemediationPolicy) {
		p.Status.Phase = v1alpha1.PhaseViolationConfirmed
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

// reconcileSystemicHalt re-evaluates telemetry to exit systemic halt if backend recovers
func (r *RemediationPolicyReconciler) reconcileSystemicHalt(ctx context.Context, key types.NamespacedName, policy *v1alpha1.RemediationPolicy) (ctrl.Result, error) {
	if r.TelemetryClient == nil {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	sliResult, err := r.TelemetryClient.QueryDeploymentSLI(ctx, policy.Spec.TargetRef.Name, policy.Spec.Detection.FastBurn.ShortWindow, policy.Spec.SLO.LatencyThresholdMs)
	if err == nil && sliResult.HasTraffic && sliResult.Value >= policy.Spec.SLO.Target {
		err := r.updateStatus(ctx, key, func(p *v1alpha1.RemediationPolicy) {
			p.Status.Phase = v1alpha1.PhaseHealthy
			p.Status.Message = "Workload recovered from systemic degradation"
		})
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// countHealthyReplicas counts pods that are Running, Ready=True, and non-terminating
func (r *RemediationPolicyReconciler) countHealthyReplicas(ctx context.Context, namespace, deploymentName string) (int32, error) {
	podList := &corev1.PodList{}
	err := r.List(ctx, podList, client.InNamespace(namespace), client.MatchingLabels{"app": deploymentName})
	if err != nil {
		return 0, err
	}

	var healthy int32
	for _, pod := range podList.Items {
		// Ignore terminating pods
		if pod.DeletionTimestamp != nil {
			continue
		}

		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		// Verify Ready condition is True
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				healthy++
				break
			}
		}
	}
	return healthy, nil
}

func (r *RemediationPolicyReconciler) eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	if r.Recorder != nil {
		r.Recorder.Eventf(object, eventtype, reason, messageFmt, args...)
	}
}

func (r *RemediationPolicyReconciler) updateStatus(ctx context.Context, key types.NamespacedName, updateFunc func(*v1alpha1.RemediationPolicy)) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latestPolicy := &v1alpha1.RemediationPolicy{}
		if err := r.Get(ctx, key, latestPolicy); err != nil {
			return err
		}
		updateFunc(latestPolicy)
		return r.Status().Update(ctx, latestPolicy)
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *RemediationPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.RemediationPolicy{}).
		Complete(r)
}
