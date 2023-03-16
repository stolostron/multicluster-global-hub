package controller_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsubv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var _ = Describe("subscriptions to database controller", func() {
	const testSchema = "spec"
	const testTable = "subscriptions"

	BeforeEach(func() {
		By("Creating test table in the database")
		_, err := postgresSQL.GetConn().Exec(ctx, `
			CREATE SCHEMA IF NOT EXISTS spec;
			CREATE TABLE IF NOT EXISTS  spec.subscriptions (
				id uuid NOT NULL,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted boolean DEFAULT false NOT NULL
			);
		`)
		Expect(err).ToNot(HaveOccurred())

		By("Check whether the table is created")
		Eventually(func() error {
			rows, err := postgresSQL.GetConn().Query(ctx, "SELECT * FROM pg_tables")
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
			return fmt.Errorf("failed to create test table %s.%s", testSchema, testTable)
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})

	It("filter subscription with MCH OwnerReferences", func() {
		By("Create subscription sub1 instance with OwnerReference")
		filteredSubscription := &appsubv1.Subscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sub1",
				Namespace: config.GetDefaultNamespace(),
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
		Expect(kubeClient.Create(ctx, filteredSubscription)).Should(Succeed())

		By("Create channel sub2 instance without OwnerReference")
		expectedSubscription := &appsubv1.Subscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sub2",
				Namespace: config.GetDefaultNamespace(),
				Labels: map[string]string{
					constants.GlobalHubGlobalResourceLabel: "",
				},
			},
			Spec: appsubv1.SubscriptionSpec{
				Channel: "test-channel",
			},
		}
		Expect(kubeClient.Create(ctx, expectedSubscription)).Should(Succeed())

		Eventually(func() error {
			expectedSubscriptionSynced := false
			rows, err := postgresSQL.GetConn().Query(ctx,
				fmt.Sprintf("SELECT payload FROM %s.%s", testSchema, testTable))
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				syncedSubscription := &appsubv1.Subscription{}
				if err := rows.Scan(syncedSubscription); err != nil {
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
