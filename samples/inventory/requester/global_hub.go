package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers/managedcluster"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	transportconfig "github.com/stolostron/multicluster-global-hub/pkg/transport/config"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/requester"
	"github.com/stolostron/multicluster-global-hub/samples/config"
)

// export SECRET_NAMESPACE=multicluster-global-hub
// export SECRET_NAME=transport-config-inventory-guest
// ./test/script/event_exporter_inventory.sh
func globalHub(ctx context.Context) error {
	transportConfigSecret, err := config.GetTransportConfigSecret("multicluster-global-hub",
		"transport-config-inventory-guest")
	if err != nil {
		return err
	}

	c, err := getRuntimeClient()
	if err != nil {
		return err
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

	hubinfo := models.ClusterInfo{}
	cluster := createMockCluster("local-cluster", "OpenShift", "4.15.24", "Amazon", "1.23.0")
	k8sCluster := managedcluster.GetK8SCluster(ctx, cluster, "guest", hubinfo)
	createResp, err := requesterClient.GetHttpClient().K8sClusterService.CreateK8SCluster(ctx,
		&kessel.CreateK8SClusterRequest{K8SCluster: k8sCluster})
	if err != nil {
		return err
	}
	fmt.Println("creating response", createResp)

	cluster = createMockCluster("local-cluster", "OpenShift", "4.15.24", "Amazon", "1.23.0")
	k8sCluster = managedcluster.GetK8SCluster(ctx, cluster, "guest", hubinfo)
	updatingResponse, err := requesterClient.GetHttpClient().K8sClusterService.UpdateK8SCluster(ctx,
		&kessel.UpdateK8SClusterRequest{K8SCluster: k8sCluster})
	if err != nil {
		return err
	}
	fmt.Println("updating response", updatingResponse)

	cluster = createMockCluster("local-cluster", "OpenShift", "4.15.24", "Amazon", "1.23.0")
	k8sCluster = managedcluster.GetK8SCluster(ctx, cluster, "guest", hubinfo)
	deletingResponse, err := requesterClient.GetHttpClient().K8sClusterService.DeleteK8SCluster(ctx,
		&kessel.DeleteK8SClusterRequest{ReporterData: k8sCluster.ReporterData})
	if err != nil {
		return err
	}
	fmt.Println("deleting response", deletingResponse)

	return nil
}

func createMockCluster(name, kubeVendor, vendorVersion, platform, kubeVersion string,
) *clusterv1.ManagedCluster {
	return &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: clusterv1.ManagedClusterSpec{
			HubAcceptsClient: true,
		},
		Status: clusterv1.ManagedClusterStatus{
			ClusterClaims: []clusterv1.ManagedClusterClaim{
				{
					Name:  constants.ClusterIdClaimName,
					Value: uuid.New().String(),
				},
				{
					Name:  "platform.open-cluster-management.io",
					Value: platform,
				},
				{
					Name:  "kubeversion.open-cluster-management.io",
					Value: kubeVersion,
				},
				{
					Name:  "version.openshift.io",
					Value: vendorVersion,
				},
				{
					Name:  "product.open-cluster-management.io",
					Value: kubeVendor,
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.ManagedClusterConditionAvailable,
					Status: metav1.ConditionTrue,
					Reason: "ManagedClusterAvailable",
				},
			},
			Capacity: map[clusterv1.ResourceName]resource.Quantity{
				clusterv1.ResourceCPU:    resource.MustParse("16"),
				clusterv1.ResourceMemory: resource.MustParse("64453796Ki"),
			},
		},
	}
}
