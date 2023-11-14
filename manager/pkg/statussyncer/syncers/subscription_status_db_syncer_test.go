package dbsyncer_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var _ = Describe("SubscriptionStatusDbSyncer", Ordered, func() {
	const (
		leafHubName = "hub1"
		testSchema  = database.StatusSchema
		testTable   = database.SubscriptionStatusesTableName
		messageKey  = constants.SubscriptionStatusMsgKey
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

	It("sync the SubscriptionStatus bundle", func() {
		By("Create SubscriptionStatus bundle")
		obj := &appsv1alpha1.SubscriptionStatus{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testAppStatus",
				Namespace: "default",
				Annotations: map[string]string{
					constants.OriginOwnerReferenceAnnotation: "2aa5547c-c172-47ed-b70b-db468c84d327",
				},
			},
		}
		statusBundle := &GenericStatusBundle{
			Objects:           make([]Object, 0),
			LeafHubName:       leafHubName,
			BundleVersion:     metadata.NewBundleVersion(),
			manipulateObjFunc: nil,
			lock:              sync.Mutex{},
		}
		statusBundle.Objects = append(statusBundle.Objects, obj)

		By("Create transport message")
		// increment the version
		statusBundle.BundleVersion.Incr()
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
				appsubStatus := &appsv1alpha1.SubscriptionStatus{}
				if err := rows.Scan(&hubName, &appsubStatus); err != nil {
					return err
				}
				if hubName == statusBundle.LeafHubName &&
					appsubStatus.Name == statusBundle.Objects[0].GetName() {
					return nil
				}
			}
			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, testTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})
})
