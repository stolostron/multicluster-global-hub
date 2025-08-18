package controllers

import (
	"context"
	"fmt"
	"os"
	"time"

	postgresv1beta1 "github.com/crunchydata/postgres-operator/pkg/apis/postgres-operator.crunchydata.com/v1beta1"
	"github.com/jackc/pgx/v5"
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
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	testutils "github.com/stolostron/multicluster-global-hub/test/integration/utils"
)

// go test ./test/integration/operator/controllers -ginkgo.focus "storage" -v
var _ = Describe("storage", Ordered, func() {
	It("should init database", func() {
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
			},
		}
		Expect(runtimeClient.Create(ctx, mgh)).To(Succeed())
		Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)).To(Succeed())

		// storage secret - BYO
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

		storageReconciler := storage.NewStorageReconciler(runtimeManager, true)
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

		err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)
		Expect(err).To(Succeed())

		// reconcile database(annotation) -> mock builtin
		postgresUserReconciler := &storage.PostgresConfigUserReconciler{Manager: runtimeManager}
		err = postgresUserReconciler.SetupWithManager(runtimeManager)
		Expect(err).To(Succeed())
		config.SetBYOPostgres(false)
		// add 1 test user
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      storage.PostgresCustomizedUsersConfigMapName,
				Namespace: mgh.Namespace,
			},
			Data: map[string]string{
				"test-user1": "[\"test1\", \"test-2\"]",
			},
		}
		Expect(runtimeClient.Create(ctx, configMap)).To(Succeed())

		// verify the tesetuser1
		Eventually(func() error {
			_, err = storageReconciler.ReconcileDatabase(ctx, mgh)
			if err != nil {
				return err
			}

			secret := &corev1.Secret{}
			err = runtimeClient.Get(ctx, types.NamespacedName{
				Namespace: mgh.Namespace,
				Name:      "postgresql-user-test-user1",
			}, secret)
			if err != nil {
				return err
			}
			config, err := pgx.ParseConfig(testPostgres.URI)
			if err != nil {
				return err
			}
			// verification in database
			config.User = "test-user1"
			config.Database = "test1"
			config.Password = string(secret.Data[storage.PostgresCustomizedUserSecretPasswordKey])
			conn, err := pgx.ConnectConfig(ctx, config)
			if err != nil {
				return err
			}
			_, err = conn.Exec(ctx, createSchemaResourceSQL)
			if err != nil {
				return fmt.Errorf("unable to create resource in customized schema: %v", err)
			}
			_, err = conn.Exec(ctx, createPublicSchemaResourceSQL)
			if err != nil {
				return fmt.Errorf("unable to create resource in public schema: %v", err)
			}
			utils.PrettyPrint(secret)
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		// add 2 test users
		Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(configMap), configMap)).To(Succeed())
		configMap.Data["test_user2"] = "[\"test3\", \"test_4\"]"
		Expect(runtimeClient.Update(ctx, configMap)).To(Succeed())
		// verify the tesetuser2
		Eventually(func() error {
			_, err = storageReconciler.ReconcileDatabase(ctx, mgh)
			if err != nil {
				return err
			}

			secret := &corev1.Secret{}
			err = runtimeClient.Get(ctx, types.NamespacedName{
				Namespace: mgh.Namespace,
				Name:      "postgresql-user-test-user2",
			}, secret)
			if err != nil {
				return err
			}
			utils.PrettyPrint(secret)
			if string(secret.Data["db.user"]) != "test_user2" ||
				string(secret.Data["db.names"]) != "[\"test3\", \"test_4\"]" {
				return fmt.Errorf("unexpected secret: %v", secret)
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		config.SetBYOPostgres(true)

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

		storageReconciler := storage.NewStorageReconciler(runtimeManager, true)

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

		// add customized postgres configuration
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multicluster-global-hub-custom-postgresql-config",
				Namespace: namespace,
			},
			Data: map[string]string{
				"postgresql.conf": "wal_level = logical\nmax_wal_size = 2GB\n",
			},
		}
		Expect(runtimeClient.Create(ctx, cm)).To(Succeed())

		storageReconciler := storage.NewStorageReconciler(runtimeManager, true)

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

		// verify the customized configuration
		cm = &corev1.ConfigMap{}
		err = runtimeClient.Get(ctx, types.NamespacedName{Namespace: mgh.Namespace, Name: "multicluster-global-hub-postgresql-config"}, cm)
		Expect(err).To(Succeed())
		Expect(cm.Data["postgresql.conf"]).To(ContainSubstring("max_wal_size = 2GB"))
		utils.PrettyPrint(cm.Data["postgresql.conf"])

		// cleanup
		Eventually(func() error {
			if err := testutils.DeleteMgh(ctx, runtimeClient, mgh); err != nil {
				return err
			}
			return deleteNamespace(namespace)
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})

const createPublicSchemaResourceSQL = `-- Step 1: Create the table
CREATE TABLE test_table (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Step 2: Modify the table by adding a new column
ALTER TABLE test_table ADD COLUMN updated_at TIMESTAMP;

-- Step 3: Create an index on the 'name' column
CREATE INDEX idx_name ON test_table (name);

-- Step 4: Create the trigger function
CREATE OR REPLACE FUNCTION update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Step 5: Attach the trigger to the table
CREATE TRIGGER update_test_table_timestamp
BEFORE UPDATE ON test_table
FOR EACH ROW
EXECUTE FUNCTION update_timestamp();`

const createSchemaResourceSQL = `
-- Create a schema
CREATE SCHEMA my_schema;

-- Create a table in the new schema
CREATE TABLE my_schema.my_table (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`
