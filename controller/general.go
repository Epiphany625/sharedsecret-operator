package controller

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

type GeneralController struct {
	queue workqueue.TypedRateLimitingInterface[string]
	primaryInformer cache.SharedIndexInformer
	supportingInformers []cache.SharedIndexInformer
	informersSynced []cache.InformerSynced

	// custom CRD controller
	customController CustomController
}

type CustomController interface {
	syncHandler(context context.Context, key string) error
}

func NewGeneralController(primaryInformer cache.SharedIndexInformer, 
	supportingInformers []cache.SharedIndexInformer, 
	informersSynced []cache.InformerSynced, 
	cc CustomController) *GeneralController {
	
	// initialize controller
	controller := &GeneralController{
		queue: workqueue.NewTypedRateLimitingQueue(
			workqueue.DefaultTypedControllerRateLimiter[string](),
		),
		primaryInformer: primaryInformer,
		supportingInformers: supportingInformers,
		informersSynced: informersSynced,
		customController: cc,
	}

	// add event handler to primary informer
	controller.primaryInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: controller.enqueueObject,
			UpdateFunc: func(_, newObj interface{}) {
				controller.enqueueObject(newObj)
			},
			// TODO. add delete func in the future. 
		},
	)

	// add event handler to supporting informers
	for _, informer := range controller.supportingInformers {
		informer.AddEventHandler(
			cache.ResourceEventHandlerFuncs{
				// enqueue the owner of the object
				AddFunc: controller.enqueueSupportingObject,

				UpdateFunc: func(_, newObj interface{}) {
					controller.enqueueSupportingObject(newObj)
				},

				// TODO. add delete func in the future. 
			},
		)
	}
	return controller
}

func (c *GeneralController) enqueueObject(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.queue.Add(key)
}

func (c *GeneralController) enqueueSupportingObject(obj interface{}) {
	objMeta, err := meta.Accessor(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return 
	}
	owner := metav1.GetControllerOf(objMeta)
	if owner == nil {
		return 
	}
	key := owner.Name
	namespace := objMeta.GetNamespace()
	if namespace != "" {
		key = namespace + "/" + key
	}
	c.queue.Add(key)
}


func (c *GeneralController) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Info("waiting for informer caches to sync")
	if !cache.WaitForCacheSync(ctx.Done(), c.informersSynced...) {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Infof("starting %d workers", workers)
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}

	<-ctx.Done()
	klog.Info("shutting down")
	return nil
}

func (c *GeneralController) runWorker(ctx context.Context) {
	for c.processNextItem(ctx) {
	}
}

// processNextItem pops one key, syncs it, and decides whether to requeue.
func (c *GeneralController) processNextItem(ctx context.Context) bool {
	key, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	// Done must be called so the queue knows this key is no longer being
	// processed (and can be re-added if another event arrives).
	defer c.queue.Done(key)

	err := c.customController.syncHandler(ctx, key)
	if err == nil {
		// Success: clear any rate-limit history for this key.
		c.queue.Forget(key)
		return true
	}

	// Failure: requeue with exponential backoff.
	utilruntime.HandleError(fmt.Errorf("syncing %q: %w (requeueing)", key, err))
	c.queue.AddRateLimited(key)
	return true
}