package kessel

import (
	"fmt"
	"math/rand"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kesselresources "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
)

// https://github.com/project-kessel/docs/blob/main/src/content/docs/inventory/kafka-event.md
var _ = Describe("kafka-event: Policy API", Ordered, func() {
	var localResourceId string
	var k8sPolicy *kesselresources.K8SPolicy
	BeforeAll(func() {
		localResourceId = fmt.Sprintf("test-policy-%d", rand.Intn(100000))
		k8sPolicy = getK8SPolicy(localResourceId, "guest")
	})

	It("Create", func() {
		_, err := inventoryClient.GetHttpClient().PolicyServiceClient.CreateK8SPolicy(ctx,
			&kesselresources.CreateK8SPolicyRequest{K8SPolicy: k8sPolicy})
		Expect(err).To(Succeed())

		Eventually(func() error {
			resourceType := "redhat.inventory.resources.k8s_policy.created"

			event, ok := receivedEvents[resourceType]
			if !ok {
				return fmt.Errorf("not recieve event %s:%s", resourceType, localResourceId)
			}
			data := &ResourceData{}
			err := event.DataAs(data)
			if err != nil {
				return fmt.Errorf("failed to decode the event due to: %v", err)
			}

			if data.ReporterData.LocalResourceId != localResourceId {
				return fmt.Errorf("LocalResourceId(%s), want %s, but got %s", resourceType, localResourceId,
					data.ReporterData.LocalResourceId)
			}
			return nil
		}, TIMEOUT, INTERVAL).Should(Succeed())
	})

	It("Update", func() {
		k8sPolicy.ResourceData.Severity = kesselresources.K8SPolicyDetail_HIGH
		_, err := inventoryClient.GetHttpClient().PolicyServiceClient.UpdateK8SPolicy(ctx,
			&kesselresources.UpdateK8SPolicyRequest{K8SPolicy: k8sPolicy})
		Expect(err).To(Succeed())

		Eventually(func() error {
			resourceType := "redhat.inventory.resources.k8s_policy.updated"

			event, ok := receivedEvents[resourceType]
			if !ok {
				return fmt.Errorf("not recieve event %s:%s", resourceType, localResourceId)
			}
			data := &ResourceData{}
			err := event.DataAs(data)
			if err != nil {
				return fmt.Errorf("failed to decode the event due to: %v", err)
			}

			if data.ReporterData.LocalResourceId != localResourceId {
				return fmt.Errorf("LocalResourceId(%s), want %s, but got %s", resourceType, localResourceId,
					data.ReporterData.LocalResourceId)
			}

			if data.ResourceData["severity"] != "HIGH" {
				return fmt.Errorf("PolicySeverity(%s), want %s, but got %s", resourceType, "HIGH",
					data.ResourceData["severity"])
			}

			return nil
		}, TIMEOUT, INTERVAL).Should(Succeed())
	})

	It("Delete", func() {
		_, err := inventoryClient.GetHttpClient().PolicyServiceClient.DeleteK8SPolicy(ctx,
			&kesselresources.DeleteK8SPolicyRequest{ReporterData: k8sPolicy.ReporterData})
		Expect(err).To(Succeed())

		Eventually(func() error {
			resourceType := "redhat.inventory.resources.k8s_policy.deleted"

			event, ok := receivedEvents[resourceType]
			if !ok {
				return fmt.Errorf("not recieve event %s:%s", resourceType, localResourceId)
			}
			data := &ResourceData{}
			err := event.DataAs(data)
			if err != nil {
				return fmt.Errorf("failed to decode the event due to: %v", err)
			}

			if data.ReporterData.LocalResourceId != localResourceId {
				return fmt.Errorf("LocalResourceId(%s), want %s, but got %s", resourceType, localResourceId,
					data.ReporterData.LocalResourceId)
			}
			return nil
		}, TIMEOUT, INTERVAL).Should(Succeed())
	})
})

// https://github.com/project-kessel/inventory-api/blob/main/test/e2e/inventory_http_test.go#L238
func getK8SPolicy(localResourceId string, reportInstanceId string) *kesselresources.K8SPolicy {
	return &kesselresources.K8SPolicy{
		Metadata: &kesselresources.Metadata{
			ResourceType: "k8s_policy",
			// WorkspaceId:  "default",
			// OrgId:        "",
		},
		ResourceData: &kesselresources.K8SPolicyDetail{
			Disabled: true,
			Severity: kesselresources.K8SPolicyDetail_MEDIUM,
		},
		ReporterData: &kesselresources.ReporterData{
			ReporterInstanceId: reportInstanceId,
			ReporterType:       kesselresources.ReporterData_ACM,
			ConsoleHref:        "www.example.com",
			ApiHref:            "www.example.com",
			LocalResourceId:    localResourceId,
			ReporterVersion:    "0.1",
		},
	}
}
