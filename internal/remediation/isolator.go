package remediation

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	TrafficLabelKey      = "airo.io/traffic"
	TrafficLabelDisabled = "disabled"
	TrafficLabelEnabled  = "enabled"
)

// Isolator defines the interface for mutating pod labels and verifying
// that traffic isolation has propagated to EndpointSlices.
type Isolator interface {
	IsolatePod(ctx context.Context, namespace, podName, podUID string) error
	IsIsolationVerified(ctx context.Context, namespace, serviceName, podIP string) (bool, error)
}

type podIsolator struct {
	client client.Client
}

// NewIsolator initializes a new pod isolator.
func NewIsolator(c client.Client) Isolator {
	return &podIsolator{
		client: c,
	}
}

// IsolatePod mutates the target pod's traffic label to disabled.
//
// The pod is first fetched by namespace/name and its immutable UID is verified
// before any mutation occurs.
func (i *podIsolator) IsolatePod(
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

	if err := i.client.Get(
		ctx,
		types.NamespacedName{
			Namespace: namespace,
			Name:      podName,
		},
		pod,
	); err != nil {
		return fmt.Errorf(
			"failed to fetch pod %s/%s for isolation: %w",
			namespace,
			podName,
			err,
		)
	}

	// Immutable UID safety check.
	if string(pod.UID) != podUID {
		return fmt.Errorf(
			"pod UID mismatch during isolation: expected %s, got %s",
			podUID,
			pod.UID,
		)
	}

	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}

	// For the sake of idempotency: if the pod is already isolated, no API update is required.
	if pod.Labels[TrafficLabelKey] == TrafficLabelDisabled {
		return nil
	}

	pod.Labels[TrafficLabelKey] = TrafficLabelDisabled

	if err := i.client.Update(ctx, pod); err != nil {
		return fmt.Errorf(
			"failed to update pod isolation labels for %s/%s: %w",
			namespace,
			podName,
			err,
		)
	}

	return nil
}

// IsIsolationVerified confirms that the target pod IP no longer appears in any
// EndpointSlice associated with the Service.
//
// AIRO intentionally uses a strict definition of isolation: the pod IP must be
// absent from EndpointSlices entirely. A present endpoint with Ready=false is
// not treated as fully verified isolation because the state-machine contract is
// explicit removal from Service endpoints.
func (i *podIsolator) IsIsolationVerified(
	ctx context.Context,
	namespace string,
	serviceName string,
	podIP string,
) (bool, error) {
	if namespace == "" {
		return false, fmt.Errorf("namespace must not be empty")
	}

	if serviceName == "" {
		return false, fmt.Errorf("service name must not be empty")
	}

	// A pod without an assigned IP cannot receive cluster Service traffic.
	if podIP == "" {
		return true, nil
	}

	sliceList := &discoveryv1.EndpointSliceList{}

	if err := i.client.List(
		ctx,
		sliceList,
		client.InNamespace(namespace),
		client.MatchingLabels{
			"kubernetes.io/service-name": serviceName,
		},
	); err != nil {
		return false, fmt.Errorf(
			"failed to list EndpointSlices for service %s/%s: %w",
			namespace,
			serviceName,
			err,
		)
	}

	for _, slice := range sliceList.Items {
		for _, endpoint := range slice.Endpoints {
			for _, address := range endpoint.Addresses {
				if address == podIP {
					return false, nil
				}
			}
		}
	}

	return true, nil
}
