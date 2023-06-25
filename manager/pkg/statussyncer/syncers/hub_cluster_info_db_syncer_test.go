package dbsyncer_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	routev1 "github.com/openshift/api/route/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle/hubcluster"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var _ = Describe("HubClusterInfoDbSyncer", Ordered, func() {
	const (
		leafHubName = "hub1"
		routeHost   = "console-openshift-console.apps.test-cluster"
		testSchema  = database.StatusSchema
		testTable   = database.HubClusterInfoTableName
		messageKey  = constants.HubClusterInfoMsgKey
	)

	BeforeAll(func() {
		By("Create leaf_hubs table in database")
		_, err := transportPostgreSQL.GetConn().Exec(ctx, `
			CREATE SCHEMA IF NOT EXISTS status;
			CREATE TABLE IF NOT EXISTS status.leaf_hubs (
				leaf_hub_name character varying(63) NOT NULL,
				payload jsonb NOT NULL,
				console_url text generated always as (payload -> 'consoleURL') stored,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted_at timestamp without time zone
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
				if schema == testSchema && table == testTable {
					return nil
				}
			}
			return fmt.Errorf("failed to create table %s.%s", testSchema, testTable)
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})

	It("sync the hubClusterInfo bundle", func() {
		By("Create hubClusterInfo bundle")
		obj := &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.OpenShiftConsoleRouteName,
				Namespace: constants.OpenShiftConsoleNamespace,
			},
			Spec: routev1.RouteSpec{
				Host: routeHost,
			},
		}
		statusBundle := hubcluster.NewLeafHubClusterInfoStatusBundle(leafHubName, 0)
		statusBundle.UpdateObject(obj)
		By("Create transport message")
		// increment the version
		payloadBytes, err := json.Marshal(statusBundle)
		Expect(err).ShouldNot(HaveOccurred())

		transportMessageKey := fmt.Sprintf("%s.%s", leafHubName, messageKey)
		transportMessage := &transport.Message{
			Key:     transportMessageKey,
			ID:      transportMessageKey,
			MsgType: constants.StatusBundle,
			Version: statusBundle.GetBundleVersion().String(),
			Payload: payloadBytes,
		}

		By("Sync message with transport")
		err = producer.Send(ctx, transportMessage)
		Expect(err).Should(Succeed())

		By("Check the leaf hubs table")
		Eventually(func() error {
			querySql := fmt.Sprintf("SELECT leaf_hub_name,console_url FROM %s.%s", testSchema, testTable)
			rows, err := transportPostgreSQL.GetConn().Query(ctx, querySql)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var hubName, consoleURL string
				if err := rows.Scan(&hubName, &consoleURL); err != nil {
					return err
				}
				if hubName == leafHubName && strings.Contains(consoleURL, routeHost) {
					return nil
				}
			}
			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, testTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})
})
