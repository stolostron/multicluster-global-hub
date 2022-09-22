package controller_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	appv1beta1 "sigs.k8s.io/application/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("application to database controller", func() {
	const testSchema = "spec"
	const testTable = "applications"

	BeforeEach(func() {
		By("Creating test table in the database")
		_, err := postgresSQL.GetConn().Exec(ctx, `
			CREATE SCHEMA IF NOT EXISTS spec;
			CREATE TABLE IF NOT EXISTS  spec.applications (
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

	It("filter application with MGH OwnerReferences", func() {
		By("Create application foo1 instance with OwnerReference")
		filteredApp := &appv1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo1",
				Namespace: config.GetDefaultNamespace(),
			},
			Spec: appv1beta1.ApplicationSpec{},
		}
		Expect(controllerutil.SetControllerReference(mghInstance, filteredApp, mgr.GetScheme()))
		Expect(kubeClient.Create(ctx, filteredApp)).Should(Succeed())

		By("Create application foo2 instance without OwnerReference")
		expectedApp := &appv1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo2",
				Namespace: config.GetDefaultNamespace(),
			},
			Spec: appv1beta1.ApplicationSpec{},
		}
		Expect(kubeClient.Create(ctx, expectedApp)).Should(Succeed())

		Eventually(func() error {
			expectedAppSynced := false
			rows, err := postgresSQL.GetConn().Query(ctx, fmt.Sprintf("SELECT payload FROM %s.%s", testSchema, testTable))
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				syncedApp := &appv1beta1.Application{}
				if err := rows.Scan(syncedApp); err != nil {
					return err
				}
				fmt.Printf("spec.applications: %s - %s \n", syncedApp.Namespace, syncedApp.Name)
				if syncedApp.GetNamespace() == expectedApp.GetNamespace() && syncedApp.GetName() == expectedApp.GetName() {
					expectedAppSynced = true
				}
				if syncedApp.GetNamespace() == filteredApp.GetNamespace() && syncedApp.GetName() == filteredApp.GetName() {
					return fmt.Errorf("app(%s) with OwnerReference(MGH) should't be synchronized to database",
						filteredApp.GetName())
				}
			}
			if expectedAppSynced {
				return nil
			} else {
				return fmt.Errorf("not find app(%s) in database", expectedApp.GetName())
			}
		}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
	})
})
