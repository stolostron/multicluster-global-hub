package dbsyncer_test

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var _ = Describe("Local StatusCompliances", Ordered, func() {
	const (
		testSchema      = "local_status"
		complianceTable = "compliance"
		leafHubName     = "hub1"
		createdPolicyId = "d9347b09-bb46-4e2b-91ea-513e83ab9ea8"
	)

	It("LocalClustersPerPolicy bundle should pass", func() {
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
			return fmt.Errorf("failed to load content of table %s.%s", testSchema, complianceTable)
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())

		By("Build a new policy bundle in the managed hub")
		version := status.NewBundleVersion()
		version.Incr()
		// policy bundle
		clusterPerPolicyBundle := status.BaseClustersPerPolicyBundle{
			Objects:       make([]*status.PolicyGenericComplianceStatus, 0),
			LeafHubName:   leafHubName,
			BundleVersion: version,
		}
		clusterPerPolicyBundle.Objects = append(clusterPerPolicyBundle.Objects, &status.PolicyGenericComplianceStatus{
			PolicyID:                  createdPolicyId,
			CompliantClusters:         []string{"cluster1"},
			NonCompliantClusters:      []string{"cluster2"},
			UnknownComplianceClusters: make([]string, 0),
		})
		// transport bundle
		clustersPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.LocalClustersPerPolicyMsgKey)
		payloadBytes, err := json.Marshal(clusterPerPolicyBundle)
		Expect(err).ToNot(HaveOccurred())

		By("Synchronize the latest ClustersPerPolicy bundle with transport")
		transportMessage := &transport.Message{
			Key:     clustersPerPolicyTransportKey,
			ID:      clustersPerPolicyTransportKey, // entry.transportBundleKey
			MsgType: constants.StatusBundle,
			Version: clusterPerPolicyBundle.BundleVersion.String(), // entry.bundle.GetBundleVersion().String()
			Payload: payloadBytes,
		}
		By("Sync message with transport")
		err = producer.Send(ctx, transportMessage)
		Expect(err).Should(Succeed())

		By("Check the ClustersPerPolicy policy is created and expired policy is deleted from database")
		Eventually(func() error {
			var localCompliances []models.LocalStatusCompliance
			err = db.Where("leaf_hub_name = ?", leafHubName).Find(&localCompliances).Error
			if err != nil {
				return err
			}
			expiredCount := 0
			addedCount := 0
			for _, localCompliance := range localCompliances {
				fmt.Printf("LocalClustersPerPolicy: id(%s) %s/%s %s \n", localCompliance.PolicyID,
					localCompliance.LeafHubName, localCompliance.ClusterName, localCompliance.Compliance)
				if localCompliance.PolicyID == expiredPolicyID {
					expiredCount++
				}
				if localCompliance.PolicyID == createdPolicyId {
					addedCount++
				}
			}
			if expiredCount == 0 && addedCount == 2 {
				return nil
			}
			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, complianceTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})

	It("LocalCompleteComplianceStatusBundle pass", func() {
		db := database.GetGorm()
		By("Create a complete compliance bundle")
		version := status.NewBundleVersion()
		version.Incr()
		completeComplianceStatusBundle := status.BaseCompleteComplianceStatusBundle{
			Objects:           make([]*status.PolicyCompleteComplianceStatus, 0),
			LeafHubName:       leafHubName,
			BundleVersion:     version,
			BaseBundleVersion: version,
		}
		// hub1-cluster1 compliant => hub1-cluster1 non_compliant
		// hub1-cluster2 non_compliant => hub1-cluster2 compliant
		completeComplianceStatusBundle.Objects = append(
			completeComplianceStatusBundle.Objects, &status.PolicyCompleteComplianceStatus{
				PolicyID:                  createdPolicyId,
				NonCompliantClusters:      []string{"cluster1"},
				UnknownComplianceClusters: []string{"cluster3"},
			})
		// transport bundle
		policyCompleteComplianceTransportKey := fmt.Sprintf("%s.%s", leafHubName,
			constants.LocalPolicyCompleteComplianceMsgKey)
		completePayloadBytes, err := json.Marshal(completeComplianceStatusBundle)
		Expect(err).ToNot(HaveOccurred())

		By("Synchronize the complete policy bundle with transport")
		transportMessage := &transport.Message{
			Key:     policyCompleteComplianceTransportKey,
			ID:      policyCompleteComplianceTransportKey, // entry.transportBundleKey
			MsgType: constants.StatusBundle,
			Version: completeComplianceStatusBundle.BundleVersion.String(), // entry.bundle.GetBundleVersion().String()
			Payload: completePayloadBytes,
		}
		By("Sync message with transport")
		err = producer.Send(ctx, transportMessage)
		Expect(err).Should(Succeed())

		By("Check the complete bundle updated all the policy status in the database")
		Eventually(func() error {
			var localCompliances []models.LocalStatusCompliance
			err = db.Where("leaf_hub_name = ?", leafHubName).Find(&localCompliances).Error
			if err != nil {
				return err
			}

			success := 0
			for _, localCompliance := range localCompliances {
				fmt.Printf("LocalCompleteCompliance: id(%s) %s/%s %s \n", localCompliance.PolicyID,
					localCompliance.LeafHubName, localCompliance.ClusterName, localCompliance.Compliance)
				if localCompliance.PolicyID == createdPolicyId {
					if localCompliance.ClusterName == "cluster1" &&
						localCompliance.Compliance == database.NonCompliant {
						success++
					}
					if localCompliance.ClusterName == "cluster2" &&
						localCompliance.Compliance == database.Compliant {
						success++
					}
					if localCompliance.ClusterName == "cluster3" {
						return fmt.Errorf("the cluster3 shouldn't synced by the local compliance bundle")
					}
				}
			}

			if success == 2 {
				return nil
			}
			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, complianceTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})
})
