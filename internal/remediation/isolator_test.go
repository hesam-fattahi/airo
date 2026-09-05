package remediation

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func remediationScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()

	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}

	if err := discoveryv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add discoveryv1 to scheme: %v", err)
	}

	return scheme
}

func TestIsolatePod(t *testing.T) {
	ctx := context.Background()

	t.Run("Successfully mutate pod label to traffic disabled", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "payment-pod-1",
				Namespace: "default",
				UID:       types.UID("uid-123"),
				Labels: map[string]string{
					"app":           "payment-api",
					TrafficLabelKey: TrafficLabelEnabled,
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(remediationScheme(t)).
			WithObjects(pod).
			Build()

		isolator := NewIsolator(fakeClient)

		err := isolator.IsolatePod(
			ctx,
			"default",
			"payment-pod-1",
			"uid-123",
		)
		if err != nil {
			t.Fatalf("unexpected error during pod isolation: %v", err)
		}

		updatedPod := &corev1.Pod{}

		err = fakeClient.Get(
			ctx,
			types.NamespacedName{
				Namespace: "default",
				Name:      "payment-pod-1",
			},
			updatedPod,
		)
		if err != nil {
			t.Fatalf("failed to retrieve updated pod: %v", err)
		}

		if updatedPod.Labels[TrafficLabelKey] != TrafficLabelDisabled {
			t.Errorf(
				"expected label %s=%s, got %s",
				TrafficLabelKey,
				TrafficLabelDisabled,
				updatedPod.Labels[TrafficLabelKey],
			)
		}
	})

	t.Run("Already isolated pod is idempotent", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "payment-pod-1",
				Namespace: "default",
				UID:       types.UID("uid-123"),
				Labels: map[string]string{
					TrafficLabelKey: TrafficLabelDisabled,
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(remediationScheme(t)).
			WithObjects(pod).
			Build()

		isolator := NewIsolator(fakeClient)

		err := isolator.IsolatePod(
			ctx,
			"default",
			"payment-pod-1",
			"uid-123",
		)
		if err != nil {
			t.Fatalf("expected idempotent isolation to succeed: %v", err)
		}
	})

	t.Run("Rejected on UID mismatch", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "payment-pod-1",
				Namespace: "default",
				UID:       types.UID("new-uid-999"),
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(remediationScheme(t)).
			WithObjects(pod).
			Build()

		isolator := NewIsolator(fakeClient)

		err := isolator.IsolatePod(
			ctx,
			"default",
			"payment-pod-1",
			"old-uid-123",
		)

		if err == nil {
			t.Fatal("expected error due to UID mismatch, got nil")
		}
	})
}

func TestIsIsolationVerified(t *testing.T) {
	ctx := context.Background()

	t.Run("Verified when pod IP is absent from EndpointSlice", func(t *testing.T) {
		slice := &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "payment-api-slice",
				Namespace: "default",
				Labels: map[string]string{
					"kubernetes.io/service-name": "payment-api",
				},
			},
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses: []string{"10.244.1.10"},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(remediationScheme(t)).
			WithObjects(slice).
			Build()

		isolator := NewIsolator(fakeClient)

		verified, err := isolator.IsIsolationVerified(
			ctx,
			"default",
			"payment-api",
			"10.244.1.5",
		)
		if err != nil {
			t.Fatalf("unexpected error during verification: %v", err)
		}

		if !verified {
			t.Error("expected isolation to be verified")
		}
	})

	t.Run("Not verified when pod IP is still present and ready", func(t *testing.T) {
		ready := true

		slice := &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "payment-api-slice",
				Namespace: "default",
				Labels: map[string]string{
					"kubernetes.io/service-name": "payment-api",
				},
			},
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses: []string{"10.244.1.5"},
					Conditions: discoveryv1.EndpointConditions{
						Ready: &ready,
					},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(remediationScheme(t)).
			WithObjects(slice).
			Build()

		isolator := NewIsolator(fakeClient)

		verified, err := isolator.IsIsolationVerified(
			ctx,
			"default",
			"payment-api",
			"10.244.1.5",
		)
		if err != nil {
			t.Fatalf("unexpected error during verification: %v", err)
		}

		if verified {
			t.Error("expected isolation to remain unverified while pod IP is present")
		}
	})

	t.Run("Not verified when pod IP remains present even if not ready", func(t *testing.T) {
		ready := false

		slice := &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "payment-api-slice",
				Namespace: "default",
				Labels: map[string]string{
					"kubernetes.io/service-name": "payment-api",
				},
			},
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses: []string{"10.244.1.5"},
					Conditions: discoveryv1.EndpointConditions{
						Ready: &ready,
					},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(remediationScheme(t)).
			WithObjects(slice).
			Build()

		isolator := NewIsolator(fakeClient)

		verified, err := isolator.IsIsolationVerified(
			ctx,
			"default",
			"payment-api",
			"10.244.1.5",
		)
		if err != nil {
			t.Fatalf("unexpected error during verification: %v", err)
		}

		if verified {
			t.Error("expected isolation to remain unverified while pod IP is present")
		}
	})

	t.Run("Verified when pod has no IP", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(remediationScheme(t)).
			Build()

		isolator := NewIsolator(fakeClient)

		verified, err := isolator.IsIsolationVerified(
			ctx,
			"default",
			"payment-api",
			"",
		)
		if err != nil {
			t.Fatalf("unexpected error during verification: %v", err)
		}

		if !verified {
			t.Error("expected pod without an IP to be treated as isolated")
		}
	})
}
