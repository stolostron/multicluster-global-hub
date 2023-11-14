package dbsyncer_test

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var _ = Describe("LocalStatusPoliciesSyncer", Ordered, func() {
	const (
		testSchema                     = database.EventSchema
		testEventTable                 = database.LocalPolicyEventTableName
		leafHubName                    = "hub1"
		localPoliciesStatusEventMsgKey = constants.LocalPolicyHistoryEventMsgKey
	)

	BeforeAll(func() {
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
		version := metadata.NewBundleVersion()
		version.Incr()
		baseClusterPolicyStatusEventBundle := base.BasePolicyHistoryEventBundle{
			ReplicasPolicyEvents: make(map[string][]*base.PolicyHistoryEvent),
			LeafHubName:          leafHubName,
			BundleVersion:        version,
		}
		lastTimestamp := time.Now()

		basePolicyEvent := &base.PolicyHistoryEvent{
			ClusterID:  "69369013-3e0e-4a9c-b38c-7efbe7770b61",
			PolicyID:   "f99c4252-bdde-43e9-9d3f-9bf0a5583543",
			Compliance: "Compliant",
			EventName:  "local-placement.policy-limitrange.176ccd711606e273",
			Message:    `Compliant; notification - limitranges [container-mem-limit-range] in namespace default found as specified, therefore this Object template is compliant`,
			CreatedAt:  lastTimestamp,
		}

		events := make([]*base.PolicyHistoryEvent, 0)
		baseClusterPolicyStatusEventBundle.ReplicasPolicyEvents["clusterPolicyId"] = append(events, basePolicyEvent)

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
				if eventName == basePolicyEvent.EventName {
					return nil
				}
			}
			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, testEventTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})
})
