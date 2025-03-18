package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	"open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	bundleevent "github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	genericconsumer "github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ = Describe("migration", Ordered, func() {
	Context("managed cluster migration tests with from hub", func() {
		var migrationInstance *migrationv1alpha1.ManagedClusterMigration
		var genericConsumer *genericconsumer.GenericConsumer
		var testCtx context.Context
		var testCtxCancel context.CancelFunc
		var fromEvent, toEvent *cloudevents.Event
		BeforeAll(func() {
			testCtx, testCtxCancel = context.WithCancel(ctx)
			var err error
			genericConsumer, err = genericconsumer.NewGenericConsumer(transportConfig)
			Expect(err).NotTo(HaveOccurred())

			// start the consumer
			By("start the consumer")
			go func() {
				if err := genericConsumer.Start(testCtx); err != nil {
					logf.Log.Error(err, "error to start the chan consumer")
				}
			}()
			Expect(err).NotTo(HaveOccurred())

			// dispatch event
			By("dispatch event from consumer")
			go func(ctx context.Context) {
				for {
					select {
					case <-ctx.Done():
						return
					case evt := <-genericConsumer.EventChan():
						// if destination is explicitly specified and does not match, drop bundle
						clusterNameVal, err := evt.Context.GetExtension(constants.CloudEventExtensionKeyClusterName)
						if err != nil {
							logf.Log.Info("dropping bundle due to error in getting cluster name", "error", err)
							continue
						}
						clusterName, ok := clusterNameVal.(string)
						if !ok {
							logf.Log.Info("dropping bundle due to invalid cluster name", "clusterName", clusterNameVal)
							continue
						}
						switch clusterName {
						case "hub1":
							fromEvent = evt
						case "hub2":
							toEvent = evt
						default:
							logf.Log.Info("dropping bundle due to cluster name mismatch", "clusterName", clusterName)
						}
					}
				}
			}(testCtx)

			By("create hub2 namespace")
			hub2Namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "hub2"}}
			Expect(mgr.GetClient().Create(testCtx, hub2Namespace)).Should(Succeed())

			// create managedclustermigration CR
			By("create managedclustermigration CR")
			migrationInstance = &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					IncludedManagedClusters: []string{"cluster1"},
					From:                    "hub1",
					To:                      "hub2",
				},
			}
			Expect(mgr.GetClient().Create(testCtx, migrationInstance)).To(Succeed())

			// create a managedcluster
			By("create a managedcluster")
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
			Expect(mgr.GetClient().Create(testCtx, mc)).To(Succeed())

			// mimic msa generated secret
			By("mimic msa generated secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "migration",
					Namespace: "hub2",
					Labels: map[string]string{
						"authentication.open-cluster-management.io/is-managed-serviceaccount": "true",
					},
				},
				Data: map[string][]byte{
					"ca.crt": []byte("test"),
					"token":  []byte("test"),
				},
			}
			Expect(mgr.GetClient().Create(testCtx, secret)).To(Succeed())
		})

		AfterAll(func() {
			By("delete hub2 namespace")
			hub2Namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "hub2"}}
			Expect(mgr.GetClient().Delete(testCtx, hub2Namespace)).Should(Succeed())

			By("cancel the test context")
			testCtxCancel()
		})

		It("should have managedserviceaccount created", func() {
			Eventually(func() error {
				return mgr.GetClient().Get(testCtx, types.NamespacedName{
					Name:      "migration",
					Namespace: "hub2",
				}, &v1beta1.ManagedServiceAccount{})
			}, 3*time.Second, 100*time.Millisecond).Should(Succeed())
		})

		It("should have bootstrap secret generated if the secret is updated", func() {
			secret := &corev1.Secret{}
			Expect(mgr.GetClient().Get(testCtx, types.NamespacedName{
				Name:      "migration",
				Namespace: "hub2",
			}, secret)).To(Succeed())

			secret.Data["ca.crt"] = []byte("updated")
			Expect(mgr.GetClient().Update(testCtx, secret)).To(Succeed())

			Eventually(func() error {
				if migrationReconciler.BootstrapSecret == nil {
					return fmt.Errorf("bootstrap secret is nil")
				}
				kubeconfigBytes := migrationReconciler.BootstrapSecret.Data["kubeconfig"]
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

		It("should have received the migration event", func() {
			Eventually(func() error {
				if fromEvent == nil {
					return fmt.Errorf("fromEvent is nil")
				}
				if toEvent == nil {
					return fmt.Errorf("toEvent is nil")
				}
				if fromEvent.Source() != constants.CloudEventGlobalHubClusterName {
					return fmt.Errorf("fromEvent source is not correct")
				}
				if toEvent.Source() != constants.CloudEventGlobalHubClusterName {
					return fmt.Errorf("toEvent source is not correct")
				}
				if fromEvent.Type() != constants.CloudEventTypeMigrationFrom {
					return fmt.Errorf("fromEvent type is not correct")
				}
				if toEvent.Type() != constants.CloudEventTypeMigrationTo {
					return fmt.Errorf("toEvent type is not correct")
				}
				clusterNameVal, err := fromEvent.Context.GetExtension(constants.CloudEventExtensionKeyClusterName)
				if err != nil {
					return fmt.Errorf("failed to get cluster name from fromEvent: %v", err)
				}
				clusterName, ok := clusterNameVal.(string)
				if !ok {
					return fmt.Errorf("failed to convert cluster name to string")
				}
				if clusterName != "hub1" {
					return fmt.Errorf("fromEvent cluster name is not correct")
				}
				clusterNameVal, err = toEvent.Context.GetExtension(constants.CloudEventExtensionKeyClusterName)
				if err != nil {
					return fmt.Errorf("failed to get cluster name from toEvent: %v", err)
				}
				clusterName, ok = clusterNameVal.(string)
				if !ok {
					return fmt.Errorf("failed to convert cluster name to string")
				}
				if clusterName != "hub2" {
					return fmt.Errorf("toEvent cluster name is not correct")
				}
				if len(toEvent.Data()) == 0 {
					return fmt.Errorf("toEvent data is not empty")
				}
				if len(fromEvent.Data()) == 0 {
					return fmt.Errorf("fromEvent data is empty")
				}
				managedClusterMigrationFromEvent := &bundleevent.ManagedClusterMigrationFromEvent{}
				if err := json.Unmarshal(fromEvent.Data(), managedClusterMigrationFromEvent); err != nil {
					return fmt.Errorf("failed to unmarshal fromEvent data: %v", err)
				}
				if len(managedClusterMigrationFromEvent.ManagedClusters) != 1 || managedClusterMigrationFromEvent.ManagedClusters[0] != "cluster1" {
					return fmt.Errorf("managedClusters is not correct")
				}
				bootstrapSecret := managedClusterMigrationFromEvent.BootstrapSecret
				if bootstrapSecret == nil {
					return fmt.Errorf("bootstrapSecret is nil")
				}
				kubeconfigBytes := bootstrapSecret.Data["kubeconfig"]
				if len(kubeconfigBytes) == 0 {
					return fmt.Errorf("kubeconfig is empty")
				}
				_, err = clientcmd.RESTConfigFromKubeConfig(kubeconfigBytes)
				if err != nil {
					return fmt.Errorf("failed to create Kubernetes client config: %v", err)
				}
				klusterletConfig := managedClusterMigrationFromEvent.KlusterletConfig
				if klusterletConfig == nil {
					return fmt.Errorf("klusterletConfig is nil")
				}
				if klusterletConfig.Name != "migration-hub2" {
					return fmt.Errorf("klusterletConfig name is not correct")
				}
				if klusterletConfig.Spec.BootstrapKubeConfigs.Type != operatorv1.LocalSecrets {
					return fmt.Errorf("klusterletConfig bootstrap kubeconfig type is not correct")
				}
				if len(klusterletConfig.Spec.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets) != 1 {
					return fmt.Errorf("klusterletConfig bootstrap kubeconfig secrets is not correct")
				}
				if klusterletConfig.Spec.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets[0].Name != "bootstrap-hub2" {
					return fmt.Errorf("klusterletConfig bootstrap kubeconfig secret name 0 is not correct")
				}

				managedClusterMigrationToEvent := &bundleevent.ManagedClusterMigrationToEvent{}
				if err := json.Unmarshal(toEvent.Data(), managedClusterMigrationToEvent); err != nil {
					return fmt.Errorf("failed to unmarshal fromEvent data: %v", err)
				}
				if managedClusterMigrationToEvent.ManagedServiceAccountName != "migration" {
					return fmt.Errorf("Managed serviceaccount name is not correct")
				}
				if managedClusterMigrationToEvent.ManagedServiceAccountInstallNamespace != "open-cluster-management-agent-addon" {
					return fmt.Errorf("Managed serviceaccount install namespace is not correct")
				}
				return nil
			}, 10*time.Second, time.Second).Should(Succeed())
		})

		It("should recreate managedserviceaccount when it is deleted", func() {
			Expect(mgr.GetClient().Delete(testCtx, &v1beta1.ManagedServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "migration",
					Namespace: "hub2",
				},
			})).To(Succeed())

			Eventually(func() error {
				return mgr.GetClient().Get(testCtx, types.NamespacedName{
					Name:      "migration",
					Namespace: "hub2",
				}, &v1beta1.ManagedServiceAccount{})
			}, 3*time.Second, 100*time.Millisecond).Should(Succeed())
		})

		It("should have managedserviceaccount deleted when migration is deleted", func() {
			Expect(mgr.GetClient().Delete(testCtx, migrationInstance)).To(Succeed())

			Eventually(func() bool {
				msa := &v1beta1.ManagedServiceAccount{}
				err := mgr.GetClient().Get(testCtx, types.NamespacedName{
					Name:      "migration",
					Namespace: "hub2",
				}, msa)
				return apierrors.IsNotFound(err)
			}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())
		})
	})

	Context("managed cluster migration tests without from hub", func() {
		var migrationInstance *migrationv1alpha1.ManagedClusterMigration
		var genericConsumer *genericconsumer.GenericConsumer
		var testCtx context.Context
		var testCtxCancel context.CancelFunc
		var fromEvent1, fromEvent2, toEvent *cloudevents.Event
		BeforeAll(func() {
			testCtx, testCtxCancel = context.WithCancel(ctx)
			var err error
			genericConsumer, err = genericconsumer.NewGenericConsumer(transportConfig)
			Expect(err).NotTo(HaveOccurred())

			// start the consumer
			By("start the consumer")
			go func() {
				if err := genericConsumer.Start(testCtx); err != nil {
					logf.Log.Error(err, "error to start the chan consumer")
				}
			}()
			Expect(err).NotTo(HaveOccurred())

			// dispatch event
			By("dispatch event from consumer")
			go func(ctx context.Context) {
				for {
					select {
					case <-ctx.Done():
						return
					case evt := <-genericConsumer.EventChan():
						// if destination is explicitly specified and does not match, drop bundle
						clusterNameVal, err := evt.Context.GetExtension(constants.CloudEventExtensionKeyClusterName)
						if err != nil {
							logf.Log.Info("dropping bundle due to error in getting cluster name", "error", err)
							continue
						}
						clusterName, ok := clusterNameVal.(string)
						if !ok {
							logf.Log.Info("dropping bundle due to invalid cluster name", "clusterName", clusterNameVal)
							continue
						}
						switch clusterName {
						case "hub1":
							fromEvent1 = evt
						case "hub2":
							fromEvent2 = evt
						case "hub3":
							toEvent = evt
						default:
							logf.Log.Info("dropping bundle due to cluster name mismatch", "clusterName", clusterName)
						}
					}
				}
			}(testCtx)

			// create hub3 namespace
			By("create hub3 namespace")
			hub3Namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "hub3"}}
			Expect(mgr.GetClient().Create(testCtx, hub3Namespace)).Should(Succeed())

			// create managedclustermigration CR
			By("create managedclustermigration CR")
			migrationInstance = &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					IncludedManagedClusters: []string{"cluster1", "cluster2", "cluster3"},
					To:                      "hub3",
				},
			}
			Expect(mgr.GetClient().Create(testCtx, migrationInstance)).To(Succeed())

			// create a managedcluster
			By("create a managedcluster")
			mc := &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hub3",
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
			Expect(mgr.GetClient().Create(testCtx, mc)).To(Succeed())

			// mimic msa generated secret
			By("mimic msa generated secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "migration",
					Namespace: "hub3",
					Labels: map[string]string{
						"authentication.open-cluster-management.io/is-managed-serviceaccount": "true",
					},
				},
				Data: map[string][]byte{
					"ca.crt": []byte("test"),
					"token":  []byte("test"),
				},
			}
			Expect(mgr.GetClient().Create(testCtx, secret)).To(Succeed())

			By("create the managed cluster data in the database")
			Expect(db.Exec(`INSERT INTO "status"."managed_clusters" ("leaf_hub_name", "cluster_id", "error", "payload") VALUES
			('hub1', '52fbf2b6-8c6b-4d79-ad51-81f2a5e3e5e0', 'none', '{"metadata": {"name": "cluster1"}}'),
			('hub2', 'aa813a35-c470-4c0a-81ac-4fbac75e8161', 'none', '{"metadata": {"name": "cluster2"}}'),
			('hub2', '2c6da4e7-0b9c-4194-9930-3d0def3619a0', 'none', '{"metadata": {"name": "cluster3"}}')`).Error).ToNot(HaveOccurred())
		})

		AfterAll(func() {
			By("delete hub3 namespace")
			hub3Namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "hub3"}}
			Expect(mgr.GetClient().Delete(testCtx, hub3Namespace)).Should(Succeed())

			By("cancel the test context")
			testCtxCancel()
		})

		It("should have managedserviceaccount created", func() {
			Eventually(func() error {
				return mgr.GetClient().Get(testCtx, types.NamespacedName{
					Name:      "migration",
					Namespace: "hub3",
				}, &v1beta1.ManagedServiceAccount{})
			}, 3*time.Second, 100*time.Millisecond).Should(Succeed())
		})

		It("should have bootstrap secret generated if the secret is updated", func() {
			secret := &corev1.Secret{}
			Expect(mgr.GetClient().Get(testCtx, types.NamespacedName{
				Name:      "migration",
				Namespace: "hub3",
			}, secret)).To(Succeed())

			secret.Data["ca.crt"] = []byte("updated")
			Expect(mgr.GetClient().Update(testCtx, secret)).To(Succeed())

			Eventually(func() error {
				if migrationReconciler.BootstrapSecret == nil {
					return fmt.Errorf("bootstrap secret is nil")
				}
				kubeconfigBytes := migrationReconciler.BootstrapSecret.Data["kubeconfig"]
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

		It("should have the migration event received", func() {
			Eventually(func() error {
				if fromEvent1 == nil {
					return fmt.Errorf("fromEvent1 is nil")
				}
				if fromEvent2 == nil {
					return fmt.Errorf("fromEvent2 is nil")
				}
				if toEvent == nil {
					return fmt.Errorf("toEvent is nil")
				}
				if fromEvent1.Source() != constants.CloudEventGlobalHubClusterName {
					return fmt.Errorf("fromEvent1 source is not correct")
				}
				if fromEvent2.Source() != constants.CloudEventGlobalHubClusterName {
					return fmt.Errorf("fromEvent2 source is not correct")
				}
				if toEvent.Source() != constants.CloudEventGlobalHubClusterName {
					return fmt.Errorf("toEvent source is not correct")
				}
				if fromEvent1.Type() != constants.CloudEventTypeMigrationFrom {
					return fmt.Errorf("fromEvent1 type is not correct")
				}
				if fromEvent2.Type() != constants.CloudEventTypeMigrationFrom {
					return fmt.Errorf("fromEvent2 type is not correct")
				}
				if toEvent.Type() != constants.CloudEventTypeMigrationTo {
					return fmt.Errorf("toEvent type is not correct")
				}
				clusterNameVal, err := fromEvent1.Context.GetExtension(constants.CloudEventExtensionKeyClusterName)
				if err != nil {
					return fmt.Errorf("failed to get cluster name from fromEvent1: %v", err)
				}
				clusterName, ok := clusterNameVal.(string)
				if !ok {
					return fmt.Errorf("failed to convert cluster name to string for fromEvent1")
				}
				if clusterName != "hub1" {
					return fmt.Errorf("fromEvent1 cluster name is not correct")
				}
				clusterNameVal, err = fromEvent2.Context.GetExtension(constants.CloudEventExtensionKeyClusterName)
				if err != nil {
					return fmt.Errorf("failed to get cluster name from fromEvent2: %v", err)
				}
				clusterName, ok = clusterNameVal.(string)
				if !ok {
					return fmt.Errorf("failed to convert cluster name to string for fromEvent2")
				}
				if clusterName != "hub2" {
					return fmt.Errorf("fromEvent2 cluster name is not correct")
				}
				clusterNameVal, err = toEvent.Context.GetExtension(constants.CloudEventExtensionKeyClusterName)
				if err != nil {
					return fmt.Errorf("failed to get cluster name from toEvent: %v", err)
				}
				clusterName, ok = clusterNameVal.(string)
				if !ok {
					return fmt.Errorf("failed to convert cluster name to string")
				}
				if clusterName != "hub3" {
					return fmt.Errorf("toEvent cluster name is not correct")
				}
				if len(toEvent.Data()) == 0 {
					return fmt.Errorf("toEvent data is not empty")
				}
				if len(fromEvent1.Data()) == 0 {
					return fmt.Errorf("fromEvent1 data is empty")
				}
				if len(fromEvent2.Data()) == 0 {
					return fmt.Errorf("fromEvent2 data is empty")
				}
				managedClusterMigrationFromEvent1 := &bundleevent.ManagedClusterMigrationFromEvent{}
				if err := json.Unmarshal(fromEvent1.Data(), managedClusterMigrationFromEvent1); err != nil {
					return fmt.Errorf("failed to unmarshal fromEvent1 data: %v", err)
				}
				if len(managedClusterMigrationFromEvent1.ManagedClusters) != 1 || managedClusterMigrationFromEvent1.ManagedClusters[0] != "cluster1" {
					return fmt.Errorf("managedClusters is not correct")
				}
				bootstrapSecret := managedClusterMigrationFromEvent1.BootstrapSecret
				if bootstrapSecret == nil {
					return fmt.Errorf("bootstrapSecret is nil")
				}
				kubeconfigBytes := bootstrapSecret.Data["kubeconfig"]
				if len(kubeconfigBytes) == 0 {
					return fmt.Errorf("kubeconfig is empty")
				}
				_, err = clientcmd.RESTConfigFromKubeConfig(kubeconfigBytes)
				if err != nil {
					return fmt.Errorf("failed to create Kubernetes client config: %v", err)
				}
				klusterletConfig := managedClusterMigrationFromEvent1.KlusterletConfig
				if klusterletConfig == nil {
					return fmt.Errorf("klusterletConfig is nil")
				}
				if klusterletConfig.Name != "migration-hub3" {
					return fmt.Errorf("klusterletConfig name is not correct")
				}
				if klusterletConfig.Spec.BootstrapKubeConfigs.Type != operatorv1.LocalSecrets {
					return fmt.Errorf("klusterletConfig bootstrap kubeconfig type is not correct")
				}
				if len(klusterletConfig.Spec.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets) != 1 {
					return fmt.Errorf("klusterletConfig bootstrap kubeconfig secrets is not correct")
				}
				if klusterletConfig.Spec.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets[0].Name != "bootstrap-hub3" {
					return fmt.Errorf("klusterletConfig bootstrap kubeconfig secret name 0 is not correct")
				}

				managedClusterMigrationFromEvent2 := &bundleevent.ManagedClusterMigrationFromEvent{}
				if err := json.Unmarshal(fromEvent2.Data(), managedClusterMigrationFromEvent2); err != nil {
					return fmt.Errorf("failed to unmarshal fromEvent2 data: %v", err)
				}
				if len(managedClusterMigrationFromEvent2.ManagedClusters) != 2 ||
					managedClusterMigrationFromEvent2.ManagedClusters[0] != "cluster2" ||
					managedClusterMigrationFromEvent2.ManagedClusters[1] != "cluster3" {
					return fmt.Errorf("managedClusters is not correct")
				}

				bootstrapSecret = managedClusterMigrationFromEvent2.BootstrapSecret
				if bootstrapSecret == nil {
					return fmt.Errorf("bootstrapSecret is nil")
				}
				kubeconfigBytes = bootstrapSecret.Data["kubeconfig"]
				if len(kubeconfigBytes) == 0 {
					return fmt.Errorf("kubeconfig is empty")
				}
				_, err = clientcmd.RESTConfigFromKubeConfig(kubeconfigBytes)
				if err != nil {
					return fmt.Errorf("failed to create Kubernetes client config: %v", err)
				}
				klusterletConfig = managedClusterMigrationFromEvent2.KlusterletConfig
				if klusterletConfig == nil {
					return fmt.Errorf("klusterletConfig is nil")
				}
				if klusterletConfig.Name != "migration-hub3" {
					return fmt.Errorf("klusterletConfig name is not correct")
				}
				if klusterletConfig.Spec.BootstrapKubeConfigs.Type != operatorv1.LocalSecrets {
					return fmt.Errorf("klusterletConfig bootstrap kubeconfig type is not correct")
				}
				if len(klusterletConfig.Spec.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets) != 1 {
					return fmt.Errorf("klusterletConfig bootstrap kubeconfig secrets is not correct")
				}
				if klusterletConfig.Spec.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets[0].Name != "bootstrap-hub3" {
					return fmt.Errorf("klusterletConfig bootstrap kubeconfig secret name 0 is not correct")
				}

				managedClusterMigrationToEvent := &bundleevent.ManagedClusterMigrationToEvent{}
				if err := json.Unmarshal(toEvent.Data(), managedClusterMigrationToEvent); err != nil {
					return fmt.Errorf("failed to unmarshal fromEvent data: %v", err)
				}
				if managedClusterMigrationToEvent.ManagedServiceAccountName != "migration" {
					return fmt.Errorf("Managed serviceaccount name is not correct")
				}
				if managedClusterMigrationToEvent.ManagedServiceAccountInstallNamespace != "open-cluster-management-agent-addon" {
					return fmt.Errorf("Managed serviceaccount install namespace is not correct")
				}
				return nil
			}, 10*time.Second, time.Second).Should(Succeed())
		})

		It("should recreate managedserviceaccount when it is deleted", func() {
			Expect(mgr.GetClient().Delete(testCtx, &v1beta1.ManagedServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "migration",
					Namespace: "hub3",
				},
			})).To(Succeed())

			Eventually(func() error {
				return mgr.GetClient().Get(testCtx, types.NamespacedName{
					Name:      "migration",
					Namespace: "hub3",
				}, &v1beta1.ManagedServiceAccount{})
			}, 3*time.Second, 100*time.Millisecond).Should(Succeed())
		})

		It("should have managedserviceaccount deleted when migration is deleted", func() {
			Expect(mgr.GetClient().Delete(testCtx, migrationInstance)).To(Succeed())

			Eventually(func() bool {
				msa := &v1beta1.ManagedServiceAccount{}
				err := mgr.GetClient().Get(testCtx, types.NamespacedName{
					Name:      "migration",
					Namespace: "hub3",
				}, msa)
				return apierrors.IsNotFound(err)
			}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())
		})
	})
})
