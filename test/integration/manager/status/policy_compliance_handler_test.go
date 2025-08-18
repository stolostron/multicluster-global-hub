package status

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test /test/integration/manager/status -v -ginkgo.focus "GlobalPolicyComplianceHandler"
var _ = Describe("GlobalPolicyComplianceHandler", Ordered, func() {
	const (
		leafHubName               = "hub1"
		createdPolicyId           = "d9347b09-bb46-4e2b-91ea-513e83ab9ea6"
		aggregatedComplianceTable = "aggregated_compliance"
	)
	var (
		complianceVersion *eventversion.Version
		completeVersion   *eventversion.Version
	)

	It("should handle the compliance event", func() {
		By("Add an expired policy to the database")
		db := database.GetGorm()
		expiredPolicyID := "b8b3e164-377e-4be1-a870-992265f31f7c"
		err := db.Create(&models.StatusCompliance{
			PolicyID:    expiredPolicyID,
			ClusterName: "cluster1",
			LeafHubName: leafHubName,
			Compliance:  database.Unknown,
			Error:       "none",
		}).Error
		Expect(err).ToNot(HaveOccurred())

		By("Check the expired policy is added in database")
		Eventually(func() error {
			var compliance models.StatusCompliance
			err = db.Where("policy_id = ?", expiredPolicyID).First(&compliance).Error
			if err != nil {
				return err
			}

			if compliance.ClusterName == "cluster1" && compliance.Compliance == database.Unknown {
				return nil
			}
			return fmt.Errorf("failed to persist data to compliance of table")
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		By("Build a new policy compliance on the managed hub")
		complianceVersion = eventversion.NewVersion()
		complianceVersion.Incr()

		data := grc.ComplianceBundle{}
		data = append(data, grc.Compliance{
			PolicyID:                  createdPolicyId,
			CompliantClusters:         []string{"cluster1"}, // generate record: createdPolicyId hub1-cluster1 compliant
			NonCompliantClusters:      []string{"cluster2"}, // generate record: createdPolicyId hub1-cluster2 non_compliant
			PendingComplianceClusters: []string{"cluster4"}, // generate record: createdPolicyId hub1-cluster4 pending
			UnknownComplianceClusters: []string{},
		})

		evt := ToCloudEvent(leafHubName, string(enum.ComplianceType), complianceVersion, data)

		By("Sync message with transport")
		err = producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())

		By("Check the compliance is created and expired policy is deleted from database")
		Eventually(func() error {
			var compliances []models.StatusCompliance
			err = db.Where("leaf_hub_name = ?", leafHubName).Find(&compliances).Error
			if err != nil {
				return err
			}

			expiredCount := 0
			addedCount := 0
			for _, c := range compliances {
				if c.PolicyID == expiredPolicyID && c.ClusterName == "cluster1" {
					expiredCount++
				}
				if c.PolicyID == createdPolicyId && c.ClusterName == "cluster1" || c.ClusterName == "cluster2" || c.ClusterName == "cluster4" {
					addedCount++
				}

				fmt.Printf("Compliance: ID(%s) %s/%s %s \n", c.PolicyID, c.LeafHubName, c.ClusterName, c.Compliance)
			}
			if expiredCount == 0 && addedCount == 3 && len(compliances) == 3 {
				return nil
			}
			return fmt.Errorf("failed to sync compliance")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("shouldn't update the by the complete compliance event", func() {
		db := database.GetGorm()
		By("Create a complete compliance bundle")
		completeVersion = eventversion.NewVersion()
		completeVersion.Incr() // first generation -> reset

		// hub1-cluster1 compliant
		// hub1-cluster2 non_compliant
		data := grc.CompleteComplianceBundle{}
		data = append(data, grc.CompleteCompliance{
			PolicyID:             createdPolicyId,
			NonCompliantClusters: []string{"cluster2"},
		})

		evt := ToCloudEvent(leafHubName, string(enum.CompleteComplianceType), completeVersion, data)
		evt.SetExtension(eventversion.ExtDependencyVersion, complianceVersion.String())

		By("Sync message with transport")
		err := producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())
		completeVersion.Next()

		time.Sleep(5 * time.Second)

		By("Check the complete bundle updated all the policy status in the database")
		Eventually(func() error {
			var compliances []models.StatusCompliance
			err = db.Where("leaf_hub_name = ?", leafHubName).Find(&compliances).Error
			if err != nil {
				return err
			}

			success := 0
			for _, c := range compliances {
				fmt.Printf("Complete(Same): id(%s) %s/%s %s \n", c.PolicyID, c.LeafHubName, c.ClusterName, c.Compliance)
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
			return fmt.Errorf("failed to sync complete compliance")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should handle the complete compliance event", func() {
		By("Create a complete compliance bundle")
		completeVersion.Incr()

		// hub1-cluster1 compliant => hub1-cluster1 non_compliant
		// hub1-cluster2 non_compliant => hub1-cluster2 compliant
		data := grc.CompleteComplianceBundle{}
		data = append(data, grc.CompleteCompliance{
			PolicyID:                  createdPolicyId,
			NonCompliantClusters:      []string{"cluster1"},
			UnknownComplianceClusters: []string{"cluster3"},
			PendingComplianceClusters: []string{"cluster4"},
		})

		evt := ToCloudEvent(leafHubName, string(enum.CompleteComplianceType), completeVersion, data)
		evt.SetExtension(eventversion.ExtDependencyVersion, complianceVersion.String())

		By("Sync message with transport")
		err := producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())
		completeVersion.Next()

		By("Check the complete bundle updated all the policy status in the database")
		Eventually(func() error {
			var compliances []models.StatusCompliance
			err = database.GetGorm().Where("leaf_hub_name = ?", leafHubName).Find(&compliances).Error
			if err != nil {
				return err
			}

			success := 0
			for _, c := range compliances {
				fmt.Printf("Complete: id(%s) %s/%s %s \n", c.PolicyID, c.LeafHubName, c.ClusterName, c.Compliance)
				if c.PolicyID == createdPolicyId {
					if c.ClusterName == "cluster1" && c.Compliance == database.NonCompliant {
						success++
					}
					if c.ClusterName == "cluster2" && c.Compliance == database.Compliant {
						success++
					}
					if c.ClusterName == "cluster4" && c.Compliance == database.Pending {
						success++
					}
					if c.ClusterName == "cluster3" {
						return fmt.Errorf("the cluster3 shouldn't synced by the compliance bundle")
					}
				}
			}

			if len(compliances) == 3 && success == 3 {
				return nil
			}
			return fmt.Errorf("failed to sync complete compliance")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should be able to handle the delta policy compliance event", func() {
		Skip("Special the delta event test for now")

		By("Create the delta policy event")
		version := eventversion.NewVersion()
		version.Incr()

		// before send the delta event:
		// id(d9347b09-bb46-4e2b-91ea-513e83ab9ea7) hub1-cluster1 non_compliant
		// id(d9347b09-bb46-4e2b-91ea-513e83ab9ea7) hub1-cluster2 compliant
		data := grc.DeltaComplianceBundle{}
		data = append(data, grc.Compliance{
			PolicyID:                  createdPolicyId,
			CompliantClusters:         []string{"cluster1"},
			NonCompliantClusters:      []string{"cluster3"},
			UnknownComplianceClusters: []string{},
		})
		// expect:
		// id(d9347b09-bb46-4e2b-91ea-513e83ab9ea7) hub1-cluster1 compliant
		// id(d9347b09-bb46-4e2b-91ea-513e83ab9ea7) hub1-cluster2 compliant

		evt := ToCloudEvent(leafHubName, string(enum.DeltaComplianceType), version, data)
		evt.SetExtension(eventversion.ExtDependencyVersion, completeVersion.String())

		err := producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())

		By("Check the delta policy bundle is only update compliance status of the existing record in database")
		Eventually(func() error {
			var compliances []models.StatusCompliance
			err = database.GetGorm().Where("leaf_hub_name = ?", leafHubName).Find(&compliances).Error
			if err != nil {
				return err
			}

			success := 0
			for _, c := range compliances {
				fmt.Printf("Delta1: id(%s) %s/%s %s \n", c.PolicyID, c.LeafHubName, c.ClusterName, c.Compliance)
				if c.PolicyID == createdPolicyId {
					if c.ClusterName == "cluster1" && c.Compliance == database.Compliant {
						success++
					}
					if c.ClusterName == "cluster2" && c.Compliance == database.Compliant {
						success++
					}
					if c.ClusterName == "cluster3" {
						return fmt.Errorf("the cluster3 shouldn't synced by the delta compliance event")
					}
				}
			}

			if len(compliances) == 2 && success == 2 {
				return nil
			}
			return fmt.Errorf("failed to sync delta compliance")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		// update the hub1-cluster1 compliant to noncompliant with DeltaComplianceBundle

		By("Create another updated delta policy event")
		data = grc.DeltaComplianceBundle{}
		// before send the delta bundle:
		// id(d9347b09-bb46-4e2b-91ea-513e83ab9ea7) hub1-cluster1 compliant
		// id(d9347b09-bb46-4e2b-91ea-513e83ab9ea7) hub1-cluster2 compliant
		data = append(data, grc.Compliance{
			PolicyID:                  createdPolicyId,
			CompliantClusters:         []string{},
			NonCompliantClusters:      []string{"cluster1"},
			UnknownComplianceClusters: []string{},
		})
		version.Incr()
		// expect:
		// id(d9347b09-bb46-4e2b-91ea-513e83ab9ea7) hub1-cluster2 compliant
		// id(d9347b09-bb46-4e2b-91ea-513e83ab9ea7) hub1-cluster1 non_compliant

		By("Synchronize the updated delta policy bundle with transport")
		evt = ToCloudEvent(leafHubName, string(enum.DeltaComplianceType), version, data)
		evt.SetExtension(eventversion.ExtDependencyVersion, completeVersion.String())

		err = producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())

		By("Check the updated delta policy bundle is synchronized to database")
		Eventually(func() error {
			var compliances []models.StatusCompliance
			err = database.GetGorm().Where("leaf_hub_name = ?", leafHubName).Find(&compliances).Error
			if err != nil {
				return err
			}

			success := 0
			for _, c := range compliances {
				fmt.Printf("Delta2: id(%s) %s/%s %s \n", c.PolicyID, c.LeafHubName, c.ClusterName, c.Compliance)
				if c.PolicyID == createdPolicyId {
					if c.ClusterName == "cluster1" && c.Compliance == database.NonCompliant {
						success++
					}
					if c.ClusterName == "cluster2" && c.Compliance == database.Compliant {
						success++
					}
					if c.ClusterName == "cluster3" {
						return fmt.Errorf("the cluster3 shouldn't synced by the delta compliance event")
					}
				}
			}

			if len(compliances) == 2 && success == 2 {
				return nil
			}
			return fmt.Errorf("failed to sync delta compliance")
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})

	It("should be able to handle the aggregated policy event where aggregationLevel = minimal", func() {
		// By("Overwrite the agent aggregationLevel with minimal")

		By("Create Event")
		version := eventversion.NewVersion()
		version.Incr()

		data := grc.MinimalComplianceBundle{}
		data = append(data, grc.MinimalCompliance{
			PolicyID:             createdPolicyId,
			RemediationAction:    policiesv1.Inform,
			NonCompliantClusters: 2,
			AppliedClusters:      3,
		})

		By("Synchronize the updated mini policy compliance with transport")
		evt := ToCloudEvent(leafHubName, string(enum.MiniComplianceType), version, data)
		err := producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())

		By("Check the minimal policy is synchronized to database")
		Eventually(func() error {
			sql := fmt.Sprintf("SELECT policy_id,leaf_hub_name,applied_clusters,non_compliant_clusters FROM %s.%s",
				database.StatusSchema, aggregatedComplianceTable)
			rows, err := database.GetGorm().Raw(sql).Rows()
			if err != nil {
				return err
			}
			defer func() {
				if err := rows.Close(); err != nil {
					fmt.Printf("failed to close rows: %v\n", err)
				}
			}()
			for rows.Next() {
				var (
					policyId, hubName                     string
					appliedClusters, nonCompliantClusters int
				)
				if err := rows.Scan(&policyId, &hubName, &appliedClusters, &nonCompliantClusters); err != nil {
					return err
				}
				fmt.Printf("MinimalCompliance: id(%s) %s %d %d \n", policyId,
					hubName, appliedClusters, nonCompliantClusters)
				if policyId == createdPolicyId && hubName == leafHubName &&
					appliedClusters == 3 && nonCompliantClusters == 2 {
					return nil
				}
			}
			return fmt.Errorf("failed to sync minimal compliance table")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})
