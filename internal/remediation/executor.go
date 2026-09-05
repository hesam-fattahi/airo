package remediation

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Executor defines the interface for executing graceful pod recovery.
type Executor interface {
	RecoverPod(ctx context.Context, namespace, podName, podUID string) error
}

type podExecutor struct {
	client client.Client
}

// NewExecutor initializes a new pod executor.
func NewExecutor(c client.Client) Executor {
	return &podExecutor{
		client: c,
	}
}

// RecoverPod issues a graceful Kubernetes Pod deletion request.
func (e *podExecutor) RecoverPod(
	ctx context.Context,
	namespace string,
	podName string,
	podUID string,
) error {
	if namespace == "" {
		return fmt.Errorf("namespace must not be empty")
	}

	if podName == "" {
		return fmt.Errorf("pod name must not be empty")
	}

	if podUID == "" {
		return fmt.Errorf("pod UID must not be empty")
	}

	pod := &corev1.Pod{}

	if err := e.client.Get(
		ctx,
		types.NamespacedName{
			Namespace: namespace,
			Name:      podName,
		},
		pod,
	); err != nil {
		if apierrors.IsNotFound(err) {
			// Idempotent success: the target pod is already gone.
			return nil
		}

		return fmt.Errorf(
			"failed to fetch pod %s/%s for recovery: %w",
			namespace,
			podName,
			err,
		)
	}

	// Immutable UID safety check.
	if string(pod.UID) != podUID {
		return fmt.Errorf(
			"pod UID mismatch during recovery: expected %s, got %s",
			podUID,
			pod.UID,
		)
	}

	uid := types.UID(podUID)

	deleteOptions := &client.DeleteOptions{
		Preconditions: &metav1.Preconditions{
			UID: &uid,
		},
	}

	if err := e.client.Delete(ctx, pod, deleteOptions); err != nil {
		if apierrors.IsNotFound(err) {
			// Idempotent success: deleted concurrently.
			return nil
		}

		return fmt.Errorf(
			"failed to issue pod deletion API call for %s/%s: %w",
			namespace,
			podName,
			err,
		)
	}

	return nil
}
