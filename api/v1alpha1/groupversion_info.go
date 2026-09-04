package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// Register objects
	GroupVersion = schema.GroupVersion{Group: "reliability.airo.io", Version: "v1alpha1"}

	// Add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// Add the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
