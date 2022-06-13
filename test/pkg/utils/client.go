package utils

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v4/pgxpool"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

type Client interface {
	KubeClient() kubernetes.Interface
	KubeDynamicClient() dynamic.Interface
	Kubectl(clusterName string, args ...string) (string, error)
	RestConfig(clusterName string) (*rest.Config, error)
}

type client struct {
	options Options
}

func NewTestClient(opt Options) *client {
	return &client{
		options: opt,
	}
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
	if HUB_OF_HUB_CLUSTER_NAME == clusterName {
		args = append(args, "--kubeconfig", c.options.HubCluster.KubeConfig)
		args = append(args, "--context", c.options.HubCluster.KubeContext)
		output, err := exec.Command("kubectl", args...).CombinedOutput()
		return string(output), err
	}
	for _, cluster := range c.options.ManagedClusters {
		if cluster.Name == clusterName {
			args = append(args, "--kubeconfig", cluster.KubeConfig)
			args = append(args, "--context", cluster.KubeContext)
			output, err := exec.Command("kubectl", args...).CombinedOutput()
			return string(output), err
		}
	}
	return "", fmt.Errorf("cluster %s is not found in options", clusterName)
}

func (c *client) GetPgPool() *pgxpool.Pool {
	clientSet := c.KubeClient()
	secretClient := clientSet.CoreV1().Secrets(c.options.HubCluster.Namespace)
	secret, err := secretClient.Get(context.TODO(), c.options.HubCluster.DatabaseSecret, metaV1.GetOptions{})
	if err != nil {
		panic(err)
	}

	originUrl := string(secret.Data["url"])
	klog.V(5).Info("original url: ", originUrl)
	reg := regexp.MustCompile(`@(.*?):(\d*?)/`)
	matched := reg.FindStringSubmatch(originUrl)
	if err != nil || len(matched) < 2 {
		panic(err)
	}
	pgService := matched[1]
	pgPort := matched[2]
	klog.V(5).Info("pg service: ", pgService, " pg port: ", pgPort)

	dynamicClientSet := c.KubeDynamicClient()
	pgbounder, err := dynamicClientSet.Resource(NewRouteGVR()).Namespace("hoh-postgres").Get(context.TODO(), "hoh-pgbouncer", metaV1.GetOptions{})
	if err != nil {
		panic(err)
	}
	routeHost := pgbounder.Object["spec"].(map[string]interface{})["host"].(string)
	klog.V(5).Infof("route host %s", routeHost)
	newUrl := strings.Replace(originUrl, pgService, routeHost, 1)
	klog.V(5).Infof("new url %s", newUrl)

	// connect to db
	pool, err := pgxpool.Connect(context.Background(), newUrl)
	if err != nil {
		panic(err)
	}
	return pool
}

func (c *client) RestConfig(clusterName string) (*rest.Config, error) {
	if c.options.HubCluster.Name == clusterName {
		return LoadConfig(c.options.HubCluster.MasterURL, c.options.HubCluster.KubeConfig, c.options.HubCluster.KubeContext)
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
		klog.V(5).Infof("clientcmd.BuildConfigFromFlags for url %s using %s\n", url, filepath.Join(usr.HomeDir, ".kube", "config"))
		if c, err := clientcmd.BuildConfigFromFlags(url, filepath.Join(usr.HomeDir, ".kube", "config")); err == nil {
			return c, nil
		}
	}

	return nil, fmt.Errorf("could not create a valid kubeconfig")
}
