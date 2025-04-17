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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/migration"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	migrationbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	genericconsumer "github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ = Describe("migration", Ordered, func() {
	var migrationInstance *migrationv1alpha1.ManagedClusterMigration
	var genericConsumer *genericconsumer.GenericConsumer
	var testCtx context.Context
	var testCtxCancel context.CancelFunc
	var sourceHubEvent, destinationHubEvent *cloudevents.Event
	BeforeAll(func() {
		testCtx, testCtxCancel = context.WithCancel(ctx)
		var err error
		genericConsumer, err = genericconsumer.NewGenericConsumer(
			transportConfig,
			[]string{transportConfig.KafkaCredential.SpecTopic},
		)
		Expect(err).NotTo(HaveOccurred())

		// start the consumer
		By("start the consumer")
		go func() {
			if err := genericConsumer.Start(testCtx); err != nil {
				logf.Log.Error(err, "error to start the chan consumer")
			}
		}()
		Expect(err).NotTo(HaveOccurred())

		// deletion interval
		migration.SetDeleteDuration(5 * time.Second)

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
						sourceHubEvent = evt
						fmt.Println("hub1 received event", sourceHubEvent.Type())
					case "hub2":
						destinationHubEvent = evt
						fmt.Println("hub2 received event", destinationHubEvent.Type())
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

		// verify the re-create
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

	It("should send the initialized event into source hub", func() {
		Eventually(func() error {
			payload := sourceHubEvent.Data()
			if payload == nil {
				return fmt.Errorf("wait for the event sent to source hub")
			}

			Expect(sourceHubEvent.Type()).To(Equal(constants.CloudEventTypeMigrationFrom))

			// handle migration.from cloud event
			managedClusterMigrationEvent := &migrationbundle.ManagedClusterMigrationFromEvent{}
			if err := json.Unmarshal(payload, managedClusterMigrationEvent); err != nil {
				return err
			}

			Expect(managedClusterMigrationEvent.Stage).To(Equal(migrationv1alpha1.MigrationResourceInitialized))
			Expect(managedClusterMigrationEvent.ToHub).To(Equal("hub2"))
			Expect(managedClusterMigrationEvent.ManagedClusters[0]).To(Equal("cluster1"))

			return nil
		}, 3*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("should initialize the migration cluster", func() {
		// The initializing resources is synced in the database by the status path from source hub to global hub
		addonConfig := addonv1.KlusterletAddonConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster1",
				Namespace: "cluster1",
			},
		}
		addonConfigPayload, err := json.Marshal(addonConfig)
		Expect(err).To(Succeed())
		mcm := &models.ManagedClusterMigration{
			FromHub:     "hub1",
			ToHub:       "hub2",
			ClusterName: "cluster1",
			Payload:     addonConfigPayload,
			Stage:       migrationv1alpha1.MigrationResourceInitialized,
		}
		Expect(db.Save(mcm).Error).To(Succeed())

		migrationInstance = &migrationv1alpha1.ManagedClusterMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "migration",
				Namespace: utils.GetDefaultNamespace(),
			},
		}

		Eventually(func() error {
			err := mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(migrationInstance), migrationInstance)
			if err != nil {
				return err
			}

			if migrationInstance.Status.Phase != migrationv1alpha1.PhaseMigrating {
				return fmt.Errorf("wait for the migration Migrating to be ready: %s", migrationInstance.Status.Phase)
			}

			registeredCond := meta.FindStatusCondition(migrationInstance.Status.Conditions,
				migrationv1alpha1.MigrationClusterRegistered)
			if registeredCond == nil {
				return fmt.Errorf("the registering condition should appears in the migration CR")
			}

			utils.PrettyPrint(migrationInstance.Status)

			return nil
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("should register the migration cluster", func() {
		// create the migrating cluster in status table
		By("create the migrating cluster in database")
		clusterPayload, err := json.Marshal(&clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster1",
			},
			Spec: clusterv1.ManagedClusterSpec{
				ManagedClusterClientConfigs: []clusterv1.ClientConfig{
					{
						URL:      "https://example.com",
						CABundle: []byte("test"),
					},
				},
			},
		})
		Expect(err).To(Succeed())
		err = db.Model(models.ManagedCluster{}).Create(&models.ManagedCluster{
			ClusterID:   "23e5ae9e-c6b2-4793-be6b-2e52f870df10",
			LeafHubName: "hub1",
			Payload:     clusterPayload,
			Error:       database.ErrorNone,
		}).Error
		Expect(err).To(Succeed())

		// get the register event in the source hub
		Eventually(func() error {
			payload := sourceHubEvent.Data()
			if payload == nil {
				return fmt.Errorf("wait for the event sent to from hub")
			}
			if sourceHubEvent.Type() != constants.CloudEventTypeMigrationFrom {
				return fmt.Errorf("source hub should receive event %s, but got %s", constants.CloudEventTypeMigrationFrom,
					sourceHubEvent.Type())
			}
			managedClusterMigrationEvent := &migrationbundle.ManagedClusterMigrationFromEvent{}
			if err := json.Unmarshal(payload, managedClusterMigrationEvent); err != nil {
				return err
			}
			if managedClusterMigrationEvent.Stage != migrationv1alpha1.MigrationClusterRegistered {
				return fmt.Errorf("source hub should receive %s event, but got %s",
					migrationv1alpha1.MigrationClusterRegistered, managedClusterMigrationEvent.Stage)
			}
			if managedClusterMigrationEvent.BootstrapSecret == nil {
				return fmt.Errorf("source hub should receive Migrating event with bootstrapSecret")
			}

			return nil
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

		// mock the cluster is registered
		err = db.Model(&models.ManagedCluster{}).Where("cluster_id = ?",
			"23e5ae9e-c6b2-4793-be6b-2e52f870df10").Update("leaf_hub_name", "hub2").Error
		Expect(err).To(Succeed())

		// check the migration status is registered
		migrationInstance = &migrationv1alpha1.ManagedClusterMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "migration",
				Namespace: utils.GetDefaultNamespace(),
			},
		}
		Eventually(func() error {
			err := mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(migrationInstance), migrationInstance)
			if err != nil {
				return err
			}

			// db should changed into deploying
			registeredCond := meta.FindStatusCondition(migrationInstance.Status.Conditions,
				migrationv1alpha1.MigrationClusterRegistered)
			if registeredCond.Status == metav1.ConditionFalse {
				return fmt.Errorf("the registering condition should be set into true")
			}

			registeredCond = meta.FindStatusCondition(migrationInstance.Status.Conditions,
				migrationv1alpha1.MigrationResourceDeployed)
			if registeredCond == nil {
				return fmt.Errorf("the ResourceDeployed condition should appears in the migration CR")
			}
			utils.PrettyPrint(migrationInstance.Status)
			return nil
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("should deploy the resource for the migration cluster", func() {
		// check the migration status is deployed
		migrationInstance = &migrationv1alpha1.ManagedClusterMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "migration",
				Namespace: utils.GetDefaultNamespace(),
			},
		}
		Eventually(func() error {
			err := mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(migrationInstance), migrationInstance)
			if err != nil {
				return err
			}

			// db should changed into deploying
			registeredCond := meta.FindStatusCondition(migrationInstance.Status.Conditions,
				migrationv1alpha1.MigrationResourceDeployed)
			if registeredCond.Status == metav1.ConditionFalse {
				return fmt.Errorf("the deploying condition should be set into true")
			}
			utils.PrettyPrint(migrationInstance.Status)
			return nil
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("should complete the process for the migration cluster", func() {
		// get the clean up event in the source hub
		Eventually(func() error {
			payload := sourceHubEvent.Data()
			if payload == nil {
				return fmt.Errorf("wait for the event sent to source hub")
			}
			if sourceHubEvent.Type() != constants.CloudEventTypeMigrationFrom {
				return fmt.Errorf("source hub should receive event %s, but got %s", constants.CloudEventTypeMigrationFrom,
					sourceHubEvent.Type())
			}
			managedClusterMigrationEvent := &migrationbundle.ManagedClusterMigrationFromEvent{}
			if err := json.Unmarshal(payload, managedClusterMigrationEvent); err != nil {
				return err
			}
			if managedClusterMigrationEvent.Stage != migrationv1alpha1.MigrationResourceCleaned {
				return fmt.Errorf("source hub should receive %s event, but got %s",
					migrationv1alpha1.MigrationResourceCleaned, managedClusterMigrationEvent.Stage)
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

		// mock the clean up confirmation from source hub
		err := db.Model(&models.ManagedClusterMigration{}).Where("cluster_name = ?", "cluster1").Update(
			"stage", migrationv1alpha1.MigrationResourceCleaned).Error
		Expect(err).To(Succeed())

		// check the migration status is deleted, managedServiceAccount is deleted
		migrationInstance = &migrationv1alpha1.ManagedClusterMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "migration",
				Namespace: utils.GetDefaultNamespace(),
			},
		}
		Eventually(func() error {
			err := mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(migrationInstance), migrationInstance)
			if err != nil {
				return err
			}
			if migrationInstance.Status.Phase != migrationv1alpha1.PhaseCompleted {
				return fmt.Errorf("migration status should be completed")
			}
			return nil
		}, 20*time.Second, 100*time.Millisecond).Should(Succeed())

		Eventually(func() bool {
			msa := &v1beta1.ManagedServiceAccount{}
			err := mgr.GetClient().Get(testCtx, types.NamespacedName{
				Name:      "migration",
				Namespace: "hub2",
			}, msa)
			return apierrors.IsNotFound(err)
		}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
	})
})
