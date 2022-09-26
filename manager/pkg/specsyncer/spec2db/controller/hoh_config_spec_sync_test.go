package controller_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	commonconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var _ = Describe("configmaps to database controller", func() {
	const testSchema = "spec"
	const testTable = "configs"

	BeforeEach(func() {
		By("Creating test table in the database")
		_, err := postgresSQL.GetConn().Exec(ctx, `
			CREATE SCHEMA IF NOT EXISTS spec;
			CREATE TABLE IF NOT EXISTS  spec.configs (
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
		}, 1*time.Second).ShouldNot(HaveOccurred())
	})

	It("synchronize ConfigMap instance to database", func() {
		By("Create Namespace for ConfigMap instance")
		Eventually(func() error {
			err := kubeClient.Get(ctx, types.NamespacedName{
				Name: constants.HOHSystemNamespace,
			}, &corev1.Namespace{})
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
			if errors.IsNotFound(err) {
				if err = kubeClient.Create(ctx, &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: constants.HOHSystemNamespace,
					},
				}); err != nil {
					return err
				}
			}
			return nil
		}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())

		By("Create ConfigMap")
		Eventually(func() error {
			err := kubeClient.Get(ctx, types.NamespacedName{
				Namespace: constants.HOHSystemNamespace,
				Name:      constants.HOHConfigName,
			}, &corev1.ConfigMap{})
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
			if errors.IsNotFound(err) {
				if err = kubeClient.Create(ctx, &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: constants.HOHSystemNamespace,
						Name:      constants.HOHConfigName,
						Labels: map[string]string{
							commonconstants.GlobalHubOwnerLabelKey: commonconstants.HoHOperatorOwnerLabelVal,
						},
					},
					Data: map[string]string{
						"aggregationLevel":    "full",
						"enableLocalPolicies": "true",
					},
				}); err != nil {
					return err
				}
			}
			return nil
		}, 1*time.Second).ShouldNot(HaveOccurred())

		By("Check the ConfigMap instance is synchronized to database")
		Eventually(func() error {
			rows, err := postgresSQL.GetConn().Query(ctx,
				fmt.Sprintf("SELECT payload FROM %s.%s", testSchema, testTable))
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				configMap := &corev1.ConfigMap{}
				if err := rows.Scan(configMap); err != nil {
					return err
				}
				if constants.HOHConfigName == configMap.Name &&
					constants.HOHSystemNamespace == configMap.Namespace {
					return nil
				}
			}
			return fmt.Errorf("not find configmap in database")
		}, 1*time.Second).ShouldNot(HaveOccurred())
	})
})
