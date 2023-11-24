package dbsyncer_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var _ = Describe("LocalSpecDbSyncer", Ordered, func() {
	const (
		testSchema                   = database.LocalSpecSchema
		testPolicyTable              = database.LocalPolicySpecTableName
		testPlacementRuleTable       = database.PlacementRulesTableName
		placementRuleTable           = "placementrules"
		leafHubName                  = "hub1"
		localPolicyMessageKey        = constants.LocalPolicySpecMsgKey
		localPlacementRuleMessageKey = constants.LocalPlacementRulesMsgKey
	)

	BeforeAll(func() {
		By("Check whether the tables are created")
		Eventually(func() error {
			rows, err := transportPostgreSQL.GetConn().Query(ctx, "SELECT * FROM pg_tables")
			if err != nil {
				return err
			}
			defer rows.Close()
			expectedCount := 0
			for rows.Next() {
				columnValues, _ := rows.Values()
				schema := columnValues[0]
				table := columnValues[1]
				if schema == testSchema && (table == testPolicyTable || table == placementRuleTable) {
					expectedCount++
				}
			}
			if expectedCount == 2 {
				return nil
			}
			return fmt.Errorf("failed to create table %s.%s and %s.%s", testSchema, testPolicyTable,
				testSchema, placementRuleTable)
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})

	It("sync LocalPolicyBundle to database", func() {
		version := metadata.NewBundleVersion()
		By("Create LocalPolicyBundle")
		policy := &policiesv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testLocalPolicy",
				Namespace: "default",
				UID:       types.UID(uuid.New().String()),
			},
			Spec: policiesv1.PolicySpec{},
		}
		statusBundle := &GenericStatusBundle{
			Objects:       make([]Object, 0),
			LeafHubName:   leafHubName,
			BundleVersion: version,
			manipulateObjFunc: func(object Object) {
				policy, ok := object.(*policiesv1.Policy)
				if !ok {
					panic("Wrong instance passed to clean policy function, not a Policy")
				}
				policy.Status = policiesv1.PolicyStatus{}
			},
			lock: sync.Mutex{},
		}
		statusBundle.Objects = append(statusBundle.Objects, policy)

		By("Create transport message")
		// increment the version
		statusBundle.BundleVersion.Incr()

		payloadBytes, err := json.Marshal(statusBundle)
		Expect(err).ShouldNot(HaveOccurred())

		transportMessageKey := fmt.Sprintf("%s.%s", leafHubName, localPolicyMessageKey)
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

		By("Check the local policy table")
		Eventually(func() error {
			querySql := fmt.Sprintf("SELECT leaf_hub_name,payload FROM %s.%s", testSchema, testPolicyTable)
			rows, err := transportPostgreSQL.GetConn().Query(ctx, querySql)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var hubName string
				policy := policiesv1.Policy{}
				if err := rows.Scan(&hubName, &policy); err != nil {
					return err
				}
				if hubName == statusBundle.LeafHubName && policy.Name == statusBundle.Objects[0].GetName() {
					fmt.Printf("LocalSpecPolicy: %s - %s/%s\n", hubName, policy.Namespace, policy.Name)
					return nil
				}
			}

			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, testPolicyTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})

	It("sync LocalPlacementRuleBundle to database", func() {
		By("Create LocalPlacementRuleBundle")
		testPlacementrule := &placementrulev1.PlacementRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-placementrule-1",
				Namespace: "default",
				UID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			},
			Spec: placementrulev1.PlacementRuleSpec{
				SchedulerName: constants.GlobalHubSchedulerName,
			},
		}
		statusBundle := &GenericStatusBundle{
			Objects:       make([]Object, 0),
			LeafHubName:   leafHubName,
			BundleVersion: metadata.NewBundleVersion(),
			manipulateObjFunc: func(object Object) {
				placementRule, ok := object.(*placementrulev1.PlacementRule)
				if !ok {
					panic("Wrong instance passed to clean policy function, not a Policy")
				}
				placementRule.Status = placementrulev1.PlacementRuleStatus{}
			},
			lock: sync.Mutex{},
		}
		statusBundle.Objects = append(statusBundle.Objects, testPlacementrule)

		By("Create transport message")
		// increment the version
		statusBundle.BundleVersion.Incr()
		payloadBytes, err := json.Marshal(statusBundle)
		Expect(err).ShouldNot(HaveOccurred())

		transportMessageKey := fmt.Sprintf("%s.%s", leafHubName, localPlacementRuleMessageKey)
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

		By("Check the local placementrule table")
		Eventually(func() error {
			querySql := fmt.Sprintf("SELECT leaf_hub_name,payload FROM %s.%s", testSchema, testPlacementRuleTable)
			rows, err := transportPostgreSQL.GetConn().Query(ctx, querySql)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var hubName string
				placementrule := placementrulev1.PlacementRule{}
				if err := rows.Scan(&hubName, &placementrule); err != nil {
					return err
				}
				if hubName == statusBundle.LeafHubName && placementrule.Name == statusBundle.Objects[0].GetName() {
					fmt.Printf("LocalSpecPlacementrule: %s - %s/%s\n", hubName, placementrule.Namespace, placementrule.Name)
					return nil
				}
			}

			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, testPlacementRuleTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})
})

type GenericStatusBundle struct {
	Objects           []Object                `json:"objects"`
	LeafHubName       string                  `json:"leafHubName"`
	BundleVersion     *metadata.BundleVersion `json:"bundleVersion"`
	manipulateObjFunc func(obj Object)
	lock              sync.Mutex
}
type Object interface {
	metav1.Object
	runtime.Object
}
