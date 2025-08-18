package spec

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsubv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ = Describe("subscriptions to database controller", func() {
	const testSchema = "spec"
	const testTable = "subscriptions"

	It("filter subscription with MCH OwnerReferences", func() {
		By("Create subscription sub1 instance with OwnerReference")
		filteredSubscription := &appsubv1.Subscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sub1",
				Namespace: utils.GetDefaultNamespace(),
				Labels: map[string]string{
					constants.GlobalHubGlobalResourceLabel: "",
				},
			},
			Spec: appsubv1.SubscriptionSpec{
				Channel: "test-channel",
			},
		}
		Expect(controllerutil.SetControllerReference(multiclusterhub, filteredSubscription,
			mgr.GetScheme())).NotTo(HaveOccurred())
		Expect(runtimeClient.Create(ctx, filteredSubscription)).Should(Succeed())

		By("Create channel sub2 instance without OwnerReference")
		expectedSubscription := &appsubv1.Subscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sub2",
				Namespace: utils.GetDefaultNamespace(),
				Labels: map[string]string{
					constants.GlobalHubGlobalResourceLabel: "",
				},
			},
			Spec: appsubv1.SubscriptionSpec{
				Channel: "test-channel",
			},
		}
		Expect(runtimeClient.Create(ctx, expectedSubscription)).Should(Succeed())

		Eventually(func() error {
			expectedSubscriptionSynced := false
			rows, err := database.GetGorm().Raw(fmt.Sprintf("SELECT payload FROM %s.%s", testSchema, testTable)).Rows()
			if err != nil {
				return err
			}
			defer func() {
				if err := rows.Close(); err != nil {
					fmt.Printf("failed to close rows: %v\n", err)
				}
			}()
			for rows.Next() {
				syncedSubscription := &appsubv1.Subscription{}
				var payload []byte
				err := rows.Scan(&payload)
				if err != nil {
					return err
				}
				if err := json.Unmarshal(payload, syncedSubscription); err != nil {
					return err
				}
				fmt.Printf("spec.subscriptions: %s - %s \n",
					syncedSubscription.Namespace, syncedSubscription.Name)
				if syncedSubscription.GetNamespace() == expectedSubscription.GetNamespace() &&
					syncedSubscription.GetName() == expectedSubscription.GetName() {
					expectedSubscriptionSynced = true
				}
				if syncedSubscription.GetNamespace() == filteredSubscription.GetNamespace() &&
					syncedSubscription.GetName() == filteredSubscription.GetName() {
					return fmt.Errorf("subscription(%s) with OwnerReference(MCH) should't be synchronized to database",
						filteredSubscription.GetName())
				}
			}
			if expectedSubscriptionSynced {
				return nil
			} else {
				return fmt.Errorf("not find channel(%s) in database", expectedSubscription.GetName())
			}
		}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
	})
})
