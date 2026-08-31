package controller

import (
	"context"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/sharedsecret-operator/pkg/apis/sharedsecret/v1alpha1"
	sharedsecretclientset "github.com/sharedsecret-operator/pkg/generated/clientset/versioned"
	sharedsecretinformers "github.com/sharedsecret-operator/pkg/generated/informers/externalversions"
	v1alphav1listers "github.com/sharedsecret-operator/pkg/generated/listers/sharedsecret/v1alpha1"
)

const (
	// finalizer const string 
	FINALIZER = "secrets.leo.dev/cleanup-copies"
)

type SharedSecretController struct {
	KubeClient kubernetes.Interface
	SharedSecretClient sharedsecretclientset.Interface
	SharedSecretLister v1alphav1listers.SharedSecretLister
	SecretLister corev1listers.SecretLister
	NamespaceLister corev1listers.NamespaceLister
}

func ownerReferenceLabel(sharedSecretObject *v1alpha1.SharedSecret) map[string]string{
	return map[string]string{
		"secrets.leo.dev/owner": "sharedsecret",
		"secrets.leo.dev/sharedsecretnamespace": sharedSecretObject.Namespace,
		"secrets.leo.dev/sharedsecretname": sharedSecretObject.Name,
	}
}

func NewSharedSecretController(kubeClient kubernetes.Interface, sharedsecretClient sharedsecretclientset.Interface, kubeFactory informers.SharedInformerFactory, sharedsecretFactory sharedsecretinformers.SharedInformerFactory) *GeneralController {

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
		SharedSecretClient: sharedsecretClient,
	}

	informersSynced := []cache.InformerSynced{
			sharedSecretInformer.HasSynced,
			secretsInformer.HasSynced,
			namespaceInformer.HasSynced,
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
		return nil 
	} else if err != nil {
		return err 
	}
	
	sharedSecretObject := cachedSharedSecretObject.DeepCopy()
	if cachedSharedSecretObject.DeletionTimestamp != nil {
		// go to finalizer to cleanup. 
		return ss.handleDeleteObject(ctx, ssNamespace, ssName, sharedSecretObject)
	}
	return ss.handleObject(ctx, ssNamespace, ssName, sharedSecretObject)

}

func (ss SharedSecretController) handleObject(
	ctx context.Context,
	ssNamespace string, 
	ssName string,
	sharedSecretObject *v1alpha1.SharedSecret,
) error {

	// before doing any work, add & confirm the finalizer. 
	if !hasFinalizer(sharedSecretObject, FINALIZER) {
		klog.Info("No finalizer found for sharedsecret, adding the finalizer")
		sharedSecretObject.Finalizers = append(sharedSecretObject.Finalizers, FINALIZER)
		var err error
		// update the object and assign back to sharedSecretObject, so that this object is always kept the newest version.
		// preventing failure in the first reconcile loop. 
		sharedSecretObject, err = ss.SharedSecretClient.AppsV1alpha1().SharedSecrets(ssNamespace).Update(ctx, sharedSecretObject, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}


	// find the secret object we are copying
	secretObject, err := ss.SecretLister.Secrets(sharedSecretObject.Namespace).Get(sharedSecretObject.Spec.SourceSecret)
	if err != nil {
		return err
	}
	secretObject = secretObject.DeepCopy()
	
	// get all targeted namespaces
	targetnamespaces, err := ss.getTargetNamespaces(sharedSecretObject)
	if err != nil {
		return err
	}
	klog.Infof("Matched namespaces: %s", targetnamespaces)

	// create or update secret.
	normalizedLabel := ownerReferenceLabel(sharedSecretObject)

	for _, ns := range targetnamespaces {
		existingSecret, err := ss.KubeClient.CoreV1().Secrets(ns.Name).Get(ctx, secretObject.Name, metav1.GetOptions{})
		newSecretObject := newSecret(secretObject.Name, ns.Name, secretObject.Data, secretObject.StringData, secretObject.Type, normalizedLabel, nil, secretObject.Immutable)
		
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

			_, updateErr := ss.KubeClient.CoreV1().Secrets(ns.Name).Update(ctx, existingSecret, metav1.UpdateOptions{})
			if updateErr != nil {
				return updateErr
			}
		}
	}
	return nil
}

func hasFinalizer(sharedSecretObject *v1alpha1.SharedSecret, finalizer string) bool {
	for _, f := range sharedSecretObject.GetFinalizers() {
		if f == finalizer {
			return true
		}
	}
	return false
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
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		targetNamespaces = append(targetNamespaces, matchedNamespace)
	}
	return targetNamespaces, nil
}

func (ss SharedSecretController) handleDeleteObject(ctx context.Context, 
	ssNameSpace string, 
	ssName string, 
	sharedSecretObject *v1alpha1.SharedSecret) error {
	
	// step 1. if this object does not have the finalizer we want, 
	// it means it is already deleted by us, but was potentially 
	// held by other finalizers that prevented it from deletion
	if !hasFinalizer(sharedSecretObject, FINALIZER) {
		return nil
	}

	// step 2. find and delete all secrets spawned by this shared secret. 
	// TODO. in the future, perhaps building an index is better than looping around. 
	selector := labels.SelectorFromSet(ownerReferenceLabel(sharedSecretObject))
	// note here: do not specify Secret(ns.Name) so that it searches matching secrets from all namespaces, 
	// not only the namespaces specified by current sharedsecret spec. 
	matchingSecrets, err := ss.SecretLister.List(selector)
	if err != nil {
		return err
	} 
	// delete all the matching secrets. 
	for _, secret := range matchingSecrets {
		err := ss.KubeClient.CoreV1().Secrets(secret.Namespace).Delete(ctx, secret.Name, metav1.DeleteOptions{})
		if (!errors.IsNotFound(err)) && err != nil {
			return err
		}
	}

	// step 3. remove the finalizers. 
	sharedSecretObject.Finalizers = removeStringFromList(sharedSecretObject.Finalizers, FINALIZER)
	_, err = ss.SharedSecretClient.AppsV1alpha1().SharedSecrets(sharedSecretObject.Namespace).Update(ctx, sharedSecretObject, metav1.UpdateOptions{})
	if err != nil {
		return err 
	}
	return nil
}

func removeStringFromList(finalizers []string, toRemove string) []string {
	var res []string
	for _, f := range finalizers {
		if f != toRemove {
			res = append(res, f)
		}
	}
	return res 
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