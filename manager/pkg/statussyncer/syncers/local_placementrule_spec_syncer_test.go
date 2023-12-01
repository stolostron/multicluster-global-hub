package dbsyncer_test

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var _ = Describe("LocalPlacementrulesSyncer", Label("localplacementrule"), func() {
	const (
		testSchema                   = database.LocalSpecSchema
		testPlacementRuleTable       = database.PlacementRulesTableName
		placementRuleTable           = "placementrules"
		leafHubName                  = "hub1"
		localPlacementRuleMessageKey = constants.LocalPlacementRulesMsgKey
	)

	It("sync LocalPlacementRuleBundle to database", func() {
		By("Create LocalPlacementRuleBundle")
		placementrule := &placementrulev1.PlacementRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-placementrule-1",
				Namespace: "default",
				UID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			},
			Spec: placementrulev1.PlacementRuleSpec{
				SchedulerName: constants.GlobalHubSchedulerName,
			},
		}

		By("Create transport message")
		statusBundle := generic.NewGenericStatusBundle(leafHubName, nil)
		statusBundle.UpdateObject(placementrule)
		payloadBytes, err := json.Marshal(statusBundle)
		Expect(err).ShouldNot(HaveOccurred())

		transportMessageKey := fmt.Sprintf("%s.%s", leafHubName, localPlacementRuleMessageKey)
		transportMessage := &transport.Message{
			Key:     transportMessageKey,
			ID:      transportMessageKey,
			MsgType: constants.StatusBundle,
			Version: statusBundle.GetVersion().String(),
			Payload: payloadBytes,
		}
		By("Sync message with transport")
		err = producer.Send(ctx, transportMessage)
		Expect(err).Should(Succeed())

		By("Check the local placementrule table")
		Eventually(func() error {
			querySql := fmt.Sprintf("SELECT leaf_hub_name,payload FROM %s.%s", testSchema, testPlacementRuleTable)
			rows, err := transportPostgreSQL.GetConn().Query(ctx, querySql)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var receivedHubName string
				receivedPlacementrule := placementrulev1.PlacementRule{}
				if err := rows.Scan(&receivedHubName, &receivedPlacementrule); err != nil {
					return err
				}
				if receivedHubName == leafHubName && receivedPlacementrule.Name == placementrule.Name {
					fmt.Printf("LocalSpecPlacementrule: %s - %s/%s\n", receivedHubName, receivedPlacementrule.Namespace, receivedPlacementrule.Name)
					return nil
				}
			}

			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, testPlacementRuleTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})
})
