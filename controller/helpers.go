package controller

import (
	"github.com/sharedsecret-operator/pkg/apis/sharedsecret/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// pointer helpper functions
func convertIntToPointer(n int) *int {
	return &n
}
func convertBoolToPointer(n bool) *bool {
	return &n
}

func newSharedSecret(
	name string, 
	namespace string,
	sourceSecretname string, 
	targetNamespaces []string, 
	namespaceSelector *metav1.LabelSelector,
) *v1alpha1.SharedSecret {
	return &v1alpha1.SharedSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Namespace: namespace,
		},
		Spec: v1alpha1.SharedSecretSpec{
			SourceSecret: sourceSecretname,
			TargetNamespaces: targetNamespaces,
			NamespaceSelector: namespaceSelector,
		},
	}
}

// kubernetes object functions
func newSecret(
	name string,
	namespace string,
	data map[string][]byte,
	stringData map[string]string,
	secretType corev1.SecretType,
	labels map[string]string, 
	annotations map[string]string,
	immutable *bool, 
) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: labels, 
			Annotations: annotations,
		},
		Data:       data,
		StringData: stringData,
		Type:       secretType,
		Immutable: immutable,

	}
}

func newNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

// form key based on namespace & name
func formKey(namespace string, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "/" + name
}


// reports whether every key/value pair of subset is already present in existingLabels.
func containsLabels(existingLabels map[string]string, subset map[string]string) bool {
	for key, val := range subset {
		if existingLabels[key] != val {
			return false
		}
	}
	return true
}
