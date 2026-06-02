package k8s

import (
	"os"
	"path/filepath"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// NewClients creates both a typed clientset and a dynamic client. Dynamic
// client is required to talk to the MPIJob CRD (kubeflow.org/v2beta1)
// without pulling kubeflow types as a hard dependency.
func NewClients() (*kubernetes.Clientset, dynamic.Interface, error) {
	config, err := restConfig()
	if err != nil {
		return nil, nil, err
	}
	typed, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}
	dyn, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}
	return typed, dyn, nil
}

func restConfig() (*rest.Config, error) {
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}
