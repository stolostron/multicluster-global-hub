package main

import (
	"context"
	"fmt"

	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers/managedcluster"
	transportconfig "github.com/stolostron/multicluster-global-hub/pkg/transport/config"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/requester"
	"github.com/stolostron/multicluster-global-hub/samples/config"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func managedHub(ctx context.Context, leafHubName string) error {
	clusterInfoList, err := listClusterInfo()
	if err != nil {
		return err
	}

	transportConfigSecret, err := config.GetTransportConfigSecret("multicluster-global-hub-agent", "transport-config")
	if err != nil {
		return err
	}

	c, err := getRuntimeClient()
	if err != nil {
		panic(err)
	}
	restfulConn, err := transportconfig.GetRestfulConnBySecret(transportConfigSecret, c)
	if err != nil {
		return err
	}
	// utils.PrettyPrint(restfulConn)

	requesterClient, err := requester.NewInventoryClient(ctx, restfulConn)
	if err != nil {
		return err
	}

	cluster := &clusterv1.ManagedCluster{}
	if err := c.Get(ctx, runtimeclient.ObjectKey{Name: clusterInfoList[0].GetName()}, cluster); err != nil {
		return err
	}

	k8sCluster := managedcluster.GetK8SCluster(ctx, cluster,
		leafHubName, c)

	resp, err := requesterClient.GetHttpClient().K8sClusterService.CreateK8SCluster(ctx,
		&kessel.CreateK8SClusterRequest{K8SCluster: k8sCluster},
	)
	if err != nil {
		return err
	}
	fmt.Println("response", resp)
	return nil
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

func getRuntimeClient() (runtimeclient.Client, error) {
	kubeconfig, err := config.DefaultKubeConfig()
	if err != nil {
		return nil, err
	}
	c, err := runtimeclient.New(kubeconfig, runtimeclient.Options{Scheme: configs.GetRuntimeScheme()})
	if err != nil {
		return nil, err
	}
	return c, nil
}
