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

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var _ = Describe("LocalPolicySpecSyncer", Label("localpolicy"), func() {
	const (
		testSchema            = database.LocalSpecSchema
		testPolicyTable       = database.LocalPolicySpecTableName
		leafHubName           = "hub1"
		localPolicyMessageKey = constants.LocalPolicySpecMsgKey
	)

	It("sync LocalPolicyBundle to database", func() {
		By("Create LocalPolicyBundle")
		policy := &policiesv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testLocalPolicy",
				Namespace: "default",
				UID:       types.UID(uuid.New().String()),
			},
			Spec: policiesv1.PolicySpec{},
		}
		statusBundle := generic.NewGenericStatusBundle(leafHubName, nil)
		statusBundle.UpdateObject(policy)

		By("Create transport message")
		payloadBytes, err := json.Marshal(statusBundle)
		Expect(err).ShouldNot(HaveOccurred())

		transportMessageKey := fmt.Sprintf("%s.%s", leafHubName, localPolicyMessageKey)
		transportMessage := &transport.Message{
			Key:     transportMessageKey,
			MsgType: constants.StatusBundle,
			Version: statusBundle.GetVersion().String(),
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
				var receivedHubName string
				receivedPolicy := policiesv1.Policy{}
				if err := rows.Scan(&receivedHubName, &receivedPolicy); err != nil {
					return err
				}
				if receivedHubName == leafHubName && receivedPolicy.Name == policy.Name {
					fmt.Printf("LocalSpecPolicy: %s - %s/%s\n", receivedHubName, receivedPolicy.Namespace, receivedPolicy.Name)
					return nil
				}
			}

			return fmt.Errorf("failed to sync content of table %s.%s", testSchema, testPolicyTable)
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
