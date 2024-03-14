package dbsyncer_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test ./manager/pkg/statussyncer/syncers -v -ginkgo.focus "LocalPolicyComplianceHandler"
var _ = Describe("LocalPolicyComplianceHandler", Ordered, func() {
	const (
		leafHubName     = "hub1"
		createdPolicyId = "d9347b09-bb46-4e2b-91ea-513e83ab9ea8"
	)
	var complianceVersion *eventversion.Version

	It("should handle the local compliance event", func() {
		By("Add an expired policy to the database")
		db := database.GetGorm()
		expiredPolicyID := "b8b3e164-377e-4be1-a870-992265f31f7c"
		err := db.Create(&models.LocalStatusCompliance{
			PolicyID:    expiredPolicyID,
			ClusterName: "cluster1",
			LeafHubName: leafHubName,
			Compliance:  database.Unknown,
			Error:       "none",
		}).Error
		Expect(err).ToNot(HaveOccurred())

		By("Check the expired policy is added in database")
		Eventually(func() error {
			var localCompliance models.LocalStatusCompliance
			err = db.Where("policy_id = ?", expiredPolicyID).First(&localCompliance).Error
			if err != nil {
				return err
			}

			if localCompliance.ClusterName == "cluster1" &&
				localCompliance.Compliance == database.Unknown {
				return nil
			}
			return fmt.Errorf("failed to persist data to local compliance of table")
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		By("Build a new policy bundle in the managed hub")
		complianceVersion = eventversion.NewVersion()
		complianceVersion.Incr()
		complianceVersion.Incr()

		data := grc.ComplianceData{}
		data = append(data, grc.Compliance{
			PolicyID:                  createdPolicyId,
			CompliantClusters:         []string{"cluster1"},
			NonCompliantClusters:      []string{"cluster2"},
			UnknownComplianceClusters: []string{},
		})

		evt := ToCloudEvent(leafHubName, string(enum.LocalComplianceType), complianceVersion, data)

		By("Sync message with transport")
		err = producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())

		By("Check the local compliance is created and expired policy is deleted from database")
		Eventually(func() error {
			var localCompliances []models.LocalStatusCompliance
			err = db.Where("leaf_hub_name = ?", leafHubName).Find(&localCompliances).Error
			if err != nil {
				return err
			}

			expiredCount := 0
			addedCount := 0
			for _, c := range localCompliances {
				if c.PolicyID == expiredPolicyID && c.ClusterName == "cluster1" {
					expiredCount++
				}
				if c.PolicyID == createdPolicyId && c.ClusterName == "cluster1" || c.ClusterName == "cluster2" {
					addedCount++
				}

				fmt.Printf("LocalCompliance: ID(%s) %s/%s %s \n", c.PolicyID, c.LeafHubName, c.ClusterName, c.Compliance)
			}
			if expiredCount == 0 && addedCount == 2 && len(localCompliances) == 2 {
				return nil
			}
			return fmt.Errorf("failed to sync local compliance")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should handle the local complete compliance event", func() {
		db := database.GetGorm()
		By("Create a complete compliance bundle")
		version := eventversion.NewVersion()
		version.Incr()

		// hub1-cluster1 compliant => hub1-cluster1 non_compliant
		// hub1-cluster2 non_compliant => hub1-cluster2 compliant
		data := grc.CompleteComplianceData{}
		data = append(data, grc.CompleteCompliance{
			PolicyID:                  createdPolicyId,
			NonCompliantClusters:      []string{"cluster1"},
			UnknownComplianceClusters: []string{"cluster3"},
		})

		evt := ToCloudEvent(leafHubName, string(enum.LocalCompleteComplianceType), version, data)
		evt.SetExtension(eventversion.ExtDependencyVersion, complianceVersion.String())

		By("Sync message with transport")
		err := producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())

		By("Check the complete bundle updated all the policy status in the database")
		Eventually(func() error {
			var localCompliances []models.LocalStatusCompliance
			err = db.Where("leaf_hub_name = ?", leafHubName).Find(&localCompliances).Error
			if err != nil {
				return err
			}

			success := 0
			for _, c := range localCompliances {
				fmt.Printf("LocalComplete: id(%s) %s/%s %s \n", c.PolicyID, c.LeafHubName, c.ClusterName, c.Compliance)
				if c.PolicyID == createdPolicyId {
					if c.ClusterName == "cluster1" && c.Compliance == database.NonCompliant {
						success++
					}
					if c.ClusterName == "cluster2" && c.Compliance == database.Compliant {
						success++
					}
					if c.ClusterName == "cluster3" {
						return fmt.Errorf("the cluster3 shouldn't synced by the local compliance bundle")
					}
				}
			}

			if len(localCompliances) == 2 && success == 2 {
				return nil
			}
			return fmt.Errorf("failed to sync local complete compliance")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})
