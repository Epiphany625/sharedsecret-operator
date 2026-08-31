package controller

import (
	"context"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/sharedsecret-operator/pkg/apis/sharedsecret/v1alpha1"
	sharedsecretinformers "github.com/sharedsecret-operator/pkg/generated/informers/externalversions"
	v1alphav1listers "github.com/sharedsecret-operator/pkg/generated/listers/sharedsecret/v1alpha1"
)

type SharedSecretController struct {
	KubeClient kubernetes.Interface
	SharedSecretLister v1alphav1listers.SharedSecretLister
	SecretLister corev1listers.SecretLister
	NamespaceLister corev1listers.NamespaceLister
}

func NewSharedSecretController(kubeClient kubernetes.Interface, kubeFactory informers.SharedInformerFactory, sharedsecretFactory sharedsecretinformers.SharedInformerFactory) *GeneralController {

	sharedSecretInformer := sharedsecretFactory.Apps().V1alpha1().SharedSecrets().Informer()
	secretsInformer := kubeFactory.Core().V1().Secrets().Informer()
	namespaceInformer := kubeFactory.Core().V1().Namespaces().Informer()

	sharedSecretLister := sharedsecretFactory.Apps().V1alpha1().SharedSecrets().Lister()
	secretLister := kubeFactory.Core().V1().Secrets().Lister()
	namespaceLister := kubeFactory.Core().V1().Namespaces().Lister()


	supportingInformers := []cache.SharedIndexInformer{
		secretsInformer, namespaceInformer,
	}

	ssController := SharedSecretController{
		SharedSecretLister: sharedSecretLister,
		SecretLister: secretLister,
		NamespaceLister: namespaceLister,
		KubeClient: kubeClient,
	}

	informersSynced := []cache.InformerSynced{
			sharedSecretInformer.HasSynced,
			secretsInformer.HasSynced,
		}
	
	generalController := NewGeneralController(sharedSecretInformer, supportingInformers, informersSynced, ssController)

	return generalController
}

func (ss SharedSecretController) syncHandler(ctx context.Context, key string) error {
	ssNamespace, ssName, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid key %q", key))
		return nil // don't retry; the key can't get better
	}
	cachedSharedSecretObject, err := ss.SharedSecretLister.SharedSecrets(ssNamespace).Get(ssName)
	
	// object was deleted. 
	if errors.IsNotFound(err) {
		return ss.handleDeleteObject(ctx, key)
	}
	if err != nil {
		return err 
	}
	sharedSecretObject := cachedSharedSecretObject.DeepCopy()
	return ss.handleObject(ctx, ssNamespace, ssName, sharedSecretObject)

}

func (ss SharedSecretController) handleObject(
	ctx context.Context,
	ssNamespace string, 
	ssName string,
	sharedSecretObject interface{},
) error {

	obj, ok := sharedSecretObject.(*v1alpha1.SharedSecret)
	if !ok {
		return fmt.Errorf(
			"expected *v1alphav1.SharedSecret, got %T",
			sharedSecretObject,
		)
	}

	// find the secret object we are copying
	secretObject, err := ss.SecretLister.Secrets(obj.Namespace).Get(obj.Spec.SourceSecret)
	if err != nil {
		return err
	}
	secretObject = secretObject.DeepCopy()
	
	// get all targeted namespaces
	targetnamespaces, err := ss.getTargetNamespaces(obj)
	if err != nil {
		return err
	}
	klog.Infof("Matched namespaces: %s", targetnamespaces)

	// create or update secret.
	normalizedLabel := map[string]string{
		"secrets.leo.dev/owner": "sharedsecret",
	}
	normalizedAnnotations := map[string]string{
		"secrets.leo.dev/sharedsecretnamespace": obj.Namespace,
		"secrets.leo.dev/sharedsecretname": obj.Name,
	}

	for _, ns := range targetnamespaces {
		existingSecret, err := ss.KubeClient.CoreV1().Secrets(ns.Name).Get(ctx, secretObject.Name, metav1.GetOptions{})
		newSecretObject := newSecret(secretObject.Name, ns.Name, secretObject.Data, secretObject.StringData, secretObject.Type, normalizedLabel, normalizedAnnotations, secretObject.Immutable)
		
		// create or update
		if errors.IsNotFound(err) {
			_, createErr := ss.KubeClient.CoreV1().Secrets(ns.Name).Create(ctx, newSecretObject, metav1.CreateOptions{})
			if createErr != nil {
				return createErr
			}
		} else if err != nil {
			return err
		} else {
			existingSecret := existingSecret.DeepCopy()
			existingSecret.Data = secretObject.Data
			existingSecret.StringData = secretObject.StringData
			existingSecret.Type = secretObject.Type
			existingSecret.Immutable = secretObject.Immutable

			if existingSecret.Labels == nil {
				existingSecret.Labels = map[string]string{}
			}

			if existingSecret.Annotations == nil {
				existingSecret.Annotations = map[string]string{}
			}

			maps.Copy(existingSecret.Labels, normalizedLabel)
			maps.Copy(existingSecret.Annotations, normalizedAnnotations)

			_, updateErr := ss.KubeClient.CoreV1().Secrets(ns.Name).Update(ctx, existingSecret, metav1.UpdateOptions{})
			if updateErr != nil {
				return updateErr
			}
		}
	}
	return nil
}

func (ss SharedSecretController) getTargetNamespaces(obj *v1alpha1.SharedSecret) (targetNamespaces []*corev1.Namespace, err error) {
	namespaceStringMatch := obj.Spec.TargetNamespaces
	namespaceSelector := obj.Spec.NamespaceSelector

	klog.Infof(
		"target namespaces: %v, namespace selector: %v",
		namespaceStringMatch,
		namespaceSelector,
	)

	selector, err := metav1.LabelSelectorAsSelector(namespaceSelector)
	if err != nil {
		return nil, err
	}
	targetNamespaces, err = ss.NamespaceLister.List(selector)
	if err != nil {
		return nil, err
	}

	for _, namespaceStr := range namespaceStringMatch {
		matchedNamespace, err := ss.NamespaceLister.Get(namespaceStr)
		if err != nil {
			return nil, err
		}
		targetNamespaces = append(targetNamespaces, matchedNamespace)
	}
	return targetNamespaces, nil
}

func (ss SharedSecretController) handleDeleteObject(ctx context.Context, key string) error {
	return nil
}

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