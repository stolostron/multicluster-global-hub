package kessel

import (
	"fmt"
	"math/rand"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kesselrelationships "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/relationships"
	kesselresources "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/controllers/inventory/managedclusterinfo"
)

var _ = Describe("relations api", Ordered, func() {
	var k8sCluster *kesselresources.K8SCluster
	var k8sPolicy *kesselresources.K8SPolicy
	var relationship *kesselrelationships.K8SPolicyIsPropagatedToK8SCluster
	var localClusterId string
	var localPolicyId string

	BeforeAll(func() {
		// also is the name of managedclusterinfo
		localClusterId = fmt.Sprintf("test-cluster-%d", rand.Intn(100000))
		clusterInfo := mockManagedClusterInfo(localClusterId, clusterinfov1beta1.KubeVendorOpenShift, "4.10.0",
			clusterinfov1beta1.CloudVendorAWS)
		cluster := createMockCluster(localClusterId, "OpenShift", "4.10.0", "AWS", "1.23.0")
		k8sCluster = managedclusterinfo.GetK8SCluster(ctx, clusterInfo, cluster, "guest", runtimeClient)

		localPolicyId = fmt.Sprintf("test-policy-%d", rand.Intn(100000))
		k8sPolicy = generateK8SPolicy(localPolicyId, "guest")

		relationship = generateK8SPolicyToCluster(
			localPolicyId,
			localClusterId,
			"guest")
	})

	It("Create a cluster", func() {
		_, err := inventoryClient.GetHttpClient().K8sClusterService.CreateK8SCluster(ctx,
			&kesselresources.CreateK8SClusterRequest{K8SCluster: k8sCluster})
		Expect(err).To(Succeed())

		Eventually(func() error {
			clusterType := "redhat.inventory.resources.k8s_cluster.created"
			event, ok := receivedEvents[clusterType]
			if !ok {
				return fmt.Errorf("not recieve event %s:%s", clusterType, localClusterId)
			}
			data := &ResourceData{}
			err := event.DataAs(data)
			if err != nil {
				return fmt.Errorf("failed to decode the event due to: %v", err)
			}

			if data.ReporterData.LocalResourceId != localClusterId {
				return fmt.Errorf("LocalResourceId(%s), want %s, but got %s", clusterType, localClusterId,
					data.ReporterData.LocalResourceId)
			}
			return nil
		}, TIMEOUT, INTERVAL).Should(Succeed())
	})

	It("Update the cluster", func() {
		k8sCluster.ResourceData.ClusterStatus = kesselresources.K8SClusterDetail_FAILED
		_, err := inventoryClient.GetHttpClient().K8sClusterService.UpdateK8SCluster(ctx,
			&kesselresources.UpdateK8SClusterRequest{K8SCluster: k8sCluster})
		Expect(err).To(Succeed())

		Eventually(func() error {
			clusterType := "redhat.inventory.resources.k8s_cluster.updated"

			event, ok := receivedEvents[clusterType]
			if !ok {
				return fmt.Errorf("not recieve event %s:%s", clusterType, localClusterId)
			}
			data := &ResourceData{}
			err := event.DataAs(data)
			if err != nil {
				return fmt.Errorf("failed to decode the event due to: %v", err)
			}

			if data.ReporterData.LocalResourceId != localClusterId {
				return fmt.Errorf("LocalResourceId(%s), want %s, but got %s", clusterType, localClusterId,
					data.ReporterData.LocalResourceId)
			}

			if data.ResourceData["cluster_status"].(float64) != 3 {
				return fmt.Errorf("K8SClusterStatus(%s), want %s, but got %s", clusterType, "FAILED",
					data.ResourceData["cluster_status"])
			}

			return nil
		}, TIMEOUT, INTERVAL).Should(Succeed())
	})

	It("Create a policy", func() {
		_, err := inventoryClient.GetHttpClient().PolicyServiceClient.CreateK8SPolicy(ctx,
			&kesselresources.CreateK8SPolicyRequest{K8SPolicy: k8sPolicy})
		Expect(err).To(Succeed())

		Eventually(func() error {
			resourceType := "redhat.inventory.resources.k8s_policy.created"

			event, ok := receivedEvents[resourceType]
			if !ok {
				return fmt.Errorf("not recieve event %s:%s", resourceType, localPolicyId)
			}
			data := &ResourceData{}
			err := event.DataAs(data)
			if err != nil {
				return fmt.Errorf("failed to decode the event due to: %v", err)
			}

			if data.ReporterData.LocalResourceId != localPolicyId {
				return fmt.Errorf("LocalResourceId(%s), want %s, but got %s", resourceType, localPolicyId,
					data.ReporterData.LocalResourceId)
			}
			return nil
		}, TIMEOUT, INTERVAL).Should(Succeed())
	})

	It("Update the policy", func() {
		k8sPolicy.ResourceData.Severity = kesselresources.K8SPolicyDetail_HIGH
		_, err := inventoryClient.GetHttpClient().PolicyServiceClient.UpdateK8SPolicy(ctx,
			&kesselresources.UpdateK8SPolicyRequest{K8SPolicy: k8sPolicy})
		Expect(err).To(Succeed())

		Eventually(func() error {
			resourceType := "redhat.inventory.resources.k8s_policy.updated"

			event, ok := receivedEvents[resourceType]
			if !ok {
				return fmt.Errorf("not recieve event %s:%s", resourceType, localPolicyId)
			}
			data := &ResourceData{}
			err := event.DataAs(data)
			if err != nil {
				return fmt.Errorf("failed to decode the event due to: %v", err)
			}

			if data.ReporterData.LocalResourceId != localPolicyId {
				return fmt.Errorf("LocalResourceId(%s), want %s, but got %s", resourceType, localPolicyId,
					data.ReporterData.LocalResourceId)
			}

			if data.ResourceData["severity"].(float64) != 4 {
				return fmt.Errorf("PolicySeverity(%s), want %s, but got %s", resourceType, "HIGH",
					data.ResourceData["severity"])
			}

			return nil
		}, TIMEOUT, INTERVAL).Should(Succeed())
	})

	It("Create a relationship", func() {
		_, err := inventoryClient.GetHttpClient().K8SPolicyIsPropagatedToK8SClusterServiceHTTPClient.
			CreateK8SPolicyIsPropagatedToK8SCluster(ctx,
				&kesselrelationships.CreateK8SPolicyIsPropagatedToK8SClusterRequest{
					K8SpolicyIspropagatedtoK8Scluster: relationship,
				})
		Expect(err).To(Succeed())

		Eventually(func() error {
			resourceType := "redhat.inventory.resources-relationship.k8s-policy_is-propagated-to_k8s-cluster.created"

			event, ok := receivedEvents[resourceType]
			if !ok {
				return fmt.Errorf("not recieve event %s:%v", resourceType, relationship)
			}
			data := &RelationshipData{}
			err := event.DataAs(data)
			if err != nil {
				return fmt.Errorf("failed to decode the event due to: %v", err)
			}

			if data.ReporterData.SubjectLocalResourceId != relationship.ReporterData.SubjectLocalResourceId ||
				data.ReporterData.ObjectLocalResourceId != relationship.ReporterData.ObjectLocalResourceId {
				return fmt.Errorf("Relationship want: %v, but got: %v", relationship, data)
			}

			return nil
		}, TIMEOUT, INTERVAL).Should(Succeed())
	})

	It("Update the relatioship", func() {
		relationship.RelationshipData.Status = kesselrelationships.K8SPolicyIsPropagatedToK8SClusterDetail_NO_VIOLATIONS
		_, err := inventoryClient.GetHttpClient().K8SPolicyIsPropagatedToK8SClusterServiceHTTPClient.
			UpdateK8SPolicyIsPropagatedToK8SCluster(ctx,
				&kesselrelationships.UpdateK8SPolicyIsPropagatedToK8SClusterRequest{
					K8SpolicyIspropagatedtoK8Scluster: relationship,
				})
		Expect(err).To(Succeed())

		Eventually(func() error {
			resourceType := "redhat.inventory.resources-relationship.k8s-policy_is-propagated-to_k8s-cluster.updated"

			event, ok := receivedEvents[resourceType]
			if !ok {
				return fmt.Errorf("not recieve event %s:%v", resourceType, relationship)
			}
			data := &RelationshipData{}
			err := event.DataAs(data)
			if err != nil {
				return fmt.Errorf("failed to decode the event due to: %v", err)
			}

			if data.ReporterData.SubjectLocalResourceId != relationship.ReporterData.SubjectLocalResourceId ||
				data.ReporterData.ObjectLocalResourceId != relationship.ReporterData.ObjectLocalResourceId {
				return fmt.Errorf("Relationship want: %v, but got: %v", relationship, data)
			}

			if data.ResourceData["status"].(float64) != 3 {
				return fmt.Errorf("RelationshipStatus want %v, but got %v", relationship.RelationshipData, data.ResourceData)
			}
			return nil
		}, TIMEOUT, INTERVAL).Should(Succeed())
	})

	It("Delete the relationship", func() {
		_, err := inventoryClient.GetHttpClient().K8SPolicyIsPropagatedToK8SClusterServiceHTTPClient.
			DeleteK8SPolicyIsPropagatedToK8SCluster(ctx,
				&kesselrelationships.DeleteK8SPolicyIsPropagatedToK8SClusterRequest{
					ReporterData: relationship.ReporterData,
				})
		Expect(err).To(Succeed())

		Eventually(func() error {
			resourceType := "redhat.inventory.resources-relationship.k8s-policy_is-propagated-to_k8s-cluster.deleted"

			event, ok := receivedEvents[resourceType]
			if !ok {
				return fmt.Errorf("not recieve event %s:%v", resourceType, relationship)
			}
			data := &RelationshipData{}
			err := event.DataAs(data)
			if err != nil {
				return fmt.Errorf("failed to decode the event due to: %v", err)
			}

			// TODO: the report data of the replactionship deletion is tweaked from the defination:
			// https://github.com/project-kessel/docs/blob/main/src/content/docs/inventory/kafka-event.md

			// if data.ReporterData.SubjectLocalResourceId != relationship.ReporterData.SubjectLocalResourceId ||
			// 	data.ReporterData.ObjectLocalResourceId != relationship.ReporterData.ObjectLocalResourceId {
			// 	return fmt.Errorf("Relationship want: %v, but got: %v", relationship, data)
			// }
			return nil
		}, TIMEOUT, INTERVAL).Should(Succeed())
	})

	It("Delete the cluster", func() {
		_, err := inventoryClient.GetHttpClient().K8sClusterService.DeleteK8SCluster(ctx,
			&kesselresources.DeleteK8SClusterRequest{ReporterData: k8sCluster.ReporterData})
		Expect(err).To(Succeed())

		Eventually(func() error {
			clusterType := "redhat.inventory.resources.k8s_cluster.deleted"

			event, ok := receivedEvents[clusterType]
			if !ok {
				return fmt.Errorf("not recieve event %s:%s", clusterType, localClusterId)
			}
			data := &ResourceData{}
			err := event.DataAs(data)
			if err != nil {
				return fmt.Errorf("failed to decode the event due to: %v", err)
			}

			if data.ReporterData.LocalResourceId != localClusterId {
				return fmt.Errorf("LocalResourceId(%s), want %s, but got %s", clusterType, localClusterId,
					data.ReporterData.LocalResourceId)
			}

			return nil
		}, TIMEOUT, INTERVAL).Should(Succeed())
	})

	It("Delete the policy", func() {
		_, err := inventoryClient.GetHttpClient().PolicyServiceClient.DeleteK8SPolicy(ctx,
			&kesselresources.DeleteK8SPolicyRequest{ReporterData: k8sPolicy.ReporterData})
		Expect(err).To(Succeed())

		Eventually(func() error {
			resourceType := "redhat.inventory.resources.k8s_policy.deleted"

			event, ok := receivedEvents[resourceType]
			if !ok {
				return fmt.Errorf("not recieve event %s:%s", resourceType, localPolicyId)
			}
			data := &ResourceData{}
			err := event.DataAs(data)
			if err != nil {
				return fmt.Errorf("failed to decode the event due to: %v", err)
			}

			if data.ReporterData.LocalResourceId != localPolicyId {
				return fmt.Errorf("LocalResourceId(%s), want %s, but got %s", resourceType, localPolicyId,
					data.ReporterData.LocalResourceId)
			}
			return nil
		}, TIMEOUT, INTERVAL).Should(Succeed())
	})
})
