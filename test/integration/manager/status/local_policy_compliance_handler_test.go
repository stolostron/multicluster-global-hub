package status

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

// go test /test/integration/manager/status -v -ginkgo.focus "LocalPolicyComplianceHandler"
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

		data := grc.ComplianceBundle{}
		data = append(data, grc.Compliance{
			PolicyID:                  createdPolicyId,
			NamespacedName:            "default/policy1",
			CompliantClusters:         []string{"cluster1"},
			NonCompliantClusters:      []string{"cluster2"},
			PendingComplianceClusters: []string{"cluster4"},
			UnknownComplianceClusters: []string{},
		})

		evt := ToCloudEvent(leafHubName, string(enum.LocalComplianceType), complianceVersion, data)

		By("Sync message with transport")
		err = producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())
		complianceVersion.Next()

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
				if c.PolicyID == createdPolicyId && c.PolicyNamespacedName == "default/policy1" && c.ClusterName == "cluster1" || c.ClusterName == "cluster2" || c.ClusterName == "cluster4" {
					addedCount++
				}

				fmt.Printf("LocalCompliance: ID(%s) %s/%s %s \n", c.PolicyID, c.LeafHubName, c.ClusterName, c.Compliance)
			}
			if expiredCount == 0 && addedCount == 3 && len(localCompliances) == 3 {
				return nil
			}
			return fmt.Errorf("failed to sync local compliance")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should handle the local compliance event with manager resync", func() {
		By("Add an expired policy to the database")
		db := database.GetGorm()
		err := db.Create(&models.LocalStatusCompliance{
			PolicyID:    createdPolicyId,
			ClusterName: "cluster3",
			LeafHubName: leafHubName,
			Compliance:  database.Compliant,
			Error:       "none",
		}).Error
		Expect(err).ToNot(HaveOccurred())

		By("Check the expired policy is added in database")
		Eventually(func() error {
			var localCompliances []models.LocalStatusCompliance
			err = db.Where("policy_id = ?", createdPolicyId).Find(&localCompliances).Error
			if err != nil {
				return err
			}

			for _, localCompliance := range localCompliances {
				if localCompliance.ClusterName == "cluster3" &&
					localCompliance.Compliance == database.Compliant {
					return nil
				}
			}
			return fmt.Errorf("failed to persist data to local compliance of table")
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		By("Build a new policy bundle in the managed hub")
		data := grc.ComplianceBundle{}
		data = append(data, grc.Compliance{
			PolicyID:                  createdPolicyId,
			NamespacedName:            "default/policy1",
			CompliantClusters:         []string{"cluster1"},
			NonCompliantClusters:      []string{"cluster2"},
			PendingComplianceClusters: []string{"cluster5"},
			UnknownComplianceClusters: []string{},
		})
		complianceVersion.Incr()

		evt := ToCloudEvent(leafHubName, string(enum.LocalComplianceType), complianceVersion, data)

		By("Sync message with transport")
		err = producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())
		complianceVersion.Next()

		By("Wait the resync finished")
		time.Sleep(5 * time.Second)

		By("Check the local compliance is created and expired policy is deleted from database")
		Eventually(func() error {
			var localCompliances []models.LocalStatusCompliance
			err = db.Where("leaf_hub_name = ?", leafHubName).Find(&localCompliances).Error
			if err != nil {
				return err
			}

			addedCount := 0
			for _, c := range localCompliances {
				fmt.Printf("LocalCompliance Resync: ID(%s) %s/%s %s \n", c.PolicyID, c.LeafHubName, c.ClusterName, c.Compliance)
				if c.PolicyID == createdPolicyId && c.PolicyNamespacedName == "default/policy1" &&
					c.ClusterName == "cluster1" || c.ClusterName == "cluster2" || c.ClusterName == "cluster5" {
					addedCount++
				}
				if c.ClusterName == "cluster3" {
					return fmt.Errorf("the cluster3 should be removed from database")
				}
			}
			if addedCount == 3 && len(localCompliances) == 3 {
				return nil
			}
			return fmt.Errorf("failed to sync local compliance")
		}, 10*time.Second, 3*time.Second).ShouldNot(HaveOccurred())
	})

	It("shouldn't update the by the local complete compliance event", func() {
		db := database.GetGorm()
		By("Create a complete compliance bundle")
		version := eventversion.NewVersion()
		version.Incr() // first generation -> reset

		// hub1-cluster1 compliant
		// hub1-cluster2 non_compliant
		data := grc.CompleteComplianceBundle{}
		data = append(data, grc.CompleteCompliance{
			PolicyID:             createdPolicyId,
			NonCompliantClusters: []string{"cluster2"},
		})

		evt := ToCloudEvent(leafHubName, string(enum.LocalCompleteComplianceType), version, data)
		evt.SetExtension(eventversion.ExtDependencyVersion, complianceVersion.String())

		By("Sync message with transport")
		err := producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())

		time.Sleep(5 * time.Second)

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
					if c.ClusterName == "cluster1" && c.Compliance == database.Compliant {
						success++
					}
					if c.ClusterName == "cluster2" && c.Compliance == database.NonCompliant {
						success++
					}
				}
			}

			if success == 2 {
				return nil
			}
			return fmt.Errorf("failed to sync local complete compliance")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should handle the local complete compliance event", func() {
		db := database.GetGorm()
		By("Create a complete compliance bundle")
		version := eventversion.NewVersion()
		version.Incr() // first generation -> reset

		// hub1-cluster1 compliant => hub1-cluster1 non_compliant
		// hub1-cluster2 non_compliant => hub1-cluster2 compliant
		data := grc.CompleteComplianceBundle{}
		data = append(data, grc.CompleteCompliance{
			PolicyID:                  createdPolicyId,
			NonCompliantClusters:      []string{"cluster1"},
			UnknownComplianceClusters: []string{"cluster3"},
			PendingComplianceClusters: []string{"cluster5"},
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
					if c.ClusterName == "cluster5" && c.Compliance == database.Pending {
						success++
					}
					if c.ClusterName == "cluster3" {
						return fmt.Errorf("the cluster3 shouldn't synced by the local compliance bundle")
					}
				}
			}

			if len(localCompliances) == 3 && success == 3 {
				return nil
			}
			return fmt.Errorf("failed to sync local complete compliance")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})
