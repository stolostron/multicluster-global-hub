package dbsyncer_test

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var _ = Describe("LocalStatusPoliciesSyncer", Ordered, func() {
	const (
		testSchema                     = database.EventSchema
		testEventTable                 = database.LocalPolicyEventTableName
		leafHubName                    = "hub1"
		localPoliciesStatusEventMsgKey = constants.LocalClusterPolicyStatusEventMsgKey
	)

	BeforeAll(func() {
		By("Create local spec table in database")
		_, err := transportPostgreSQL.GetConn().Exec(ctx, `
			CREATE SCHEMA IF NOT EXISTS event;
			CREATE SCHEMA IF NOT EXISTS local_status;
			DO $$ BEGIN
				CREATE TYPE local_status.compliance_type AS ENUM (
					'compliant',
					'non_compliant',
					'unknown'
				);
			EXCEPTION
				WHEN duplicate_object THEN null;
			END $$;
			CREATE TABLE IF NOT EXISTS event.local_policies (
				event_name character varying(63) NOT NULL,
				policy_id uuid NOT NULL,
				cluster_id uuid NOT NULL,
				leaf_hub_name character varying(63) NOT NULL,
				message text,
				reason text,
				count integer NOT NULL DEFAULT 0,
				source jsonb,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				compliance local_status.compliance_type NOT NULL,
				CONSTRAINT local_policies_unique_constraint UNIQUE (event_name, count)
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
			for rows.Next() {
				columnValues, _ := rows.Values()
				schema := columnValues[0]
				table := columnValues[1]
				if schema == testSchema && table == testEventTable {
					return nil
				}
			}
			return fmt.Errorf("failed to create table %s.%s", testSchema, testEventTable)
		}, 20*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})

	It("sync ClusterPolicyStatusEventBundle to database", func() {
		By("Create ClusterPolicyStatusEventBundle")
		baseClusterPolicyStatusEventBundle := status.BaseClusterPolicyStatusEventBundle{
			PolicyStatusEvents: make(map[string][]*status.PolicyStatusEvent),
			LeafHubName:        leafHubName,
			BundleVersion:      status.NewBundleVersion(1, 0),
		}
		lastTimestamp := metav1.NewTime(time.Now())
		policyEvent := &status.PolicyStatusEvent{
			EventName:     "local-placement.policy-limitrange.176ccd711606e273",
			PolicyID:      "f99c4252-bdde-43e9-9d3f-9bf0a5583543",
			ClusterID:     "69369013-3e0e-4a9c-b38c-7efbe7770b61",
			Compliance:    "Compliant",
			Message:       `Compliant; notification - limitranges [container-mem-limit-range] in namespace default found as specified, therefore this Object template is compliant`,
			LastTimestamp: lastTimestamp,
		}

		events := make([]*status.PolicyStatusEvent, 0)
		baseClusterPolicyStatusEventBundle.PolicyStatusEvents["clusterPolicyId"] = append(events, policyEvent)
		baseClusterPolicyStatusEventBundle.BundleVersion.Generation++

		By("Create transport message")
		payloadBytes, err := json.Marshal(baseClusterPolicyStatusEventBundle)
		Expect(err).ShouldNot(HaveOccurred())

		transportMessageKey := fmt.Sprintf("%s.%s", leafHubName, localPoliciesStatusEventMsgKey)
		transportMessage := &transport.Message{
			Key:     transportMessageKey,
			ID:      transportMessageKey,
			MsgType: constants.StatusBundle,
			Version: baseClusterPolicyStatusEventBundle.BundleVersion.String(),
			Payload: payloadBytes,
		}

		By("Sync message with transport")
		err = producer.Send(ctx, transportMessage)
		Expect(err).Should(Succeed())

		By("Check the local policy event table")
		Eventually(func() error {
			querySql := fmt.Sprintf("SELECT event_name FROM %s.%s", testSchema, testEventTable)
			rows, err := transportPostgreSQL.GetConn().Query(ctx, querySql)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var eventName string
				if err := rows.Scan(&eventName); err != nil {
					return err
				}
				if eventName == policyEvent.EventName {
					return nil
				}
			}
			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, testEventTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})
})
