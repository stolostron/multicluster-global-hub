package main

import (
	"context"
	"log"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"
	agentconfig "github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	transportconfig "github.com/stolostron/multicluster-global-hub/pkg/transport/config"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/inventory/client"
	"github.com/stolostron/multicluster-global-hub/samples/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func main() {
	clusterInfoList, err := listClusterInfo()
	if err != nil {
		log.Fatalf("failed to get cluster info list: %v", err)
	}

	transportConfigSecret, err := config.GetTransportConfigSecret("open-cluster-management", "transport-config")
	if err != nil {
		log.Fatalf("failed to get transport config secret: %v", err)
	}

	c, err := getRuntimeClient()
	if err != nil {
		panic(err)
	}
	restfulConn, err := transportconfig.GetRestfulConnBySecret(transportConfigSecret, c)
	if err != nil {
		log.Fatalf("failed to extract rest credentail: %v", err)
	}
	// utils.PrettyPrint(restfulConn)

	inventoryClient, err := client.NewInventoryClient(context.Background(), restfulConn)
	if err != nil {
		log.Fatalf("failed to init the inventory client: %v", err)
	}

	evt := cloudevents.NewEvent()
	evt.SetType(string(enum.ManagedClusterInfoType))
	evt.SetSource("test")
	evt.SetData(cloudevents.ApplicationJSON, clusterInfoList)

	err = inventoryClient.Request(context.Background(), evt)
	if err != nil {
		log.Fatalf("failed to send the request: %v", err)
	}
}

func listClusterInfo() ([]clusterinfov1beta1.ManagedClusterInfo, error) {
	c, err := getRuntimeClient()
	if err != nil {
		return nil, err
	}
	clusterInfoList := clusterinfov1beta1.ManagedClusterInfoList{}
	err = c.List(context.Background(), &clusterInfoList)
	if err != nil {
		return nil, err
	}
	return clusterInfoList.Items, nil
}

func getTransportConfigSecret(namespace, name string) (*corev1.Secret, error) {
	c, err := getRuntimeClient()
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

func getRuntimeClient() (runtimeclient.Client, error) {
	kubeconfig, err := config.DefaultKubeConfig()
	if err != nil {
		return nil, err
	}
	c, err := runtimeclient.New(kubeconfig, runtimeclient.Options{Scheme: agentconfig.GetRuntimeScheme()})
	if err != nil {
		return nil, err
	}
	return c, nil
}
