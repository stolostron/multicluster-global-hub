package dbsyncer_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	clustersv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
)

var _ = Describe("PlacementDecisionsDbSyncer", Ordered, func() {
	const (
		leafHubName = "hub1"
		testSchema  = database.StatusSchema
		testTable   = database.PlacementDecisionsTableName
		messageKey  = constants.PlacementDecisionMsgKey
	)

	BeforeAll(func() {
		By("Create leaf_hub_heartbeats table in database")
		_, err := transportPostgreSQL.GetConn().Exec(ctx, `
			CREATE SCHEMA IF NOT EXISTS status;
			CREATE TABLE IF NOT EXISTS  status.placementdecisions (
				id uuid NOT NULL,
				leaf_hub_name character varying(63) NOT NULL,
				payload jsonb NOT NULL
			);
			CREATE UNIQUE INDEX IF NOT EXISTS placementdecisions_leaf_hub_name_and_payload_id_namespace_idx ON status.placementdecisions USING btree (leaf_hub_name, id, (((payload -> 'metadata'::text) ->> 'namespace'::text)));
			CREATE INDEX IF NOT EXISTS placementdecisions_payload_name_and_namespace_idx ON status.placementdecisions USING btree ((((payload -> 'metadata'::text) ->> 'name'::text)), (((payload -> 'metadata'::text) ->> 'namespace'::text)));
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
				if schema == testSchema && table == testTable {
					return nil
				}
			}
			return fmt.Errorf("failed to create table %s.%s", testSchema, testTable)
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})

	It("sync the PlacementDecision bundle", func() {
		By("Create PlacementDecision bundle")
		obj := &clustersv1beta1.PlacementDecision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testPlacementDecision",
				Namespace: "default",
				Annotations: map[string]string{
					constants.OriginOwnerReferenceAnnotation: "2aa5547c-c172-47ed-b70b-db468c84d327",
				},
			},
		}
		statusBundle := &GenericStatusBundle{
			Objects:           make([]Object, 0),
			LeafHubName:       leafHubName,
			BundleVersion:     status.NewBundleVersion(0, 0),
			manipulateObjFunc: nil,
			lock:              sync.Mutex{},
		}
		statusBundle.Objects = append(statusBundle.Objects, obj)

		By("Create transport message")
		// increment the version
		statusBundle.BundleVersion.Generation++
		payloadBytes, err := json.Marshal(statusBundle)
		Expect(err).ShouldNot(HaveOccurred())

		transportMessageKey := fmt.Sprintf("%s.%s", leafHubName, messageKey)
		transportMessage := &transport.Message{
			Key:     transportMessageKey,
			ID:      transportMessageKey,
			MsgType: constants.StatusBundle,
			Version: statusBundle.BundleVersion.String(),
			Payload: payloadBytes,
		}

		By("Sync message with transport")
		kafkaProducer.SendAsync(transportMessage)

		By("Check the managed cluster table")
		Eventually(func() error {
			querySql := fmt.Sprintf("SELECT leaf_hub_name,payload FROM %s.%s", testSchema, testTable)
			rows, err := transportPostgreSQL.GetConn().Query(ctx, querySql)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var hubName string
				placementDecision := clustersv1beta1.PlacementDecision{}
				if err := rows.Scan(&hubName, &placementDecision); err != nil {
					return err
				}
				if hubName == statusBundle.LeafHubName &&
					placementDecision.Name == statusBundle.Objects[0].GetName() {
					return nil
				}
			}
			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, testTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})
})
