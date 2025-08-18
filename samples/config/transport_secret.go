package config

import (
	"context"
	"fmt"
	"os"

	operatorconfig "github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func GetTransportSecret() (*corev1.Secret, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		return nil, fmt.Errorf("failed to get kubeconfig")
	}
	namespace := os.Getenv("SECRET_NAMESPACE")
	if namespace == "" {
		namespace = "multicluster-global-hub"
	}
	name := os.Getenv("SECRET_NAME")
	if name == "" {
		name = "multicluster-global-hub-transport"
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubeclient %v", err)
	}
	secret, err := clientset.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %v", err)
	}
	return secret, nil
}

func GetTransportConfigSecret(namespace, name string) (*corev1.Secret, error) {
	kubeconfig, err := DefaultKubeConfig()
	if err != nil {
		return nil, err
	}
	c, err := client.New(kubeconfig, client.Options{Scheme: operatorconfig.GetRuntimeScheme()})
	if err != nil {
		return nil, err
	}

	transportConfig := &corev1.Secret{}

	err = c.Get(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, transportConfig)
	if err != nil {
		return nil, err
	}
	return transportConfig, nil
}
