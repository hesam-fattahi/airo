package remediation

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func executorScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()

	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}

	return scheme
}

func TestRecoverPod(t *testing.T) {
	ctx := context.Background()

	t.Run("Successfully deletes pod by immutable UID", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "payment-pod-1",
				Namespace: "default",
				UID:       types.UID("uid-123"),
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(executorScheme(t)).
			WithObjects(pod).
			Build()

		executor := NewExecutor(fakeClient)

		err := executor.RecoverPod(
			ctx,
			"default",
			"payment-pod-1",
			"uid-123",
		)
		if err != nil {
			t.Fatalf("unexpected error during pod recovery: %v", err)
		}

		deletedPod := &corev1.Pod{}

		err = fakeClient.Get(
			ctx,
			types.NamespacedName{
				Namespace: "default",
				Name:      "payment-pod-1",
			},
			deletedPod,
		)

		if !apierrors.IsNotFound(err) {
			t.Errorf(
				"expected pod to be deleted and return NotFound, got error: %v",
				err,
			)
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
			WithScheme(executorScheme(t)).
			WithObjects(pod).
			Build()

		executor := NewExecutor(fakeClient)

		err := executor.RecoverPod(
			ctx,
			"default",
			"payment-pod-1",
			"old-uid-123",
		)

		if err == nil {
			t.Fatal("expected error due to UID mismatch, got nil")
		}
	})

	t.Run("Already deleted pod is idempotent success", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(executorScheme(t)).
			Build()

		executor := NewExecutor(fakeClient)

		err := executor.RecoverPod(
			ctx,
			"default",
			"payment-pod-1",
			"uid-123",
		)

		if err != nil {
			t.Fatalf(
				"expected already deleted pod to be idempotent success, got: %v",
				err,
			)
		}
	})
}
