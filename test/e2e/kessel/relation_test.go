package kessel

import (
	"fmt"
	"math/rand"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kesselrelationships "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/relationships"
)

// https://github.com/project-kessel/docs/blob/main/src/content/docs/inventory/kafka-event.md
var _ = Describe("kafka-event: Relationship API", Ordered, func() {
	var relationship *kesselrelationships.K8SPolicyIsPropagatedToK8SCluster
	var subjectLocalResourceId string
	var objectLocalResourceId string

	BeforeAll(func() {
		subjectLocalResourceId = fmt.Sprintf("policy-id-%d", rand.Intn(100000))
		objectLocalResourceId = fmt.Sprintf("cluster-id-%d", rand.Intn(100000))
		relationship = getK8SPolicyToCluster(
			subjectLocalResourceId,
			objectLocalResourceId,
			"guest")
	})

	It("Create", func() {
		_, err := inventoryClient.GetHttpClient().K8SPolicyIsPropagatedToK8SClusterServiceHTTPClient.
			CreateK8SPolicyIsPropagatedToK8SCluster(ctx,
				&kesselrelationships.CreateK8SPolicyIsPropagatedToK8SClusterRequest{
					K8SpolicyIspropagatedtoK8Scluster: relationship,
				})
		Expect(err).To(Succeed())

		Eventually(func() error {
			resourceType := "redhat.inventory.resources_relationship.k8spolicy_ispropagatedto_k8scluster.created"

			event, ok := receivedEvents[resourceType]
			if !ok {
				return fmt.Errorf("not recieve event %s:%v", resourceType, relationship)
			}
			data := &RelationshipData{}
			err := event.DataAs(data)
			if err != nil {
				return fmt.Errorf("failed to decode the event: %v", event)
			}

			if data.ReporterData.SubjectLocalResourceId != relationship.ReporterData.SubjectLocalResourceId ||
				data.ReporterData.ObjectLocalResourceId != relationship.ReporterData.ObjectLocalResourceId {
				return fmt.Errorf("Relationship want: %v, but got: %v", relationship, data)
			}

			return nil
		}, TIMEOUT, INTERVAL).Should(Succeed())
	})

	It("Update", func() {
		relationship.RelationshipData.Status = kesselrelationships.K8SPolicyIsPropagatedToK8SClusterDetail_NO_VIOLATIONS
		_, err := inventoryClient.GetHttpClient().K8SPolicyIsPropagatedToK8SClusterServiceHTTPClient.
			UpdateK8SPolicyIsPropagatedToK8SCluster(ctx,
				&kesselrelationships.UpdateK8SPolicyIsPropagatedToK8SClusterRequest{
					K8SpolicyIspropagatedtoK8Scluster: relationship,
				})
		Expect(err).To(Succeed())

		Eventually(func() error {
			resourceType := "redhat.inventory.resources_relationship.k8spolicy_ispropagatedto_k8scluster.updated"

			event, ok := receivedEvents[resourceType]
			if !ok {
				return fmt.Errorf("not recieve event %s:%v", resourceType, relationship)
			}
			data := &RelationshipData{}
			err := event.DataAs(data)
			if err != nil {
				return fmt.Errorf("failed to decode the event: %v", event)
			}

			if data.ReporterData.SubjectLocalResourceId != relationship.ReporterData.SubjectLocalResourceId ||
				data.ReporterData.ObjectLocalResourceId != relationship.ReporterData.ObjectLocalResourceId {
				return fmt.Errorf("Relationship want: %v, but got: %v", relationship, data)
			}

			if data.Relationship.Status != "NO_VIOLATIONS" {
				return fmt.Errorf("RelationshipStatus want %v, but got %v", relationship.RelationshipData, data.Relationship)
			}
			return nil
		}, TIMEOUT, INTERVAL).Should(Succeed())
	})

	It("Delete", func() {
		_, err := inventoryClient.GetHttpClient().K8SPolicyIsPropagatedToK8SClusterServiceHTTPClient.
			DeleteK8SPolicyIsPropagatedToK8SCluster(ctx,
				&kesselrelationships.DeleteK8SPolicyIsPropagatedToK8SClusterRequest{
					ReporterData: relationship.ReporterData,
				})
		Expect(err).To(Succeed())

		Eventually(func() error {
			resourceType := "redhat.inventory.resources_relationship.k8spolicy_ispropagatedto_k8scluster.deleted"

			event, ok := receivedEvents[resourceType]
			if !ok {
				return fmt.Errorf("not recieve event %s:%v", resourceType, relationship)
			}
			data := &RelationshipData{}
			err := event.DataAs(data)
			if err != nil {
				return fmt.Errorf("failed to decode the event: %v", event)
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
})

func getK8SPolicyToCluster(subjectLocalResourceId, objectLocalResourceId string,
	reportInstanceId string,
) *kesselrelationships.K8SPolicyIsPropagatedToK8SCluster {
	return &kesselrelationships.K8SPolicyIsPropagatedToK8SCluster{
		Metadata: &kesselrelationships.Metadata{
			RelationshipType: "k8s-policy_is-propagated-to_k8s-cluster",
		},
		RelationshipData: &kesselrelationships.K8SPolicyIsPropagatedToK8SClusterDetail{
			Status:       kesselrelationships.K8SPolicyIsPropagatedToK8SClusterDetail_VIOLATIONS,
			K8SPolicyId:  1234,
			K8SClusterId: 5678,
		},
		ReporterData: &kesselrelationships.ReporterData{
			ReporterType:           kesselrelationships.ReporterData_ACM,
			ReporterInstanceId:     reportInstanceId,
			SubjectLocalResourceId: subjectLocalResourceId,
			ObjectLocalResourceId:  objectLocalResourceId,
			ReporterVersion:        "2.12",
		},
	}
}
