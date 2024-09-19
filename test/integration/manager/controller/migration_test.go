package controller

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/migration"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var _ = Describe("migration", Ordered, func() {
	var migrationInstance *migrationv1alpha1.ManagedClusterMigration
	BeforeAll(func() {
		mghSystemNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: constants.GHDefaultNamespace}}
		Expect(mgr.GetClient().Create(ctx, mghSystemNamespace)).Should(Succeed())

		hub2Namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "hub2"}}
		Expect(mgr.GetClient().Create(ctx, hub2Namespace)).Should(Succeed())

		// create managedclustermigration CR
		migrationInstance = &migrationv1alpha1.ManagedClusterMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "migration",
				Namespace: constants.GHDefaultNamespace,
			},
			Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
				IncludedManagedClusters: []string{"cluster1"},
				From:                    "hub1",
				To:                      "hub2",
			},
		}
		Expect(mgr.GetClient().Create(ctx, migrationInstance)).To(Succeed())

		// create a managedcluster
		mc := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "hub2",
			},
			Spec: clusterv1.ManagedClusterSpec{
				ManagedClusterClientConfigs: []clusterv1.ClientConfig{
					{
						URL:      "https://example.com",
						CABundle: []byte("test"),
					},
				},
			},
		}
		Expect(mgr.GetClient().Create(ctx, mc)).To(Succeed())

		// mimic msa generated secret
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "migration",
				Namespace: "hub2",
			},
			Data: map[string][]byte{
				"ca.crt": []byte("test"),
				"token":  []byte("test"),
			},
		}
		Expect(mgr.GetClient().Create(ctx, secret)).To(Succeed())
	})
	It("should have managedserviceaccount created", func() {
		_, err := migration.NewMigrationReconciler(mgr).Reconcile(ctx, ctrl.Request{
			NamespacedName: client.ObjectKeyFromObject(migrationInstance),
		})
		Expect(err).To(Succeed())

		Eventually(func() error {
			return mgr.GetClient().Get(ctx, types.NamespacedName{
				Name:      "migration",
				Namespace: "hub2",
			}, &v1beta1.ManagedServiceAccount{})
		}, 3*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("should have bootstrap secret generated", func() {
		reconciler := migration.NewMigrationReconciler(mgr)
		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      "migration",
				Namespace: "hub2",
			},
		})
		Expect(err).To(Succeed())

		Eventually(func() error {
			kubeconfigBytes := reconciler.BootstrapSecret.Data["kubeconfig"]
			if string(kubeconfigBytes) == "" {
				return fmt.Errorf("failed to generate kubeconfig")
			}

			_, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigBytes)
			if err != nil {
				return fmt.Errorf("failed to create Kubernetes client config: %v", err)
			}
			return nil
		}, 3*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("should have managedserviceaccount deleted when migration is deleted", func() {
		Expect(mgr.GetClient().Delete(ctx, migrationInstance)).To(Succeed())
		Expect(mgr.GetClient().Delete(ctx, &v1beta1.ManagedServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "migration",
				Namespace: "hub2",
			},
		})).To(Succeed())

		Eventually(func() bool {
			msa := &v1beta1.ManagedServiceAccount{}
			err := mgr.GetClient().Get(ctx, types.NamespacedName{
				Name:      "migration",
				Namespace: "hub2",
			}, msa)
			return apierrors.IsNotFound(err)
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())
	})

	It("should setup with manager without error", func() {
		Expect(migration.NewMigrationReconciler(mgr).SetupWithManager(mgr)).To(Succeed())
	})

	AfterAll(func() {
		mghSystemNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: constants.GHDefaultNamespace}}
		Expect(mgr.GetClient().Delete(ctx, mghSystemNamespace)).Should(Succeed())

		hub2Namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "hub2"}}
		Expect(mgr.GetClient().Delete(ctx, hub2Namespace)).Should(Succeed())
	})
})
