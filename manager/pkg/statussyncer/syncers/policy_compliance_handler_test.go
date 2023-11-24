package dbsyncer_test

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/registration"
)

var _ = Describe("Status Compliances", Ordered, func() {
	const (
		testSchema                = "status"
		complianceTable           = "compliance"
		aggregatedComplianceTable = "aggregated_compliance"
		leafHubName               = "hub1"
		createdPolicyId           = "d9347b09-bb46-4e2b-91ea-513e83ab9ea7"
	)

	BeforeAll(func() {
		By("Check whether the tables are created")
		Eventually(func() error {
			rows, err := transportPostgreSQL.GetConn().Query(ctx, "SELECT * FROM pg_tables")
			if err != nil {
				return err
			}
			defer rows.Close()
			complianceReady := false
			aggregatedComplianceReady := false
			for rows.Next() {
				columnValues, _ := rows.Values()
				schema := columnValues[0]
				table := columnValues[1]
				if schema == testSchema && table == complianceTable {
					complianceReady = true
				}
				if schema == testSchema && table == aggregatedComplianceTable {
					aggregatedComplianceReady = true
				}
			}
			if complianceReady && aggregatedComplianceReady {
				return nil
			}
			return fmt.Errorf("failed to create test table %s: %s and %s", testSchema,
				complianceTable, aggregatedComplianceTable)
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})

	It("delete and insert policy with ClustersPerPolicy Bundle where aggregationLevel = full", func() {
		By("Add an expired policy to the database")
		deletedPolicyId := "b8b3e164-477e-4be1-a870-992265f31f7d"
		_, err := transportPostgreSQL.GetConn().Exec(ctx,
			fmt.Sprintf(`INSERT INTO %s.%s (policy_id,cluster_name,leaf_hub_name,error,compliance) VALUES($1, $2, $3, $4, $5)`,
				testSchema, complianceTable), deletedPolicyId, "cluster1", leafHubName, "none", "unknown")
		Expect(err).ToNot(HaveOccurred())

		By("Check the expired policy is added in database")
		Eventually(func() error {
			rows, err := transportPostgreSQL.GetConn().Query(ctx,
				fmt.Sprintf("SELECT policy_id FROM %s.%s", testSchema, complianceTable))
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var policyId string
				err = rows.Scan(&policyId)
				if err != nil {
					return err
				}
				if policyId == deletedPolicyId {
					return nil
				}
			}
			return fmt.Errorf("failed to load content of table %s.%s", testSchema, complianceTable)
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())

		By("Build a new policy bundle in the managed hub")
		// policy bundle
		clusterPerPolicyBundle := base.BaseComplianceBundle{
			Objects:       make([]*base.GenericCompliance, 0),
			LeafHubName:   leafHubName,
			BundleVersion: metadata.NewBundleVersion(),
		}
		clusterPerPolicyBundle.Objects = append(clusterPerPolicyBundle.Objects, &base.GenericCompliance{
			PolicyID:                  createdPolicyId,
			CompliantClusters:         []string{"cluster1"}, // generate record: createdPolicyId hub1-cluster1 compliant
			NonCompliantClusters:      []string{"cluster2"}, // generate record: createdPolicyId hub1-cluster2 non_compliant
			UnknownComplianceClusters: make([]string, 0),
		})
		// transport bundle
		clusterPerPolicyBundle.BundleVersion.Incr()
		clustersPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.ComplianceMsgKey)
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
			querySql := fmt.Sprintf("SELECT policy_id,cluster_name,leaf_hub_name,compliance FROM %s.%s", testSchema, complianceTable)
			rows, err := transportPostgreSQL.GetConn().Query(ctx, querySql)
			if err != nil {
				return err
			}
			defer rows.Close()
			deletedPolicyCount := 0
			createdPolicyCount := 0
			for rows.Next() {
				var (
					policyId, clusterName, leafHubName string
					complianceStatus                   database.ComplianceStatus
				)
				if err := rows.Scan(&policyId, &clusterName, &leafHubName, &complianceStatus); err != nil {
					return err
				}
				fmt.Printf("ClustersPerPolicy: id(%s) %s-%s %s \n", policyId,
					leafHubName, clusterName, complianceStatus)
				if policyId == createdPolicyId {
					createdPolicyCount++
				}
				if policyId == deletedPolicyId {
					deletedPolicyCount++
				}
			}
			if deletedPolicyCount == 0 && createdPolicyCount > 0 {
				return nil
			}
			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, complianceTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})

	It("update the policy status with complete and delta bundle where aggregationLevel = full", func() {
		By("Create a complete compliance bundle")
		completeComplianceStatusBundle := base.BaseCompleteComplianceBundle{
			Objects:           make([]*base.GenericCompleteCompliance, 0),
			LeafHubName:       leafHubName,
			BundleVersion:     metadata.NewBundleVersion(),
			BaseBundleVersion: metadata.NewBundleVersion(),
		}
		// hub1-cluster1 compliant => hub1-cluster1 non_compliant
		// hub1-cluster2 non_compliant => hub1-cluster2 compliant
		completeComplianceStatusBundle.Objects = append(
			completeComplianceStatusBundle.Objects, &base.GenericCompleteCompliance{
				PolicyID:                  createdPolicyId,
				NonCompliantClusters:      []string{"cluster1"},
				UnknownComplianceClusters: []string{"cluster3"},
			})
		// transport bundle
		completeComplianceStatusBundle.BaseBundleVersion.Incr()
		completeComplianceStatusBundle.BundleVersion.Incr()
		policyCompleteComplianceTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.CompleteComplianceMsgKey)
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
			querySql := fmt.Sprintf("SELECT policy_id,cluster_name,leaf_hub_name,compliance FROM %s.%s", testSchema, complianceTable)
			fmt.Printf("CompleteCompliance: Query from the %s.%s \n", testSchema, complianceTable)
			rows, err := transportPostgreSQL.GetConn().Query(ctx, querySql)
			if err != nil {
				return err
			}
			defer rows.Close()
			cluster1Updated := false
			cluster2Updated := false
			for rows.Next() {
				var (
					policyId, clusterName, hubName string
					complianceStatus               database.ComplianceStatus
				)
				if err := rows.Scan(&policyId, &clusterName, &hubName, &complianceStatus); err != nil {
					return err
				}
				fmt.Printf("CompleteCompliance: id(%s) %s-%s %s \n", policyId,
					leafHubName, clusterName, complianceStatus)
				if policyId == createdPolicyId && hubName == leafHubName {
					if clusterName == "cluster1" && complianceStatus == "non_compliant" {
						cluster1Updated = true
					}
					if clusterName == "cluster2" && complianceStatus == "compliant" {
						cluster2Updated = true
					}
				}
			}
			// check deletion do not take effect
			if cluster1Updated && cluster2Updated {
				return nil
			}
			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, complianceTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())

		By("Create the delta policy bundle")
		deltaComplianceStatusBundle := base.BaseDeltaComplianceBundle{
			Objects:           make([]*base.GenericCompliance, 0),
			LeafHubName:       leafHubName,
			BaseBundleVersion: metadata.NewBundleVersion(),
			BundleVersion:     metadata.NewBundleVersion(),
		}
		// before send the delta bundle:
		// id(d9347b09-bb46-4e2b-91ea-513e83ab9ea7) hub1-cluster1 non_compliant
		// id(d9347b09-bb46-4e2b-91ea-513e83ab9ea7) hub1-cluster2 compliant
		deltaComplianceStatusBundle.Objects = append(deltaComplianceStatusBundle.Objects,
			&base.GenericCompliance{
				PolicyID:                  createdPolicyId,
				CompliantClusters:         []string{"cluster1"},
				NonCompliantClusters:      []string{"cluster3"},
				UnknownComplianceClusters: make([]string, 0),
			})
		// expect:
		// id(d9347b09-bb46-4e2b-91ea-513e83ab9ea7) hub1-cluster1 compliant
		// id(d9347b09-bb46-4e2b-91ea-513e83ab9ea7) hub1-cluster2 compliant

		// transport bundle
		deltaComplianceStatusBundle.BaseBundleVersion.Incr()
		deltaComplianceStatusBundle.BundleVersion.Incr()
		policyDeltaComplianceTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.DeltaComplianceMsgKey)
		payloadBytes, err := json.Marshal(deltaComplianceStatusBundle)
		Expect(err).ToNot(HaveOccurred())

		By("Synchronize the delta policy bundle with transport")
		transportMessage = &transport.Message{
			Key:     policyDeltaComplianceTransportKey,
			ID:      policyDeltaComplianceTransportKey, // entry.transportBundleKey
			MsgType: constants.StatusBundle,
			Version: deltaComplianceStatusBundle.BundleVersion.String(), // entry.bundle.GetBundleVersion().String()
			Payload: payloadBytes,
		}
		By("Sync message with transport")
		err = producer.Send(ctx, transportMessage)
		Expect(err).Should(Succeed())

		By("Check the delta policy bundle is only update compliance status of the existing record in database")
		Eventually(func() error {
			querySql := fmt.Sprintf("SELECT policy_id,cluster_name,leaf_hub_name,compliance FROM %s.%s", testSchema, complianceTable)
			fmt.Printf("DeltaCompliance1: Query from the %s.%s \n", testSchema, complianceTable)
			rows, err := transportPostgreSQL.GetConn().Query(ctx, querySql)
			if err != nil {
				return err
			}
			defer rows.Close()
			isDeleted := true
			isUpdated := false
			isInserted := false
			for rows.Next() {
				var (
					policyId, clusterName, hubName string
					complianceStatus               database.ComplianceStatus
				)
				if err := rows.Scan(&policyId, &clusterName, &hubName, &complianceStatus); err != nil {
					return err
				}
				fmt.Printf("DeltaCompliance1: id(%s) %s-%s %s \n", policyId,
					leafHubName, clusterName, complianceStatus)
				if policyId == createdPolicyId && hubName == leafHubName {
					// delete record: createdPolicyId hub1 cluster1 compliant
					if clusterName == "cluster1" && complianceStatus == "compliant" {
						isUpdated = true
					}
					// update record: createdPolicyId hub1-cluster2 non_compliant => createdPolicyId hub1-cluster2 compliant
					if clusterName == "cluster2" {
						isDeleted = false
					}
					// insert record: createdPolicyId hub1-cluster3 non_compliant
					if clusterName == "cluster3" {
						isInserted = true
					}
				}
			}
			// check deletion and creation do not take effect
			if !isDeleted && !isInserted && isUpdated {
				return nil
			}
			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, complianceTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())

		// update the hub1-cluster1 compliant to noncompliant with DeltaComplianceBundle
		By("Create another updated delta policy bundle")
		deltaComplianceStatusBundle = base.BaseDeltaComplianceBundle{
			Objects:           make([]*base.GenericCompliance, 0),
			LeafHubName:       leafHubName,
			BaseBundleVersion: deltaComplianceStatusBundle.BaseBundleVersion,
			BundleVersion:     deltaComplianceStatusBundle.BundleVersion, // increase bundle version
		}
		// before send the delta bundle:
		// id(d9347b09-bb46-4e2b-91ea-513e83ab9ea7) hub1-cluster1 compliant
		// id(d9347b09-bb46-4e2b-91ea-513e83ab9ea7) hub1-cluster2 compliant
		deltaComplianceStatusBundle.BundleVersion.Incr()
		deltaComplianceStatusBundle.Objects = append(deltaComplianceStatusBundle.Objects,
			&base.GenericCompliance{
				PolicyID:                  createdPolicyId,
				CompliantClusters:         []string{},
				NonCompliantClusters:      []string{"cluster1"},
				UnknownComplianceClusters: make([]string, 0),
			})
		// expect:
		// id(d9347b09-bb46-4e2b-91ea-513e83ab9ea7) hub1-cluster2 compliant
		// id(d9347b09-bb46-4e2b-91ea-513e83ab9ea7) hub1-cluster1 non_compliant

		// transport bundle
		policyDeltaComplianceTransportKey = fmt.Sprintf("%s.%s", leafHubName, constants.DeltaComplianceMsgKey)
		payloadBytes, err = json.Marshal(deltaComplianceStatusBundle)
		Expect(err).ToNot(HaveOccurred())

		By("Synchronize the updated delta policy bundle with transport")
		transportMessage = &transport.Message{
			Key:     policyDeltaComplianceTransportKey,
			ID:      policyDeltaComplianceTransportKey, // entry.transportBundleKey
			MsgType: constants.StatusBundle,
			Version: deltaComplianceStatusBundle.BaseBundleVersion.String(), // entry.bundle.GetBundleVersion().String()
			Payload: payloadBytes,
		}
		By("Sync message with transport")
		err = producer.Send(ctx, transportMessage)
		Expect(err).Should(Succeed())

		By("Check the updated delta policy bundle is synchronized to database")
		Eventually(func() error {
			querySql := fmt.Sprintf("SELECT policy_id,cluster_name,leaf_hub_name,compliance FROM %s.%s", testSchema, complianceTable)
			fmt.Printf("DeltaCompliance2: Query from the %s.%s \n", testSchema, complianceTable)
			rows, err := transportPostgreSQL.GetConn().Query(ctx, querySql)
			if err != nil {
				return err
			}
			defer rows.Close()
			isUpdated := false
			for rows.Next() {
				var (
					policyId, clusterName, hubName string
					complianceStatus               database.ComplianceStatus
				)
				if err := rows.Scan(&policyId, &clusterName, &hubName, &complianceStatus); err != nil {
					return err
				}
				fmt.Printf("DeltaCompliance2: id(%s) %s-%s %s \n", policyId,
					leafHubName, clusterName, complianceStatus)
				if policyId == createdPolicyId && hubName == leafHubName {
					// update record: createdPolicyId hub1 cluster1 compliant => createdPolicyId hub1 cluster1 noncompliant
					if clusterName == "cluster1" && complianceStatus == "non_compliant" {
						isUpdated = true
					}
				}
			}
			if isUpdated {
				return nil
			}
			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, complianceTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})

	It("sync the aggregated policy with MinimalPolicyCompliance bundle where aggregationLevel = minimal", func() {
		By("Overwrite the MinimalComplianceStatusBundle Predicate function, so that the minimal bundle cloud be processed")
		// kafkaConsumer.BundleRegister(&registration.BundleRegistration{
		// 	MsgID:            constants.MinimalPolicyComplianceMsgKey,
		// 	CreateBundleFunc: statusbundle.NewMinimalComplianceStatusBundle,
		// 	Predicate: func() bool {
		// 		return true // syncer.config.Data["aggregationLevel"] == "minimal"
		// 	},
		// })
		transportDispatcher.BundleRegister(&registration.BundleRegistration{
			MsgID:            constants.MinimalComplianceMsgKey,
			CreateBundleFunc: grc.NewManagerMinimalComplianceBundle,
			Predicate: func() bool {
				return true // syncer.config.Data["aggregationLevel"] == "minimal"
			},
		})

		By("Create the minimal policy bundle")
		minimalComplianceBundle := base.BaseMinimalComplianceBundle{
			Objects:       make([]*base.MinimalCompliance, 0),
			LeafHubName:   leafHubName,
			BundleVersion: metadata.NewBundleVersion(),
		}
		minimalComplianceBundle.Objects = append(minimalComplianceBundle.Objects, &base.MinimalCompliance{
			PolicyID:             createdPolicyId,
			RemediationAction:    policyv1.Inform,
			NonCompliantClusters: 2,
			AppliedClusters:      3,
		})
		// transport bundle
		minimalComplianceBundle.BundleVersion.Incr()
		minimalPolicyComplianceTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.MinimalComplianceMsgKey)
		payloadBytes, err := json.Marshal(minimalComplianceBundle)
		Expect(err).ToNot(HaveOccurred())

		By("Synchronize the policy bundle with transport")
		transportMessage := &transport.Message{
			Key:     minimalPolicyComplianceTransportKey,
			ID:      minimalPolicyComplianceTransportKey, // entry.transportBundleKey
			MsgType: constants.StatusBundle,
			Version: minimalComplianceBundle.BundleVersion.String(), // entry.bundle.GetBundleVersion().String()
			Payload: payloadBytes,
		}
		By("Sync message with transport")
		err = producer.Send(ctx, transportMessage)
		Expect(err).Should(Succeed())

		By("Check the minimal policy is synchronized to database")
		Eventually(func() error {
			querySql := fmt.Sprintf("SELECT policy_id,leaf_hub_name,applied_clusters,non_compliant_clusters FROM %s.%s", testSchema, aggregatedComplianceTable)
			rows, err := transportPostgreSQL.GetConn().Query(ctx, querySql)
			if err != nil {
				return err
			}
			defer rows.Close()
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
			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, aggregatedComplianceTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})
})
