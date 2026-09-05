package controller

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/hesam-fattahi/airo/api/v1alpha1"
	"github.com/hesam-fattahi/airo/internal/telemetry"
)

type mockTelemetryClient struct {
	sliResult telemetry.SLIResult
	err       error
}

func (m *mockTelemetryClient) QueryDeploymentSLI(ctx context.Context, appName, window string, latencyThresholdMs int64) (telemetry.SLIResult, error) {
	return m.sliResult, m.err
}

func (m *mockTelemetryClient) QueryPodSLIs(ctx context.Context, appName, window string, latencyThresholdMs int64) ([]telemetry.PodSLIMetric, error) {
	return nil, nil
}

func controllerScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	return scheme
}

func TestReconcileInitializeStatus(t *testing.T) {
	ctx := context.Background()
	scheme := controllerScheme(t)

	policy := &v1alpha1.RemediationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payment-api-policy",
			Namespace: "default",
		},
		Spec: v1alpha1.RemediationPolicySpec{
			TargetRef: v1alpha1.TargetRef{Name: "payment-api"},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(policy).WithStatusSubresource(policy).Build()

	reconciler := &RemediationPolicyReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "payment-api-policy",
		},
	}

	res, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Unexpected error during reconcile: %v", err)
	}

	if !res.Requeue {
		t.Errorf("Expected requeue after phase initialization")
	}

	updatedPolicy := &v1alpha1.RemediationPolicy{}
	_ = fakeClient.Get(ctx, req.NamespacedName, updatedPolicy)

	if updatedPolicy.Status.Phase != v1alpha1.PhaseHealthy {
		t.Errorf("Expected initialized phase to be Healthy, got %s", updatedPolicy.Status.Phase)
	}
}

func TestReconcileEvaluationHoldoff(t *testing.T) {
	ctx := context.Background()
	scheme := controllerScheme(t)

	t.Run("Holdoff Active - Requeues for remaining duration", func(t *testing.T) {
		future := metav1.NewTime(time.Now().Add(3 * time.Minute))
		policy := &v1alpha1.RemediationPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "policy-1", Namespace: "default"},
			Spec: v1alpha1.RemediationPolicySpec{
				TargetRef: v1alpha1.TargetRef{Name: "payment-api"},
				SLO:       v1alpha1.SLOConfig{Target: 0.99, LatencyThresholdMs: 50},
			},
			Status: v1alpha1.RemediationPolicyStatus{
				Phase:                  v1alpha1.PhaseEvaluationHoldoff,
				EvaluationHoldoffUntil: &future,
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(policy).WithStatusSubresource(policy).Build()
		reconciler := &RemediationPolicyReconciler{
			Client:          fakeClient,
			Scheme:          scheme,
			Recorder:        record.NewFakeRecorder(10),
			TelemetryClient: &mockTelemetryClient{sliResult: telemetry.SLIResult{Value: 0.99, HasTraffic: true}},
		}

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "policy-1"}}
		res, err := reconciler.Reconcile(ctx, req)

		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if res.RequeueAfter <= 0 || res.RequeueAfter > 3*time.Minute {
			t.Errorf("Expected RequeueAfter around 3 minutes, got %v", res.RequeueAfter)
		}
	})

	t.Run("Holdoff Expired & SLI Recovered - Transitions to Healthy", func(t *testing.T) {
		past := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		policy := &v1alpha1.RemediationPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "policy-1", Namespace: "default"},
			Spec: v1alpha1.RemediationPolicySpec{
				TargetRef: v1alpha1.TargetRef{Name: "payment-api"},
				SLO:       v1alpha1.SLOConfig{Target: 0.99, LatencyThresholdMs: 50},
			},
			Status: v1alpha1.RemediationPolicyStatus{
				Phase:                  v1alpha1.PhaseEvaluationHoldoff,
				EvaluationHoldoffUntil: &past,
				TargetPodName:          "payment-pod-1",
				TargetPodUID:           "uid-123",
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(policy).WithStatusSubresource(policy).Build()
		reconciler := &RemediationPolicyReconciler{
			Client:          fakeClient,
			Scheme:          scheme,
			Recorder:        record.NewFakeRecorder(10),
			TelemetryClient: &mockTelemetryClient{sliResult: telemetry.SLIResult{Value: 0.995, HasTraffic: true}},
		}

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "policy-1"}}
		_, err := reconciler.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		updatedPolicy := &v1alpha1.RemediationPolicy{}
		_ = fakeClient.Get(ctx, req.NamespacedName, updatedPolicy)

		if updatedPolicy.Status.Phase != v1alpha1.PhaseHealthy {
			t.Errorf("Expected phase Healthy after recovery, got %s", updatedPolicy.Status.Phase)
		}
	})
}
