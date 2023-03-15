package utils

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	runClient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Client interface {
	KubeClient() kubernetes.Interface
	KubeDynamicClient() dynamic.Interface
	ControllerRuntimeClient(clusterName string, scheme *runtime.Scheme) (runClient.Client, error)
	Kubectl(clusterName string, args ...string) (string, error)
	RestConfig(clusterName string) (*rest.Config, error)
	HubClusterName() string
	LeafHubClusterName() string
}

type client struct {
	options Options
}

func NewTestClient(opt Options) *client {
	return &client{
		options: opt,
	}
}

func (c *client) ControllerRuntimeClient(clusterName string, scheme *runtime.Scheme) (runClient.Client, error) {
	cfg, err := c.RestConfig(clusterName)
	if err != nil {
		return nil, err
	}
	controllerClient, err := runClient.New(cfg, runClient.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	return controllerClient, nil
}

func (c *client) KubeClient() kubernetes.Interface {
	opt := c.options
	config, err := LoadConfig(opt.HubCluster.KubeConfig, opt.HubCluster.KubeConfig, opt.HubCluster.KubeContext)
	if err != nil {
		panic(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	return clientset
}

func (c *client) KubeDynamicClient() dynamic.Interface {
	opt := c.options
	url := ""
	kubeConfig := opt.HubCluster.KubeConfig
	kubeContext := opt.HubCluster.KubeContext
	config, err := LoadConfig(url, kubeConfig, kubeContext)
	if err != nil {
		panic(err)
	}
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	return client
}

func (c *client) Kubectl(clusterName string, args ...string) (string, error) {
	if c.options.HubCluster.Name == clusterName {
		// insert to the first
		args = append([]string{"--context", c.options.HubCluster.KubeContext}, args...)
		args = append([]string{"--kubeconfig", c.options.HubCluster.KubeConfig}, args...)
		output, err := exec.Command("kubectl", args...).CombinedOutput()
		return string(output), err
	}
	for _, cluster := range c.options.ManagedClusters {
		if cluster.Name == clusterName {
			args = append([]string{"--context", cluster.KubeContext}, args...)
			args = append([]string{"--kubeconfig", cluster.KubeConfig}, args...)
			output, err := exec.Command("kubectl", args...).CombinedOutput()
			return string(output), err
		}
	}
	return "", fmt.Errorf("cluster %s is not found in options", clusterName)
}

func (c *client) RestConfig(clusterName string) (*rest.Config, error) {
	if c.options.HubCluster.Name == clusterName {
		return LoadConfig(c.options.HubCluster.ApiServer, c.options.HubCluster.KubeConfig, c.options.HubCluster.KubeContext)
	}
	for _, cluster := range c.options.ManagedClusters {
		if cluster.Name == clusterName {
			return LoadConfig("", cluster.KubeConfig, cluster.KubeContext)
		}
	}
	return nil, fmt.Errorf("cluster %s is not found in options", clusterName)
}

func LoadConfig(url, kubeconfig, context string) (*rest.Config, error) {
	if kubeconfig == "" {
		kubeconfig = os.Getenv("KUBECONFIG")
	}
	// If we have an explicit indication of where the kubernetes config lives, read that.
	if kubeconfig != "" {
		if context == "" {
			klog.V(6).Infof("clientcmd.BuildConfigFromFlags with %s and %s", url, kubeconfig)
			return clientcmd.BuildConfigFromFlags(url, kubeconfig)
		} else {
			return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
				&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig},
				&clientcmd.ConfigOverrides{
					CurrentContext: context,
				}).ClientConfig()
		}
	}
	// If not, try the in-cluster config.
	if c, err := rest.InClusterConfig(); err == nil {
		return c, nil
	}
	// If no in-cluster config, try the default location in the user's home directory.
	if usr, err := user.Current(); err == nil {
		klog.V(5).Infof("clientcmd.BuildConfigFromFlags for url %s using %s\n", url,
			filepath.Join(usr.HomeDir, ".kube", "config"))
		if c, err := clientcmd.BuildConfigFromFlags(url,
			filepath.Join(usr.HomeDir, ".kube", "config")); err == nil {
			return c, nil
		}
	}

	return nil, fmt.Errorf("could not create a valid kubeconfig")
}

func (c *client) HubClusterName() string {
	return c.options.HubCluster.Name
}

func (c *client) LeafHubClusterName() string {
	for _, cluster := range c.options.ManagedClusters {
		if cluster.Name == cluster.LeafHubName {
			return cluster.Name
		}
	}
	return ""
}
