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
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	managerscheme "github.com/stolostron/multicluster-global-hub/manager/pkg/scheme"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db/postgresql"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/spec2db/controller"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	commonconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	TEST_SCHEMA = "spec"
	TEST_TABLE  = "configs"
)

var _ = Describe("spec to database controller", Ordered, func() {
	var mgr ctrl.Manager
	var postgresSQL *postgresql.PostgreSQL
	var kubeClient client.Client

	BeforeAll(func() {
		By("Creating the Manager")
		var err error
		mgr, err = ctrl.NewManager(cfg, ctrl.Options{
			MetricsBindAddress: ":8083",
			Scheme:             scheme.Scheme,
		})
		Expect(err).NotTo(HaveOccurred())

		By("Add to Scheme")
		err = managerscheme.AddToScheme(mgr.GetScheme())
		Expect(err).NotTo(HaveOccurred())
		kubeClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		Expect(kubeClient).NotTo(BeNil())

		By("Connect to the database")
		postgresSQL, err = postgresql.NewPostgreSQL(testPostgres.URI)
		Expect(err).NotTo(HaveOccurred())
		Expect(postgresSQL).NotTo(BeNil())

		By("Adding the controllers to the manager")
		controller.AddHubOfHubsConfigController(mgr, postgresSQL)
		go func() {
			defer GinkgoRecover()
			err = mgr.Start(ctx)
			Expect(err).ToNot(HaveOccurred(), "failed to run manager")
		}()

		By("Waiting for the manager to be ready")
		Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())
	})

	It("get the spec.configs table from database", func() {
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
				if schema == TEST_SCHEMA && table == TEST_TABLE {
					return nil
				}
				fmt.Printf("table %s.%s \n", schema, table)
			}
			return fmt.Errorf("failed to create test table %s.%s", TEST_SCHEMA, TEST_TABLE)
		}, 10*time.Second, 2*time.Second).ShouldNot(HaveOccurred())
	})

	It("create the configmap", func() {
		By(fmt.Sprintf("Create Namespace: %s \n", constants.HOHSystemNamespace))
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

		By(fmt.Sprintf("Create ConfigMap: %s.%s", constants.HOHSystemNamespace, constants.HOHConfigName))
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
		}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
	})

	It("get configmap from database", func() {
		Eventually(func() error {
			rows, err := postgresSQL.GetConn().Query(ctx,
				fmt.Sprintf("SELECT payload FROM %s.%s", TEST_SCHEMA, TEST_TABLE))
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
		}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
	})

	AfterAll(func() {
		cancel()
		postgresSQL.Stop()
	})
})
