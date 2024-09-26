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

	"github.com/stolostron/multicluster-global-hub/manager/pkg/migration"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	bundleevent "github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	genericconsumer "github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	genericproducer "github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ = Describe("migration", Ordered, func() {
	var migrationInstance *migrationv1alpha1.ManagedClusterMigration
	var migrationReconciler *migration.MigrationReconciler
	var genericConsumer *genericconsumer.GenericConsumer
	var fromEvent, toEvent *cloudevents.Event
	BeforeAll(func() {
		var err error
		genericConsumer, err = genericconsumer.NewGenericConsumer(transportConfig)
		Expect(err).NotTo(HaveOccurred())

		// start the consumer
		go func() {
			if err := genericConsumer.Start(ctx); err != nil {
				logf.Log.Error(err, "error to start the chan consumer")
			}
		}()
		Expect(err).NotTo(HaveOccurred())

		// dispatch event
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
		}(ctx)
		hub2Namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "hub2"}}
		Expect(mgr.GetClient().Create(ctx, hub2Namespace)).Should(Succeed())

		// create managedclustermigration CR
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
		genericProducer, err := genericproducer.NewGenericProducer(transportConfig)
		Expect(err).NotTo(HaveOccurred())
		migrationReconciler = migration.NewMigrationReconciler(mgr.GetClient(), genericProducer, false)
		Expect(migrationReconciler.SetupWithManager(mgr)).To(Succeed())

		Eventually(func() error {
			return mgr.GetClient().Get(ctx, types.NamespacedName{
				Name:      "migration",
				Namespace: "hub2",
			}, &v1beta1.ManagedServiceAccount{})
		}, 3*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("should have bootstrap secret generated after managedserviceaccount is reconciled", func() {
		msa := &v1beta1.ManagedServiceAccount{}
		Expect(mgr.GetClient().Get(ctx, types.NamespacedName{
			Name:      "migration",
			Namespace: "hub2",
		}, msa)).To(Succeed())

		// mimic the managedserviceaccount is reconciled
		msa.Spec.Rotation.Validity = metav1.Duration{Duration: 3600 * time.Hour}
		Expect(mgr.GetClient().Update(ctx, msa)).To(Succeed())

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
			if fromEvent.Source() != constants.CloudEventSourceGlobalHub {
				return fmt.Errorf("fromEvent source is not correct")
			}
			if toEvent.Source() != constants.CloudEventSourceGlobalHub {
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
		Expect(mgr.GetClient().Delete(ctx, &v1beta1.ManagedServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "migration",
				Namespace: "hub2",
			},
		})).To(Succeed())

		Eventually(func() error {
			return mgr.GetClient().Get(ctx, types.NamespacedName{
				Name:      "migration",
				Namespace: "hub2",
			}, &v1beta1.ManagedServiceAccount{})
		}, 3*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("should have managedserviceaccount deleted when migration is deleted", func() {
		Expect(mgr.GetClient().Delete(ctx, migrationInstance)).To(Succeed())

		Eventually(func() bool {
			msa := &v1beta1.ManagedServiceAccount{}
			err := mgr.GetClient().Get(ctx, types.NamespacedName{
				Name:      "migration",
				Namespace: "hub2",
			}, msa)
			return apierrors.IsNotFound(err)
		}, 1*time.Second, 100*time.Millisecond).Should(BeTrue())
	})

	AfterAll(func() {
		hub2Namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "hub2"}}
		Expect(mgr.GetClient().Delete(ctx, hub2Namespace)).Should(Succeed())
	})
})
