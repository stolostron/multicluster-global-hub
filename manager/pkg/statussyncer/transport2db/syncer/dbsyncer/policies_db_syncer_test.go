package dbsyncer_test

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/db"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/registration"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var _ = Describe("Policies", Ordered, func() {
	const (
		testSchema                = "status"
		complianceTable           = "compliance"
		aggregatedComplianceTable = "aggregated_compliance"
		leafHubName               = "hub1"
		createdPolicyId           = "d9347b09-bb46-4e2b-91ea-513e83ab9ea7"
	)

	BeforeAll(func() {
		By("Create status compliance table in database")
		_, err := transportPostgreSQL.GetConn().Exec(ctx, `
			CREATE SCHEMA IF NOT EXISTS status;
			DO $$ BEGIN
				CREATE TYPE status.compliance_type AS ENUM (
					'compliant',
					'non_compliant',
					'unknown'
				);
			EXCEPTION
				WHEN duplicate_object THEN null;
			END $$;
			DO $$ BEGIN
				CREATE TYPE status.error_type AS ENUM (
					'disconnected',
					'none'
				);
			EXCEPTION
				WHEN duplicate_object THEN null;
			END $$;
			CREATE TABLE IF NOT EXISTS  status.compliance (
				id uuid NOT NULL,
				cluster_name character varying(63) NOT NULL,
				leaf_hub_name character varying(63) NOT NULL,
				error status.error_type NOT NULL,
				compliance status.compliance_type NOT NULL
			);
			CREATE TABLE IF NOT EXISTS  status.aggregated_compliance (
				id uuid NOT NULL,
				leaf_hub_name character varying(63) NOT NULL,
				applied_clusters integer NOT NULL,
				non_compliant_clusters integer NOT NULL
			);
		`)
		Expect(err).ToNot(HaveOccurred())

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
			fmt.Sprintf(`INSERT INTO %s.%s (id,cluster_name,leaf_hub_name,error,compliance) VALUES($1, $2, $3, $4, $5)`,
				testSchema, complianceTable), deletedPolicyId, "cluster1", leafHubName, "none", "unknown")
		Expect(err).ToNot(HaveOccurred())

		By("Check the expired policy is added in database")
		Eventually(func() error {
			rows, err := transportPostgreSQL.GetConn().Query(ctx,
				fmt.Sprintf("SELECT id FROM %s.%s", testSchema, complianceTable))
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

		By("Build a new policy bundle in the regional hub")
		// policy bundle
		transportPayload := status.BaseClustersPerPolicyBundle{
			Objects:       make([]*status.PolicyGenericComplianceStatus, 0),
			LeafHubName:   leafHubName,
			BundleVersion: status.NewBundleVersion(1, 0),
		}
		transportPayload.Objects = append(transportPayload.Objects, &status.PolicyGenericComplianceStatus{
			PolicyID:                  createdPolicyId,
			CompliantClusters:         []string{"cluster1"}, // generate record: createdPolicyId hub1-cluster1 compliant
			NonCompliantClusters:      []string{"cluster2"}, // generate record: createdPolicyId hub1-cluster2 non_compliant
			UnknownComplianceClusters: make([]string, 0),
		})
		// transport bundle
		clustersPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.ClustersPerPolicyMsgKey)
		payloadBytes, err := json.Marshal(transportPayload)
		Expect(err).ToNot(HaveOccurred())

		By("Synchronize the latest ClustersPerPolicy bundle with transport")
		kafkaMessage, err := buildKafkaMessage(clustersPerPolicyTransportKey,
			clustersPerPolicyTransportKey, payloadBytes)
		Expect(err).ToNot(HaveOccurred())
		kafkaMessageChan <- kafkaMessage

		By("Check the ClustersPerPolicy policy is created and expired policy is deleted from database")
		Eventually(func() error {
			querySql := fmt.Sprintf("SELECT id,cluster_name,leaf_hub_name,compliance FROM %s.%s", testSchema, complianceTable)
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
					complianceStatus                   db.ComplianceStatus
				)
				if err := rows.Scan(&policyId, &clusterName, &leafHubName, &complianceStatus); err != nil {
					return err
				}
				fmt.Printf("id(%s) %s-%s: %s \n", policyId, leafHubName, clusterName, complianceStatus)
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

	It("update the policy status with PolicyDeltaCompliance bundle where aggregationLevel = full", func() {
		By("Create the delta policy bundle")
		transportPayload := status.BaseDeltaComplianceStatusBundle{
			Objects:       make([]*status.PolicyGenericComplianceStatus, 0),
			LeafHubName:   leafHubName,
			BundleVersion: status.NewBundleVersion(1, 0),
		}
		// the delta policy bundle tries to do the following action:
		// 1. delete record: createdPolicyId hub1-cluster1 compliant
		// 2. update record: createdPolicyId hub1-cluster2 non_compliant => createdPolicyId hub1-cluster2 compliant
		// 3. insert record: createdPolicyId hub1-cluster3 non_compliant
		transportPayload.Objects = append(transportPayload.Objects, &status.PolicyGenericComplianceStatus{
			PolicyID:                  createdPolicyId,
			CompliantClusters:         []string{"cluster2"},
			NonCompliantClusters:      []string{"cluster3"},
			UnknownComplianceClusters: make([]string, 0),
		})
		// transport bundle
		policyDeltaComplianceTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.PolicyDeltaComplianceMsgKey)
		payloadBytes, err := json.Marshal(transportPayload)
		Expect(err).ToNot(HaveOccurred())

		By("Synchronize the delta policy bundle with transport")
		kafkaMessage, err := buildKafkaMessage(policyDeltaComplianceTransportKey,
			policyDeltaComplianceTransportKey, payloadBytes)
		Expect(err).ToNot(HaveOccurred())
		kafkaMessageChan <- kafkaMessage

		By("Check the delta policy bundle is only update compliance status of the existing record in database")
		Eventually(func() error {
			querySql := fmt.Sprintf("SELECT id,cluster_name,leaf_hub_name,compliance FROM %s.%s", testSchema, complianceTable)
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
					complianceStatus               db.ComplianceStatus
				)
				if err := rows.Scan(&policyId, &clusterName, &hubName, &complianceStatus); err != nil {
					return err
				}
				fmt.Printf("id(%s) %s-%s: %s \n", policyId, leafHubName, clusterName, complianceStatus)
				if policyId == createdPolicyId && hubName == leafHubName {
					// delete record: createdPolicyId hub1 cluster1 compliant
					if clusterName == "cluster1" && complianceStatus == "compliant" {
						isDeleted = false
					}
					// update record: createdPolicyId hub1-cluster2 non_compliant => createdPolicyId hub1-cluster2 compliant
					if clusterName == "cluster2" && complianceStatus == "compliant" {
						isUpdated = true
					}
					// insert record: createdPolicyId hub1-cluster3 non_compliant
					if clusterName == "cluster3" && complianceStatus == "non_compliant" {
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
	})

	It("sync the aggregated policy with MinimalPolicyCompliance bundle where aggregationLevel = minimal", func() {
		By("Overwrite the MinimalComplianceStatusBundle Predicate function, so that the minimal bundle cloud be processed")
		statusTransport.BundleRegister(&registration.BundleRegistration{
			MsgID:            constants.MinimalPolicyComplianceMsgKey,
			CreateBundleFunc: statusbundle.NewMinimalComplianceStatusBundle,
			Predicate: func() bool {
				return true // syncer.config.Data["aggregationLevel"] == "minimal"
			},
		})

		By("Create the minimal policy bundle")
		transportPayload := status.BaseMinimalComplianceStatusBundle{
			Objects:       make([]*status.MinimalPolicyComplianceStatus, 0),
			LeafHubName:   leafHubName,
			BundleVersion: status.NewBundleVersion(1, 0),
		}
		transportPayload.Objects = append(transportPayload.Objects, &status.MinimalPolicyComplianceStatus{
			PolicyID:             createdPolicyId,
			RemediationAction:    policyv1.Inform,
			NonCompliantClusters: 2,
			AppliedClusters:      3,
		})
		// transport bundle
		minimalPolicyComplianceTransportKey :=
			fmt.Sprintf("%s.%s", leafHubName, constants.MinimalPolicyComplianceMsgKey)
		payloadBytes, err := json.Marshal(transportPayload)
		Expect(err).ToNot(HaveOccurred())

		By("Synchronize the policy bundle with transport")
		kafkaMessage, err := buildKafkaMessage(minimalPolicyComplianceTransportKey,
			minimalPolicyComplianceTransportKey, payloadBytes)
		Expect(err).ToNot(HaveOccurred())
		kafkaMessageChan <- kafkaMessage

		By("Check the minimal policy is synchronized to database")
		Eventually(func() error {
			querySql := fmt.Sprintf("SELECT id,leaf_hub_name,applied_clusters,non_compliant_clusters FROM %s.%s", testSchema, aggregatedComplianceTable)
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
				fmt.Printf("%s %s %d %d \n", policyId, hubName, appliedClusters, nonCompliantClusters)
				if policyId == createdPolicyId && hubName == leafHubName &&
					appliedClusters == 3 && nonCompliantClusters == 2 {
					return nil
				}
			}
			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, aggregatedComplianceTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})
})
