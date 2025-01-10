package controllers

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	postgresv1beta1 "github.com/crunchydata/postgres-operator/pkg/apis/postgres-operator.crunchydata.com/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/storage"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	testutils "github.com/stolostron/multicluster-global-hub/test/integration/utils"
)

// go test ./test/integration/operator/controllers -ginkgo.focus "storage" -v
var _ = Describe("storage", Ordered, func() {
	It("should init database with BYO", func() {
		namespace := fmt.Sprintf("namespace-%s", rand.String(6))
		mghName := "test-mgh"

		// mgh
		err := os.Setenv("POD_NAMESPACE", namespace)
		Expect(err).To(Succeed())
		Expect(runtimeClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})).To(Succeed())
		mgh := &v1alpha4.MulticlusterGlobalHub{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mghName,
				Namespace: namespace,
			},
			Spec: v1alpha4.MulticlusterGlobalHubSpec{
				EnableMetrics: true,
				DataLayerSpec: v1alpha4.DataLayerSpec{
					Postgres: v1alpha4.PostgresSpec{
						DatabaseInitSQL: v1alpha4.DatabaseInitSQL{
							Name: "pgcmain-bootstrap-sql",
							Key:  "bootstrap.sql",
						},
					},
				},
			},
		}
		Expect(runtimeClient.Create(ctx, mgh)).To(Succeed())
		Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)).To(Succeed())

		err = createInitSQLConfigMap(ctx, runtimeClient, namespace, "pgcmain-bootstrap-sql", "bootstrap.sql")
		Expect(err).To(Succeed())

		// storage secret
		// pgURI := strings.Replace(testPostgres.URI, "sslmode=verify-ca", "sslmode=require", -1)
		storageSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.GHStorageSecretName,
				Namespace: mgh.Namespace,
			},
			Data: map[string][]byte{
				"database_uri":                   []byte(testPostgres.URI),
				"database_uri_with_readonlyuser": []byte(testPostgres.URI),
				"ca.crt":                         []byte(""),
			},
			Type: corev1.SecretTypeOpaque,
		}
		Expect(runtimeClient.Create(ctx, storageSecret)).To(Succeed())

		storageReconciler := storage.NewStorageReconciler(runtimeManager, true, false)
		Expect(err).To(Succeed())
		err = storageReconciler.SetupWithManager(runtimeManager)
		Expect(err).To(Succeed())

		// the subscription
		_, err = storageReconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: mgh.Namespace,
				Name:      mgh.Name,
			},
		})
		Expect(err).To(Succeed())

		// check the database
		exist := checkTableExistsInDatabase("hoh", "status", "managed_clusters")
		Expect(exist).To(BeTrue())

		// check the database
		exist = checkTableExistsInDatabase("test", "public", "COMPANY")
		Expect(exist).To(BeTrue())

		err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)
		Expect(err).To(Succeed())

		err = runtimeClient.Delete(ctx, storageSecret)
		Expect(err).To(Succeed())
		Eventually(func() error {
			return testutils.DeleteMgh(ctx, runtimeClient, mgh)
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		err = runtimeClient.Delete(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})
		Expect(err).To(Succeed())
	})

	It("should init storage with crunchy operator", func() {
		subCtx, subCancel := context.WithCancel(ctx)
		defer subCancel()
		// mgh namespce
		namespace := fmt.Sprintf("namespace-%s", rand.String(6))
		err := os.Setenv("POD_NAMESPACE", namespace)
		Expect(err).To(Succeed())
		Expect(runtimeClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})).To(Succeed())

		// mgh instance
		mgh := &v1alpha4.MulticlusterGlobalHub{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-mgh",
				Namespace: namespace,
				Annotations: map[string]string{
					operatorconstants.AnnotationMGHInstallCrunchyOperator: "true",
				},
			},
		}
		Expect(runtimeClient.Create(ctx, mgh)).To(Succeed())
		Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)).To(Succeed())

		storageReconciler := storage.NewStorageReconciler(runtimeManager, true, false)

		// blocking until get the connection
		go func() {
			_, _ = storageReconciler.ReconcileStorage(subCtx, mgh)
		}()

		// the subscription
		Eventually(func() error {
			sub, err := operatorutils.GetSubscriptionByName(ctx, runtimeClient, namespace, storage.SubscriptionName)
			if err != nil {
				return err
			}
			if sub == nil {
				return fmt.Errorf("should get the subscription %s", storage.SubscriptionName)
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		// the postgres cluster
		Eventually(func() error {
			postgresCluster := &postgresv1beta1.PostgresCluster{}
			err := runtimeClient.Get(ctx, types.NamespacedName{
				Name:      config.PostgresName,
				Namespace: namespace,
			}, postgresCluster)
			if err != nil {
				return err
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		// cleanup
		Eventually(func() error {
			return testutils.DeleteMgh(ctx, runtimeClient, mgh)
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		err = runtimeClient.Delete(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})
		Expect(err).To(Succeed())
	})

	It("should init storage with statefulset", func() {
		subCtx, subCancel := context.WithCancel(ctx)
		defer subCancel()
		// mgh namespce
		namespace := fmt.Sprintf("namespace-%s", rand.String(6))
		err := os.Setenv("POD_NAMESPACE", namespace)
		Expect(err).To(Succeed())
		Expect(runtimeClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})).To(Succeed())

		// mgh instance
		mgh := &v1alpha4.MulticlusterGlobalHub{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "test-mgh",
				Namespace:   namespace,
				Annotations: map[string]string{},
			},
		}
		Expect(runtimeClient.Create(ctx, mgh)).To(Succeed())
		Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)).To(Succeed())

		storageReconciler := storage.NewStorageReconciler(runtimeManager, true, false)

		// blocking until get the connection
		go func() {
			_, _ = storageReconciler.ReconcileStorage(subCtx, mgh)
		}()

		// the statefulset
		Eventually(func() error {
			statefulSet := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      storage.BuiltinPostgresName,
					Namespace: mgh.Namespace,
				},
			}
			err := runtimeClient.Get(ctx, client.ObjectKeyFromObject(statefulSet), statefulSet)
			if err != nil {
				return err
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		// cleanup
		Eventually(func() error {
			if err := testutils.DeleteMgh(ctx, runtimeClient, mgh); err != nil {
				return err
			}
			return deleteNamespace(namespace)
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})

// Function to check if a table exists in the given database
func checkTableExistsInDatabase(databaseName, schemaName, tableName string) bool {
	sql := fmt.Sprintf(`
		SELECT EXISTS (
			SELECT 1
			FROM %s.information_schema.tables
			WHERE table_schema = $1
			AND table_name = $2
		)`, databaseName) // Use the provided database name in the query

	var exists bool
	err := conn.QueryRow(context.Background(), sql, schemaName, tableName).Scan(&exists)
	if err != nil {
		logger.DefaultZapLogger().Errorf("failed to checking if table exists:", err)
		return false
	}
	return exists
}

// Function to create ConfigMap
func createInitSQLConfigMap(ctx context.Context, c client.Client, namespace, name, sqlKey string) error {
	// Define the ConfigMap with the provided SQL script
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			sqlKey: `
-- Create the test database if it doesn't exist
CREATE DATABASE test;

-- Create the COMPANY table in the 'test' database
CREATE TABLE IF NOT EXISTS public.COMPANY (
  ID INT PRIMARY KEY NOT NULL,
  NAME TEXT NOT NULL,
  AGE INT NOT NULL,
  ADDRESS CHAR(50),
  SALARY REAL
);
`,
		},
	}

	// Create the ConfigMap in Kubernetes
	if err := c.Create(ctx, configMap); err != nil {
		return fmt.Errorf("failed to create ConfigMap: %w", err)
	}

	log.Println("ConfigMap created successfully.")
	return nil
}
