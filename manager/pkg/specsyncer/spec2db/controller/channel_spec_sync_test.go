package controller_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	channelv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var _ = Describe("channels to database controller", func() {
	const testSchema = "spec"
	const testTable = "channels"

	BeforeEach(func() {
		By("Creating test table in the database")
		_, err := postgresSQL.GetConn().Exec(ctx, `
			CREATE SCHEMA IF NOT EXISTS spec;
			CREATE TABLE IF NOT EXISTS  spec.channels (
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

	It("filter channel with MCH OwnerReferences", func() {
		By("Create channel ch1 instance with OwnerReference")
		filteredChannel := &channelv1.Channel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ch1",
				Namespace: config.GetDefaultNamespace(),
			},
			Spec: channelv1.ChannelSpec{
				Type:     channelv1.ChannelTypeGit,
				Pathname: "test-path",
			},
		}
		Expect(controllerutil.SetControllerReference(multiclusterhub, filteredChannel,
			mgr.GetScheme())).NotTo(HaveOccurred())
		Expect(kubeClient.Create(ctx, filteredChannel)).Should(Succeed())

		By("Create channel ch2 instance without OwnerReference")
		expectedChannel := &channelv1.Channel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ch2",
				Namespace: config.GetDefaultNamespace(),
				Labels:    map[string]string{constants.GlobalHubGlobalResourceLabel: ""},
			},
			Spec: channelv1.ChannelSpec{
				Type:     channelv1.ChannelTypeGit,
				Pathname: "test-path",
			},
		}
		Expect(kubeClient.Create(ctx, expectedChannel)).Should(Succeed())

		Eventually(func() error {
			expectedChannelSynced := false
			rows, err := postgresSQL.GetConn().Query(ctx,
				fmt.Sprintf("SELECT payload FROM %s.%s", testSchema, testTable))
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				syncedChannel := &channelv1.Channel{}
				if err := rows.Scan(syncedChannel); err != nil {
					return err
				}
				fmt.Printf("spec.channels: %s - %s \n", syncedChannel.Namespace, syncedChannel.Name)
				if syncedChannel.GetNamespace() == expectedChannel.GetNamespace() &&
					syncedChannel.GetName() == expectedChannel.GetName() {
					expectedChannelSynced = true
				}
				if syncedChannel.GetNamespace() == filteredChannel.GetNamespace() &&
					syncedChannel.GetName() == filteredChannel.GetName() {
					return fmt.Errorf("channel(%s) with OwnerReference(MCH) should't be synchronized to database",
						filteredChannel.GetName())
				}
			}
			if expectedChannelSynced {
				return nil
			} else {
				return fmt.Errorf("not find channel(%s) in database", expectedChannel.GetName())
			}
		}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
	})
})
