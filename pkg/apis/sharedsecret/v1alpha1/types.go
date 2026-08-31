package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SharedSecretSpec is the desired state.
// +kubebuilder:validation:XValidation:rule="has(self.targetNamespaces) || has(self.namespaceSelector)",message="at least one of targetNamespaces or namespaceSelector must be set"
type SharedSecretSpec struct {
	// Name of the source Secret in the same namespace as this SharedSecret.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	SourceSecret string `json:"sourceSecret"`

	// Explicit list of target namespaces.
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=63
	// +listType=set
	// +optional
	TargetNamespaces []string `json:"targetNamespaces,omitempty"`

	// Label selector over Namespaces; unioned with targetNamespaces.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
}

// SharedSecretStatus is the observed state.
type SharedSecretStatus struct {

	// +listType=set
	// +optional
	SyncedNamespaces []string `json:"syncedNamespaces,omitempty"`

	// resourceVersion of the source Secret last copied.
	// +optional
	ObservedSourceVersion string `json:"observedSourceVersion,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.spec.sourceSecret`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SharedSecret replicates a Secret into other namespaces.
type SharedSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SharedSecretSpec   `json:"spec"`
	Status SharedSecretStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// SharedSecretList is a list of SharedSecret.
type SharedSecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SharedSecret `json:"items"`
}