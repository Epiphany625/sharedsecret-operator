package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sharedsecret-operator/controller"
	sharedsecretinformers "github.com/sharedsecret-operator/pkg/generated/informers/externalversions"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog/v2"

	clientset "github.com/sharedsecret-operator/pkg/generated/clientset/versioned"
)


func main() {
	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		flag.StringVar(&kubeconfig, "kubeconfig", filepath.Join(home, ".kube", "config"), "path to kubeconfig (used when not in-cluster)")
	} else {
		flag.StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig")
	}
	workers := flag.Int("workers", 2, "number of concurrent workers")
	klog.InitFlags(nil)
	flag.Parse()

	// In-cluster config when running as a Pod, else kubeconfig.
	cfg, err := rest.InClusterConfig()
	if err != nil {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			klog.Fatalf("building config: %v", err)
		}
	}

	resync := 10 * time.Minute
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("building kube client: %v", err)
	}
	sharedsecretClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("building shared secret client: %v", err)
	}
	kubeFactory := informers.NewSharedInformerFactory(kubeClient, resync)
	sharedsecretFactory := sharedsecretinformers.NewSharedInformerFactory(sharedsecretClient, resync)

	sharedsecretController := controller.NewSharedSecretController(kubeClient, kubeFactory, sharedsecretFactory)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start() only starts informers that were requested above (via the
	// factory accessor calls inside controller.New).
	kubeFactory.Start(ctx.Done())
	sharedsecretFactory.Start(ctx.Done())

	if err := sharedsecretController.Run(ctx, *workers); err != nil {
		klog.Error(err)
		os.Exit(1)
	}
	
}