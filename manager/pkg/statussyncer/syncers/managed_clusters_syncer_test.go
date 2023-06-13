package dbsyncer_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/helper"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var _ = Describe("ManagedClustersDbSyncer", Ordered, func() {
	const (
		leafHubName = "hub1"
		testSchema  = database.StatusSchema
		testTable   = database.ManagedClustersTableName
		messageKey  = constants.ManagedClustersMsgKey
	)

	BeforeAll(func() {
		By("Create managed_clusters table in database")
		_, err := transportPostgreSQL.GetConn().Exec(ctx, `
			CREATE SCHEMA IF NOT EXISTS status;
			DO $$ BEGIN
				CREATE TYPE status.error_type AS ENUM (
					'disconnected',
					'none'
				);
			EXCEPTION
				WHEN duplicate_object THEN null;
			END $$;
			CREATE TABLE IF NOT EXISTS status.managed_clusters (
				leaf_hub_name character varying(63) NOT NULL,
				cluster_name character varying(63) generated always as (payload -> 'metadata' ->> 'name') stored,
				cluster_id uuid NOT NULL,
				payload jsonb NOT NULL,
				error status.error_type NOT NULL,
				created_at timestamp without time zone,
				updated_at timestamp without time zone,
				deleted_at timestamp without time zone
			);
			CREATE UNIQUE INDEX IF NOT EXISTS managed_clusters_leaf_hub_name_metadata_uid_idx ON status.managed_clusters USING btree (leaf_hub_name, (((payload -> 'metadata'::text) ->> 'uid'::text)));
			CREATE INDEX IF NOT EXISTS managed_clusters_metadata_name_idx ON status.managed_clusters USING btree ((((payload -> 'metadata'::text) ->> 'name'::text)));
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

	It("sync the ManagedCluster bundle", func() {
		By("Create ManagedCluster bundle")
		cluster := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testManagedCluster",
				Namespace: "default",
			},
			Status: clusterv1.ManagedClusterStatus{
				ClusterClaims: []clusterv1.ManagedClusterClaim{
					{
						Name:  "id.k8s.io",
						Value: "3f406177-34b2-4852-88dd-ff2809680335",
					},
				},
			},
		}
		statusBundle := &GenericStatusBundle{
			Objects:       make([]Object, 0),
			LeafHubName:   leafHubName,
			BundleVersion: status.NewBundleVersion(0, 0),
			manipulateObjFunc: func(object Object) {
				helper.AddAnnotations(object, map[string]string{
					constants.ManagedClusterManagedByAnnotation: leafHubName,
				})
			},
			lock: sync.Mutex{},
		}
		statusBundle.Objects = append(statusBundle.Objects, cluster)

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
		err = producer.Send(ctx, transportMessage)
		Expect(err).Should(Succeed())

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
				cluster := clusterv1.ManagedCluster{}
				if err := rows.Scan(&hubName, &cluster); err != nil {
					return err
				}
				if hubName == statusBundle.LeafHubName &&
					cluster.Name == statusBundle.Objects[0].GetName() {
					return nil
				}
			}

			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, testTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})
})
