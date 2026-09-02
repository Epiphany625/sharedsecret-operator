package controller

import (
	"slices"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHappyPath(t *testing.T) {

	testNamespace := "test-namespace-1"
	testSecretName := "test-name-1"
	testSharedSecretName := "test-sharedsecret-1"
	testSharedSecretTargetNamespace := "test-sharedsecret-targetnamespace-1"
	testSecretKey := "test-secret-key"

	// create namespaces
	testNamespaceObject := newNamespace(testNamespace)
	testTargetNamespaceObject := newNamespace(testSharedSecretTargetNamespace)
	_, err := kubeClient.CoreV1().Namespaces().Create(t.Context(), testNamespaceObject, metav1.CreateOptions{})
	assert.NoError(t, err)
	_, err = kubeClient.CoreV1().Namespaces().Create(t.Context(), testTargetNamespaceObject, metav1.CreateOptions{})
	assert.NoError(t, err)

	// create a secret
	testSecretObject := newSecret(testSecretName, testNamespace, nil, map[string]string{
		testSecretKey: "test-secret-val",
	}, corev1.SecretTypeOpaque, nil, nil, convertBoolToPointer(false))
	_, err = kubeClient.CoreV1().Secrets(testNamespace).Create(t.Context(), testSecretObject, metav1.CreateOptions{})
	assert.NoError(t, err)
	// create a sharedsecret that includes this namespace.
	testSharedSecretObject := newSharedSecret(testSharedSecretName, testNamespace, testSecretName, []string{testSharedSecretTargetNamespace}, nil)
	_, err = sharedsecretClient.AppsV1alpha1().SharedSecrets(testNamespace).Create(t.Context(), testSharedSecretObject, metav1.CreateOptions{})
	assert.NoError(t, err)

	// check finalizers exist in the shared secret object
	time.Sleep(2 * time.Second)
	updatedSharedSecretObject, err := sharedsecretClient.AppsV1alpha1().SharedSecrets(testNamespace).Get(t.Context(), testSharedSecretName, metav1.GetOptions{})
	assert.True(t, slices.Contains(updatedSharedSecretObject.Finalizers, FINALIZER))


	createdSecret, err := kubeClient.CoreV1().Secrets(testSharedSecretTargetNamespace).Get(t.Context(), testSecretName, metav1.GetOptions{})
	assert.NoError(t, err)
	// the data exists
	_, ok := createdSecret.Data[testSecretKey]
	assert.True(t, ok)

	// confirm labels exist and are correct.
	ownerKind, ok := createdSecret.Labels[LABEL_OWNERKIND]
	assert.True(t, ok)
	ownerNamespace, ok := createdSecret.Labels[LABEL_OWNERNAMESPACE]
	assert.True(t, ok)
	ownerName, ok := createdSecret.Labels[LABEL_OWNERNAME]
	assert.True(t, ok)
	assert.Equal(t, SHARED_SECRET_CRD, ownerKind)
	assert.Equal(t, testNamespace, ownerNamespace)
	assert.Equal(t, testSharedSecretName, ownerName)

	// delete the shared secret object.
	err = sharedsecretClient.AppsV1alpha1().SharedSecrets(testNamespace).Delete(t.Context(), testSharedSecretName, metav1.DeleteOptions{})
	assert.NoError(t, err)
	time.Sleep(2 * time.Second)

	_, err = kubeClient.CoreV1().Secrets(testSharedSecretTargetNamespace).Get(t.Context(), testSecretName, metav1.GetOptions{})
	assert.True(t, errors.IsNotFound(err))
}
