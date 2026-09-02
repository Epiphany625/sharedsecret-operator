package controller

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	sharedsecretclientset "github.com/sharedsecret-operator/pkg/generated/clientset/versioned"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	sharedsecretinformers "github.com/sharedsecret-operator/pkg/generated/informers/externalversions"
)

// global variables used for other unit & integration tests.
var (
	sharedsecretClient *sharedsecretclientset.Clientset
	kubeClient         *kubernetes.Clientset
)

func TestMain(m *testing.M) {
	const (
		RESYNC = 1 * time.Hour
	)
	var (
		err error
	)
	ctx, cancel := context.WithCancel(context.Background())

	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "config", "crd"),
		},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	if err != nil {
		klog.Errorf("error setting up test environment: %v", err)
		os.Exit(1)
	}

	sharedsecretClient, err = sharedsecretclientset.NewForConfig(cfg)
	if err != nil {
		klog.Errorf("error setting up shared secret client: %v", err)
		_ = testEnv.Stop()
		os.Exit(1)
	}
	kubeClient, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Errorf("error setting up Kubernetes client: %v", err)
		_ = testEnv.Stop()
		os.Exit(1)
	}

	sharedSecretFactory := sharedsecretinformers.NewSharedInformerFactory(sharedsecretClient, RESYNC)
	kubeFactory := informers.NewSharedInformerFactory(kubeClient, RESYNC)
	sharedsecretController := NewSharedSecretController(kubeClient, sharedsecretClient, kubeFactory, sharedSecretFactory)

	sharedSecretFactory.Start(ctx.Done())
	kubeFactory.Start(ctx.Done())


	if sharedsecretClient == nil || kubeClient == nil {
		klog.Error("at least one of the clients is nil")
		cancel()
		_ = testEnv.Stop()
		os.Exit(1)
	}

	controllerErr := make(chan error, 1)
	go func() {
		controllerErr <- sharedsecretController.Run(ctx, 2)
	}()

	code := m.Run()

	cancel()
	if err := <-controllerErr; err != nil {
		klog.Errorf("controller stopped with error: %v", err)
		code = 1
	}

	if err := testEnv.Stop(); err != nil {
		klog.Errorf("failed to stop test environment: %v", err)
		code = 1
	}

	os.Exit(code)
}
