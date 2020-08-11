package clusterinstallation

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// GenerateK8sClient create client for kubernetes
func GenerateK8sClient() (*kubernetes.Clientset, *rest.Config) {
	config, _ := rest.InClusterConfig()
	clientset, _ := kubernetes.NewForConfig(config)
	return clientset, config
}
