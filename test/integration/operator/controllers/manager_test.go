package controllers

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/manager"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	testutils "github.com/stolostron/multicluster-global-hub/test/integration/utils"
)

// go test ./test/integration/operator -ginkgo.focus "manager" -v
var _ = Describe("manager", Ordered, func() {
	var mgh *v1alpha4.MulticlusterGlobalHub
	var namespace string
	var initOption config.ControllerOption
	var reconciler *manager.ManagerReconciler
	BeforeAll(func() {
		namespace = fmt.Sprintf("namespace-%s", rand.String(6))
		mghName := "test-mgh"
		reconciler = manager.NewManagerReconciler(runtimeManager, kubeClient, &config.OperatorConfig{
			EnablePprof:           false,
			GlobalResourceEnabled: true,
		})
		// mgh
		Expect(runtimeClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})).To(Succeed())
		mgh = &v1alpha4.MulticlusterGlobalHub{
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

		// update the middleware configuration
		// storage
		_ = config.SetStorageConnection(&config.PostgresConnection{
			SuperuserDatabaseURI:    "test-url",
			ReadonlyUserDatabaseURI: "test-url",
			CACert:                  []byte("test-crt"),
		})
		config.SetDatabaseReady(true)
		initOption = config.ControllerOption{
			Manager:               runtimeManager,
			MulticlusterGlobalHub: mgh,
			OperatorConfig:        &config.OperatorConfig{},
			KubeClient:            kubeClient,
		}
		// transport
		err := CreateTestSecretTransport(runtimeClient, mgh.Namespace)
		Expect(err).To(Succeed())
	})

	It("should start the manager controller", func() {
		_, err := manager.StartController(initOption)
		Expect(err).To(Succeed())
	})

	It("should generate the manager resources", func() {
		var err error
		// deployment
		Eventually(func() error {
			deployment := &appsv1.Deployment{}
			err = runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "multicluster-global-hub-manager",
				Namespace: mgh.Namespace,
			}, deployment)
			if err != nil {
				return err
			}
			clusterManagementAddOn := &v1alpha1.ClusterManagementAddOn{}
			err = runtimeClient.Get(ctx, types.NamespacedName{
				Name: constants.GHClusterManagementAddonName,
			}, clusterManagementAddOn)
			if err != nil {
				return err
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		Eventually(func() error {
			// service monitor
			serviceMonitor := &promv1.ServiceMonitor{}
			err = runtimeClient.Get(ctx, types.NamespacedName{
				Namespace: mgh.Namespace,
				Name:      operatorconstants.GHServiceMonitorName,
			}, serviceMonitor)
			return err
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should delete the ServiceMonitor when mgh deleted", func() {
		// create managed cluster migration
		mcm := &migrationv1alpha1.ManagedClusterMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test", Namespace: mgh.Namespace,
			},
			Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
				From:                    "hub1",
				To:                      "hub2",
				IncludedManagedClusters: []string{"cluster1"},
			},
		}
		err := runtimeClient.Create(ctx, mcm)
		Expect(err).To(Succeed())

		mgh.Finalizers = []string{"fz"}
		err = runtimeClient.Update(ctx, mgh)
		Expect(err).To(Succeed())

		err = runtimeClient.Delete(ctx, mgh)
		Expect(err).To(Succeed())

		_, err = reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: mgh.Namespace,
				Name:      mgh.Name,
			},
		})
		Expect(err).To(Succeed())
		Eventually(func() error {
			serviceMonitor := &promv1.ServiceMonitor{}
			err = runtimeClient.Get(ctx, types.NamespacedName{
				Namespace: utils.GetDefaultNamespace(),
				Name:      operatorconstants.GHServiceMonitorName,
			}, serviceMonitor)

			if errors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to delete service monitor, %v", err)
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	AfterAll(func() {
		Eventually(func() error {
			if err := testutils.DeleteMgh(ctx, runtimeClient, mgh); err != nil {
				return err
			}
			return deleteNamespace(namespace)
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		err := runtimeClient.Delete(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.GHTransportSecretName,
				Namespace: mgh.Namespace,
			},
		})
		Expect(err).To(Succeed())
	})
})
