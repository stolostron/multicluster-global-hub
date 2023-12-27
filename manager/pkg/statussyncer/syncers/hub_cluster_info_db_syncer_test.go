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
	clustersv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
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
		route := &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.OpenShiftConsoleRouteName,
				Namespace: constants.OpenShiftConsoleNamespace,
			},
			Spec: routev1.RouteSpec{
				Host: routeHost,
			},
		}

		// create bundle
		statusBundle := cluster.NewAgentHubClusterInfoBundle(leafHubName)

		// update bundle with route handler
		routeHandler := cluster.NewHubClusterInfoRouteObject()
		routeHandler.BundleUpdate(route, statusBundle)

		claim := &clustersv1alpha1.ClusterClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "id.k8s.io",
			},
			Spec: clustersv1alpha1.ClusterClaimSpec{
				Value: "00000000-0000-0000-0000-000000000001",
			},
		}
		calimHandler := cluster.NewHubClusterInfoClaimObject()
		calimHandler.BundleUpdate(claim, statusBundle)

		By("Create transport message")
		payloadBytes, err := json.Marshal(statusBundle)
		Expect(err).ShouldNot(HaveOccurred())

		transportMessageKey := fmt.Sprintf("%s.%s", leafHubName, messageKey)
		transportMessage := &transport.Message{
			Key:     transportMessageKey,
			MsgType: constants.StatusBundle,
			Payload: payloadBytes,
		}

		By("Sync message with transport")
		err = producer.Send(ctx, transportMessage)
		Expect(err).Should(Succeed())

		By("Check the leaf hubs table")
		Eventually(func() error {
			querySql := fmt.Sprintf("SELECT leaf_hub_name,console_url,cluster_id FROM %s.%s", testSchema, testTable)
			rows, err := transportPostgreSQL.GetConn().Query(ctx, querySql)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var receivedHubName, receivedConsoleURL, receivedClusterID string
				if err := rows.Scan(&receivedHubName, &receivedConsoleURL, &receivedClusterID); err != nil {
					return err
				}
				if receivedHubName == leafHubName && strings.Contains(receivedConsoleURL, routeHost) &&
					receivedClusterID == claim.Spec.Value {
					return nil
				}
			}
			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, testTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})
})
