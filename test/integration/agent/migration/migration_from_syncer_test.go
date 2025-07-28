package migration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/syncers"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ = Describe("MigrationFromSyncer", Ordered, func() {
	var (
		testCtx           context.Context
		testCtxCancel     context.CancelFunc
		migrationSyncer   *syncers.MigrationSourceSyncer
		testMigrationID   = "test-migration-123"
		testFromHub       = "hub1"
		testToHub         = "hub2"
		testClusterName   = "test-cluster-1"
		testBootstrapName = "bootstrap-hub2"
	)

	BeforeAll(func() {
		receivedEvents = []*cloudevents.Event{}
		testCtx, testCtxCancel = context.WithCancel(ctx)
		migrationSyncer = syncers.NewMigrationSourceSyncer(
			runtimeClient,
			testenv.Config,
			transportClient,
			transportConfig,
		)

		namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: utils.GetDefaultNamespace()}}
		Expect(runtimeClient.Create(testCtx, namespace)).Should(Succeed())

		cluster := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:   testClusterName,
				Labels: map[string]string{"test-label": "test-value"},
			},
			Spec: clusterv1.ManagedClusterSpec{
				HubAcceptsClient: true,
				ManagedClusterClientConfigs: []clusterv1.ClientConfig{{
					URL: "https://test-cluster.example.com",
				}},
			},
		}
		Expect(runtimeClient.Create(testCtx, cluster)).Should(Succeed())

		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: testBootstrapName, Namespace: utils.GetDefaultNamespace()},
			Data:       map[string][]byte{"kubeconfig": []byte("test-kubeconfig-data")},
		}
		Expect(runtimeClient.Create(testCtx, bootstrapSecret)).Should(Succeed())

		multiclusterHub := &mchv1.MultiClusterHub{
			ObjectMeta: metav1.ObjectMeta{Name: "multiclusterhub", Namespace: utils.GetDefaultNamespace()},
			Status:     mchv1.MultiClusterHubStatus{CurrentVersion: "2.14.0"},
		}
		Expect(runtimeClient.Create(testCtx, multiclusterHub)).Should(Succeed())

		addonNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testClusterName}}
		Expect(runtimeClient.Create(testCtx, addonNamespace)).Should(Succeed())

		addonConfig := &addonv1.KlusterletAddonConfig{
			ObjectMeta: metav1.ObjectMeta{Name: testClusterName, Namespace: testClusterName},
			Spec:       addonv1.KlusterletAddonConfigSpec{ClusterName: testClusterName, ClusterNamespace: testClusterName},
		}
		Expect(runtimeClient.Create(testCtx, addonConfig)).Should(Succeed())

		configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: testFromHub})
	})

	AfterAll(func() {
		resources := []client.Object{
			&clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: testClusterName}},
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: testBootstrapName, Namespace: utils.GetDefaultNamespace()}},
			&mchv1.MultiClusterHub{ObjectMeta: metav1.ObjectMeta{Name: "multiclusterhub"}},
			&addonv1.KlusterletAddonConfig{ObjectMeta: metav1.ObjectMeta{Name: testClusterName, Namespace: testClusterName}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testClusterName}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: utils.GetDefaultNamespace()}},
		}
		for _, resource := range resources {
			_ = runtimeClient.Delete(testCtx, resource)
		}
		testCtxCancel()
	})

	Context("when handling migration lifecycle in from hub", func() {
		It("should initialize migration successfully", func() {
			By("Creating migration event for initializing stage")
			event := createMigrationFromEvent(testMigrationID, migrationv1alpha1.PhaseInitializing, testFromHub, testToHub, []string{testClusterName})
			event.DataEncoded, _ = json.Marshal(&migration.ManagedClusterMigrationFromEvent{
				MigrationId:     testMigrationID,
				Stage:           migrationv1alpha1.PhaseInitializing,
				ToHub:           testToHub,
				ManagedClusters: []string{testClusterName},
				BootstrapSecret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testBootstrapName,
						Namespace: utils.GetDefaultNamespace(),
					},
					Data: map[string][]byte{
						"kubeconfig": []byte("test-kubeconfig-data"),
					},
				},
			})

			By("Processing the migration event")
			err := migrationSyncer.Sync(testCtx, event)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying klusterletconfig was created")
			Eventually(func() error {
				klusterletConfig := &unstructured.Unstructured{}
				klusterletConfig.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "config.open-cluster-management.io",
					Version: "v1alpha1",
					Kind:    "KlusterletConfig",
				})
				return runtimeClient.Get(testCtx, types.NamespacedName{
					Name: fmt.Sprintf("migration-%s", testToHub),
				}, klusterletConfig)
			}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

			By("Verifying klusterletaddon configuration was added to managed cluster")
			Eventually(func() error {
				cluster := &clusterv1.ManagedCluster{}
				err := runtimeClient.Get(testCtx, types.NamespacedName{Name: testClusterName}, cluster)
				if err != nil {
					return nil
				}
				annotations := cluster.GetAnnotations()
				if annotations == nil {
					return fmt.Errorf("annotations is nil")
				}
				val, ok := annotations["agent.open-cluster-management.io/klusterlet-config"]
				if !ok {
					return fmt.Errorf("annotation agent.open-cluster-management.io/klusterlet-config is not set")
				}
				if val != fmt.Sprintf("migration-%s", testToHub) {
					return fmt.Errorf("annotation agent.open-cluster-management.io/klusterlet-config is not set correctly")
				}

				return nil
			}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

			By("Verifying the initializing event is sent to global hub")
			Eventually(func() error {
				return verifyMigrationEvent(testFromHub, string(enum.ManagedClusterMigrationType),
					constants.CloudEventGlobalHubClusterName, testMigrationID, migrationv1alpha1.PhaseInitializing)
			}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
		})

		It("should deploy resources successfully", func() {
			By("Creating migration event for deploying stage")
			event := createMigrationFromEvent(testMigrationID, migrationv1alpha1.PhaseDeploying, testFromHub, testToHub, []string{testClusterName})
			event.DataEncoded, _ = json.Marshal(&migration.ManagedClusterMigrationFromEvent{
				MigrationId:     testMigrationID,
				Stage:           migrationv1alpha1.PhaseDeploying,
				ToHub:           testToHub,
				ManagedClusters: []string{testClusterName},
			})

			By("Processing the deployment event")
			err := migrationSyncer.Sync(testCtx, event)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the deploying event is sent to target hub")
			Eventually(func() error {
				return verifyDeployingEvent(testFromHub, testMigrationID)
			}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
		})

		It("should registering: set HubAcceptsClient to false for clusters", func() {
			By("Creating migration event for registering stage")
			event := createMigrationFromEvent(testMigrationID, migrationv1alpha1.PhaseRegistering, testFromHub, testToHub, []string{testClusterName})
			event.DataEncoded, _ = json.Marshal(&migration.ManagedClusterMigrationFromEvent{
				MigrationId:     testMigrationID,
				Stage:           migrationv1alpha1.PhaseRegistering,
				ToHub:           testToHub,
				ManagedClusters: []string{testClusterName},
			})

			By("Processing the registering event")
			err := migrationSyncer.Sync(testCtx, event)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying HubAcceptsClient was set to false")
			Eventually(func() bool {
				cluster := &clusterv1.ManagedCluster{}
				err := runtimeClient.Get(testCtx, types.NamespacedName{Name: testClusterName}, cluster)
				if err != nil {
					return false
				}
				return !cluster.Spec.HubAcceptsClient
			}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
		})

		It("should clean up resources successfully", func() {
			By("Creating migration event for cleaning stage")
			event := createMigrationFromEvent(testMigrationID, migrationv1alpha1.PhaseCleaning, testFromHub, testToHub, []string{testClusterName})
			event.DataEncoded, _ = json.Marshal(&migration.ManagedClusterMigrationFromEvent{
				MigrationId:     testMigrationID,
				Stage:           migrationv1alpha1.PhaseCleaning,
				ToHub:           testToHub,
				ManagedClusters: []string{testClusterName},
				BootstrapSecret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testBootstrapName,
						Namespace: utils.GetDefaultNamespace(),
					},
				},
			})

			By("Processing the cleaning event")
			err := migrationSyncer.Sync(testCtx, event)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying bootstrap secret was deleted")
			Eventually(func() bool {
				secret := &corev1.Secret{}
				err := runtimeClient.Get(testCtx, types.NamespacedName{
					Name:      testBootstrapName,
					Namespace: utils.GetDefaultNamespace(),
				}, secret)
				return apierrors.IsNotFound(err)
			}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())

			By("Verifying klusterletconfig was deleted")
			Eventually(func() bool {
				klusterletConfig := &unstructured.Unstructured{}
				klusterletConfig.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "config.open-cluster-management.io",
					Version: "v1alpha1",
					Kind:    "KlusterletConfig",
				})
				err := runtimeClient.Get(testCtx, types.NamespacedName{
					Name: fmt.Sprintf("migration-%s", testToHub),
				}, klusterletConfig)
				return apierrors.IsNotFound(err)
			}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())

			By("Verifying the cluster is removed from managed cluster list")
			Eventually(func() bool {
				clusterList := &clusterv1.ManagedClusterList{}
				err := runtimeClient.List(testCtx, clusterList)
				if err != nil {
					return false
				}
				for _, cluster := range clusterList.Items {
					if cluster.Name == testClusterName {
						return false
					}
				}
				return true
			}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
		})
	})

	Context("Error handling scenarios", func() {
		It("should handle missing bootstrap secret during initialization", func() {
			By("Creating migration event with missing bootstrap secret")
			event := createMigrationFromEvent("error-test-1", migrationv1alpha1.PhaseInitializing, testFromHub, testToHub, []string{testClusterName})
			event.DataEncoded, _ = json.Marshal(&migration.ManagedClusterMigrationFromEvent{
				MigrationId:     "error-test-1",
				Stage:           migrationv1alpha1.PhaseInitializing,
				ToHub:           testToHub,
				ManagedClusters: []string{testClusterName},
				BootstrapSecret: nil, // Missing bootstrap secret
			})

			By("Processing event and expecting error")
			err := migrationSyncer.Sync(testCtx, event)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("bootstrap secret is nil"))
		})

		It("should handle missing managed cluster during deployment", func() {
			By("Creating migration event for non-existent cluster")
			initEvent := createMigrationFromEvent("error-test-2", migrationv1alpha1.PhaseInitializing, testFromHub, testToHub, []string{"non-existent-cluster"})

			By("Creating bootstrap secret for test")
			bootstrapSecretName := fmt.Sprintf("bootstrap-%s-test2", testToHub)
			bootstrapSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bootstrapSecretName,
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string][]byte{
					"kubeconfig": []byte("test-kubeconfig-data"),
				},
			}
			Expect(runtimeClient.Create(testCtx, bootstrapSecret)).Should(Succeed())
			defer func() {
				_ = runtimeClient.Delete(testCtx, bootstrapSecret)
			}()

			By("Preparing clean bootstrap secret for event")
			cleanBootstrapSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bootstrapSecretName,
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string][]byte{
					"kubeconfig": []byte("test-kubeconfig-data"),
				},
			}

			initEvent.DataEncoded, _ = json.Marshal(&migration.ManagedClusterMigrationFromEvent{
				MigrationId:     "error-test-2",
				Stage:           migrationv1alpha1.PhaseInitializing,
				ToHub:           testToHub,
				ManagedClusters: []string{"non-existent-cluster"},
				BootstrapSecret: cleanBootstrapSecret,
			})

			By("Processing event and expecting failure for non-existent cluster")
			err := migrationSyncer.Sync(testCtx, initEvent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("\"non-existent-cluster\" not found"))
		})

		It("should handle empty migration ID", func() {
			By("Creating migration event with empty migration ID")
			event := createMigrationFromEvent("", migrationv1alpha1.PhaseInitializing, testFromHub, testToHub, []string{testClusterName})

			By("Processing event and expecting error")
			err := migrationSyncer.Sync(testCtx, event)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("migrationId is required"))
		})
	})
})

func createMigrationFromEvent(migrationID, stage, fromHub, toHub string, clusters []string) *cloudevents.Event {
	event := cloudevents.NewEvent()
	event.SetType(string(enum.ManagedClusterMigrationType))
	event.SetSource(fromHub)
	event.SetSubject(constants.CloudEventGlobalHubClusterName)

	payload := &migration.ManagedClusterMigrationFromEvent{
		MigrationId:     migrationID,
		Stage:           stage,
		ToHub:           toHub,
		ManagedClusters: clusters,
	}

	data, _ := json.Marshal(payload)
	event.SetData("application/json", data)
	return &event
}
