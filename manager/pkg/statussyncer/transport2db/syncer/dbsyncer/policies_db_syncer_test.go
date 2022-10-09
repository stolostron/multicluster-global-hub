package dbsyncer_test

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	producer "github.com/stolostron/multicluster-global-hub/agent/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/db"
	statusbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/compressor"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/kafka/headers"
)

var _ = Describe("Policies", Ordered, func() {
	const testSchema = "status"
	const testTable = "compliance"
	const leafHubName = "hub1"

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
		`)
		Expect(err).ToNot(HaveOccurred())

		By("Check whether the table is created")
		Eventually(func() error {
			rows, err := transportPostgreSQL.GetConn().Query(ctx, "SELECT * FROM pg_tables")
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				columnValues, _ := rows.Values()
				schema := columnValues[0]
				table := columnValues[1]
				if schema == testSchema && table == testTable {
					return nil
				}
			}
			return fmt.Errorf("failed to create test table %s.%s", testSchema, testTable)
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})

	It("delete the expired policy from database", func() {
		deletedPolicyId := "b8b3e164-477e-4be1-a870-992265f31f7d"
		createdPolicyId := "d9347b09-bb46-4e2b-91ea-513e83ab9ea7"
		By("Add an expired policy to the database")
		_, err := transportPostgreSQL.GetConn().Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.%s (id,cluster_name,leaf_hub_name,error,compliance) VALUES($1, $2, $3, $4, $5)`,
			testSchema, testTable), deletedPolicyId, "cluster1", leafHubName, "none", "unknown")
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() error {
			rows, err := transportPostgreSQL.GetConn().Query(ctx, fmt.Sprintf("SELECT id FROM %s.%s", testSchema, testTable))
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
				if policyId == "b8b3e164-477e-4be1-a870-992265f31f7d" {
					return nil
				}
			}
			return fmt.Errorf("failed to load content of table %s.%s", testSchema, testTable)
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())

		By("Build a new policy bundle in the regional hub")
		// policy bundle
		transportPayload := statusbundle.BaseClustersPerPolicyBundle{
			Objects:       make([]*statusbundle.PolicyGenericComplianceStatus, 0),
			LeafHubName:   leafHubName,
			BundleVersion: statusbundle.NewBundleVersion(1, 0),
		}

		transportPayload.Objects = append(transportPayload.Objects, &statusbundle.PolicyGenericComplianceStatus{
			PolicyID:                  createdPolicyId,
			CompliantClusters:         []string{"cluster1"},
			NonCompliantClusters:      []string{"cluster2"},
			UnknownComplianceClusters: make([]string, 0),
		})

		// transport bundle
		clustersPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.ClustersPerPolicyMsgKey)
		payloadBytes, err := json.Marshal(transportPayload)
		Expect(err).ToNot(HaveOccurred())

		By("Synchronize the latest policy bundle with transport")
		kafkaMessage, err := buildKafkaMessage(clustersPerPolicyTransportKey, clustersPerPolicyTransportKey, payloadBytes)
		Expect(err).ToNot(HaveOccurred())
		kafkaMessageChan <- kafkaMessage

		By("Check the latest policy is created and expired policy is deleted from database")
		Eventually(func() error {
			querySql := fmt.Sprintf("SELECT id,cluster_name,leaf_hub_name,compliance FROM %s.%s", testSchema, testTable)
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
			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, testTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})
})

func buildKafkaMessage(key, id string, payload []byte) (*kafka.Message, error) {
	transportMessage := &producer.Message{
		Key:     key,
		ID:      id, // entry.transportBundleKey
		MsgType: constants.StatusBundle,
		Version: "0.2", // entry.bundle.GetBundleVersion().String()
		Payload: payload,
	}
	transportMessageBytes, err := json.Marshal(transportMessage)
	if err != nil {
		return nil, err
	}

	compressor, err := compressor.NewCompressor(compressor.GZip)
	if err != nil {
		return nil, err
	}
	compressedTransportBytes, err := compressor.Compress(transportMessageBytes)
	if err != nil {
		return nil, err
	}

	topic := "status"
	kafkaMessage := &kafka.Message{
		Key: []byte(transportMessage.Key),
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: 0,
		},
		Headers: []kafka.Header{
			{Key: headers.CompressionType, Value: []byte(compressor.GetType())},
		},
		Value:         compressedTransportBytes,
		TimestampType: 1,
		Timestamp:     time.Now(),
	}
	return kafkaMessage, nil
}
