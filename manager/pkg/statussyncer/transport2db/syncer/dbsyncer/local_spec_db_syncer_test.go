package dbsyncer_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

var _ = Describe("LocalSpecDbSyncer", Ordered, func() {
	const (
		testSchema         = "local_spec"
		policyTable        = "policies"
		placementRuleTable = "placementrules"
		leafHubName        = "hub1"
	)

	BeforeAll(func() {
		By("Create leaf_hub_heartbeats table in database")
		_, err := transportPostgreSQL.GetConn().Exec(ctx, `
			CREATE SCHEMA IF NOT EXISTS local_spec;
			CREATE TABLE IF NOT EXISTS  local_spec.placementrules (
				leaf_hub_name text,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL
			);
			CREATE TABLE IF NOT EXISTS  local_spec.policies (
				leaf_hub_name text,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL
			);
			CREATE UNIQUE INDEX IF NOT EXISTS placementrules_leaf_hub_name_id_idx ON local_spec.placementrules USING btree (leaf_hub_name, (((payload -> 'metadata'::text) ->> 'uid'::text)));
			CREATE UNIQUE INDEX IF NOT EXISTS policies_leaf_hub_name_id_idx ON local_spec.policies USING btree (leaf_hub_name, (((payload -> 'metadata'::text) ->> 'uid'::text)));
		`)
		Expect(err).ToNot(HaveOccurred())

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
				if schema == testSchema && (table == policyTable || table == placementRuleTable) {
					expectedCount++
				}
			}
			if expectedCount == 2 {
				return nil
			}
			return fmt.Errorf("failed to create table %s.%s and %s.%s", testSchema, policyTable,
				testSchema, placementRuleTable)
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})

	It("sync the local policy bundle", func() {
		By("Create localPolicySpec bundle")
		policy := &policiesv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testLocalPolicy",
				Namespace: "default",
			},
			Spec: policiesv1.PolicySpec{},
		}
		localPolicyStatusBundle := &GenericStatusBundle{
			Objects:       make([]Object, 0),
			LeafHubName:   leafHubName,
			BundleVersion: status.NewBundleVersion(0, 0),
			manipulateObjFunc: func(object Object) {
				policy, ok := object.(*policiesv1.Policy)
				if !ok {
					panic("Wrong instance passed to clean policy function, not a Policy")
				}
				policy.Status = policiesv1.PolicyStatus{}
			},
			lock: sync.Mutex{},
		}
		localPolicyStatusBundle.Objects = append(localPolicyStatusBundle.Objects, policy)

		By("Create transport message")
		// increment the version
		localPolicyStatusBundle.BundleVersion.Generation++
		payloadBytes, err := json.Marshal(localPolicyStatusBundle)
		Expect(err).ShouldNot(HaveOccurred())

		localPolicySpecTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.LocalPolicySpecMsgKey)
		transportMessage := &transport.Message{
			Key:     localPolicySpecTransportKey,
			ID:      localPolicySpecTransportKey,
			MsgType: constants.StatusBundle,
			Version: localPolicyStatusBundle.BundleVersion.String(),
			Payload: payloadBytes,
		}

		By("Sync message with transport")
		kafkaProducer.SendAsync(transportMessage)

		By("Check the local policy table")
		Eventually(func() error {
			querySql := fmt.Sprintf("SELECT leaf_hub_name,payload FROM %s.%s", testSchema, policyTable)
			rows, err := transportPostgreSQL.GetConn().Query(ctx, querySql)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var leaf_hub_name string
				policy := policiesv1.Policy{}
				if err := rows.Scan(&leaf_hub_name, &policy); err != nil {
					return err
				}
				if leaf_hub_name == localPolicyStatusBundle.LeafHubName &&
					policy.Name == localPolicyStatusBundle.Objects[0].GetName() {
					return nil
				}
			}

			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, policyTable)
		}, 30*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})
})

type GenericStatusBundle struct {
	Objects           []Object              `json:"objects"`
	LeafHubName       string                `json:"leafHubName"`
	BundleVersion     *status.BundleVersion `json:"bundleVersion"`
	manipulateObjFunc func(obj Object)
	lock              sync.Mutex
}
type Object interface {
	metav1.Object
	runtime.Object
}
