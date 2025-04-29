package migration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

		// create kafka user
		kafkaUser1 := &kafkav1beta2.KafkaUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hub1-kafka-user",
				Namespace: utils.GetDefaultNamespace(),
			},
			Spec: &kafkav1beta2.KafkaUserSpec{
				Authorization: &kafkav1beta2.KafkaUserSpecAuthorization{
					Type: kafkav1beta2.KafkaUserSpecAuthorizationTypeSimple,
					Acls: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{},
				},
			},
		}
		kafkaUser2 := &kafkav1beta2.KafkaUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hub2-kafka-user",
				Namespace: utils.GetDefaultNamespace(),
			},
			Spec: &kafkav1beta2.KafkaUserSpec{
				Authorization: &kafkav1beta2.KafkaUserSpecAuthorization{
					Type: kafkav1beta2.KafkaUserSpecAuthorizationTypeSimple,
					Acls: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{},
				},
			},
		}
		Expect(mgr.GetClient().Create(testCtx, kafkaUser1)).To(Succeed())
		Expect(mgr.GetClient().Create(testCtx, kafkaUser2)).To(Succeed())

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

		// create a managed hub cluster
		By("create a managed hub cluster")
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

		// create a managed hub cluster
		By("create a managed cluster")
		cluster1 := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster1",
				Labels: map[string]string{
					"foo": "bar",
				},
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
		Expect(mgr.GetClient().Create(testCtx, cluster1)).To(Succeed())

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

	It("should pass the validation for the migrating", func() {
		migrationInstance = &migrationv1alpha1.ManagedClusterMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "migration",
				Namespace: utils.GetDefaultNamespace(),
			},
		}

		// validating: hub
		By("validating: not found hub")
		Eventually(func() error {
			err := mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(migrationInstance), migrationInstance)
			if err != nil {
				return err
			}

			cond := meta.FindStatusCondition(migrationInstance.Status.Conditions, migrationv1alpha1.ConditionTypeValidated)
			if cond == nil {
				return fmt.Errorf("should find the condition: %s", migrationv1alpha1.ConditionTypeValidated)
			}
			if cond.Status == metav1.ConditionFalse && cond.Reason == migration.ConditionReasonHubClusterNotFound {
				return nil
			}
			return fmt.Errorf("should throw hub not found error")
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed())
		By("create the hub cluster in database")
		err := db.Model(models.LeafHub{}).Create(&models.LeafHub{
			LeafHubName: "hub1",
			ClusterID:   "00000000-0000-0000-0000-000000000001",
			Payload:     []byte(`{}`),
		}).Error
		Expect(err).To(Succeed())
		err = db.Model(models.LeafHub{}).Create(&models.LeafHub{
			LeafHubName: "hub2",
			ClusterID:   "00000000-0000-0000-0000-000000000002",
			Payload:     []byte(`{}`),
		}).Error
		Expect(err).To(Succeed())
		hub1 := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "hub1",
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
		err = mgr.GetClient().Create(ctx, hub1)
		Expect(err).To(Succeed())
		hub2 := &clusterv1.ManagedCluster{
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
		err = mgr.GetClient().Create(ctx, hub2)
		if !errors.IsAlreadyExists(err) {
			Expect(err).To(Succeed())
		}

		// add a label to trigger the reconcile
		err = mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(migrationInstance), migrationInstance)
		Expect(err).To(Succeed())
		migrationInstance.Labels = map[string]string{"test": "foo"}
		err = mgr.GetClient().Update(ctx, migrationInstance)
		Expect(err).To(Succeed())

		// validating: not found cluster
		By("validating: not found cluster")
		Eventually(func() error {
			err = mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(migrationInstance), migrationInstance)
			if err != nil {
				return err
			}

			cond := meta.FindStatusCondition(migrationInstance.Status.Conditions, migrationv1alpha1.ConditionTypeValidated)
			if cond == nil {
				return fmt.Errorf("should find the condition: %s", migrationv1alpha1.ConditionTypeValidated)
			}
			if cond.Status == metav1.ConditionFalse && cond.Reason == migration.ConditionReasonClusterNotFound {
				return nil
			}
			utils.PrettyPrint(migrationInstance.Status)
			return fmt.Errorf("should throw error cluster not found")
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed())

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

		// validating: validated -> true
		Eventually(func() error {
			err := mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(migrationInstance), migrationInstance)
			if err != nil {
				return err
			}

			cond := meta.FindStatusCondition(migrationInstance.Status.Conditions, migrationv1alpha1.ConditionTypeValidated)
			if cond.Status == metav1.ConditionTrue && cond.Reason == migration.ConditionReasonResourceValidated &&
				migrationInstance.Status.Phase == migrationv1alpha1.PhaseInitializing {
				return nil
			}
			utils.PrettyPrint(migrationInstance.Status)
			return fmt.Errorf("should get the initializing resource, but got %s", migrationInstance.Status.Phase)
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed())
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
			if sourceHubEvent == nil {
				return fmt.Errorf("wait for the event sent to from hub")
			}
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

			Expect(managedClusterMigrationEvent.Stage).To(Equal(migrationv1alpha1.PhaseInitializing))
			Expect(managedClusterMigrationEvent.ToHub).To(Equal("hub2"))
			Expect(managedClusterMigrationEvent.ManagedClusters[0]).To(Equal("cluster1"))

			return nil
		}, 3*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("should grant the proper permission to kafka user", func() {
		Eventually(func() error {
			user1 := &kafkav1beta2.KafkaUser{}
			Expect(mgr.GetClient().Get(testCtx, types.NamespacedName{
				Name:      "hub1-kafka-user",
				Namespace: utils.GetDefaultNamespace(),
			}, user1)).To(Succeed())

			Expect(user1.Spec.Authorization.Acls[0].Operations[0]).To(Equal(kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemWrite))

			user2 := &kafkav1beta2.KafkaUser{}
			Expect(mgr.GetClient().Get(testCtx, types.NamespacedName{
				Name:      "hub2-kafka-user",
				Namespace: utils.GetDefaultNamespace(),
			}, user2)).To(Succeed())

			Expect(user2.Spec.Authorization.Acls[0].Operations).Should(ContainElements(
				kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemDescribe,
				kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
			))

			return nil
		}, 3*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("should initialize the migration cluster", func() {
		// hub1 confirmation
		migration.SetFinished(string(migrationInstance.GetUID()), "hub1", migrationv1alpha1.PhaseInitializing)

		// hub2 confirmation
		migration.SetFinished(string(migrationInstance.GetUID()), "hub2", migrationv1alpha1.PhaseInitializing)

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

			if migrationInstance.Status.Phase != migrationv1alpha1.PhaseInitializing &&
				migrationInstance.Status.Phase != migrationv1alpha1.PhaseCompleted {
				utils.PrettyPrint(migrationInstance.Status)
				return fmt.Errorf("wait for the migration Migrating to be ready: %s", migrationInstance.Status.Phase)
			}

			initCond := meta.FindStatusCondition(migrationInstance.Status.Conditions,
				migrationv1alpha1.ConditionTypeInitialized)
			if initCond == nil {
				utils.PrettyPrint(migrationInstance.Status)
				return fmt.Errorf("the initializing condition should appears in the migration CR")
			}
			return nil
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed())
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

			// mock the target hub report result
			migration.SetFinished(string(migrationInstance.GetUID()), migrationInstance.Spec.To,
				migrationv1alpha1.PhaseDeploying)

			// db should changed into deploying
			deployedCond := meta.FindStatusCondition(migrationInstance.Status.Conditions,
				migrationv1alpha1.ConditionTypeDeployed)
			if deployedCond == nil || deployedCond.Status == metav1.ConditionFalse {
				utils.PrettyPrint(migrationInstance.Status)
				return fmt.Errorf("the deploying condition should be set into true")
			}
			// utils.PrettyPrint(migrationInstance.Status)
			return nil
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("should register the migration cluster", func() {
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
			if managedClusterMigrationEvent.Stage != migrationv1alpha1.PhaseRegistering {
				return fmt.Errorf("source hub should receive %s event, but got %s",
					migrationv1alpha1.PhaseRegistering, managedClusterMigrationEvent.Stage)
			}

			return nil
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

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

			// mock the target hub report result
			migration.SetFinished(string(migrationInstance.GetUID()), migrationInstance.Spec.To,
				migrationv1alpha1.PhaseRegistering)

			// db should changed into deploying
			registeredCond := meta.FindStatusCondition(migrationInstance.Status.Conditions,
				migrationv1alpha1.ConditionTypeRegistered)
			if registeredCond == nil || registeredCond.Status == metav1.ConditionFalse {
				return fmt.Errorf("the registering condition should be set into true")
			}

			return nil
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	// It("should complete the process for the migration cluster", func() {
	// 	// // get the clean up event in the source hub
	// 	// Eventually(func() error {
	// 	// 	payload := sourceHubEvent.Data()
	// 	// 	if payload == nil {
	// 	// 		return fmt.Errorf("wait for the event sent to source hub")
	// 	// 	}
	// 	// 	if sourceHubEvent.Type() != constants.CloudEventTypeMigrationFrom {
	// 	// 		return fmt.Errorf("source hub should receive event %s, but got %s", constants.CloudEventTypeMigrationFrom,
	// 	// 			sourceHubEvent.Type())
	// 	// 	}
	// 	// 	managedClusterMigrationEvent := &migrationbundle.ManagedClusterMigrationFromEvent{}
	// 	// 	if err := json.Unmarshal(payload, managedClusterMigrationEvent); err != nil {
	// 	// 		return err
	// 	// 	}
	// 	// 	// utils.PrettyPrint(managedClusterMigrationEvent)
	// 	// 	if managedClusterMigrationEvent.Stage != migrationv1alpha1.ConditionTypeCleaned {
	// 	// 		return fmt.Errorf("source hub should receive %s event, but got %s",
	// 	// 			migrationv1alpha1.ConditionTypeCleaned, managedClusterMigrationEvent.Stage)
	// 	// 	}
	// 	// 	return nil
	// 	// }, 10*time.Second, 100*time.Millisecond).Should(Succeed())

	// 	// mock the clean up confirmation from source hub
	// 	err := db.Model(&models.ManagedClusterMigration{}).Where("cluster_name = ?", "cluster1").Update(
	// 		"stage", migrationv1alpha1.ConditionTypeCleaned).Error
	// 	Expect(err).To(Succeed())

	// 	// check the migration status is deleted, managedServiceAccount is deleted
	// 	migrationInstance = &migrationv1alpha1.ManagedClusterMigration{
	// 		ObjectMeta: metav1.ObjectMeta{
	// 			Name:      "migration",
	// 			Namespace: utils.GetDefaultNamespace(),
	// 		},
	// 	}
	// 	Eventually(func() error {
	// 		err := mgr.GetClient().Get(testCtx, client.ObjectKeyFromObject(migrationInstance), migrationInstance)
	// 		if err != nil {
	// 			return err
	// 		}
	// 		if migrationInstance.Status.Phase != migrationv1alpha1.PhaseCompleted {
	// 			return fmt.Errorf("migration status should be completed")
	// 		}
	// 		return nil
	// 	}, 20*time.Second, 100*time.Millisecond).Should(Succeed())

	// 	Eventually(func() bool {
	// 		msa := &v1beta1.ManagedServiceAccount{}
	// 		err := mgr.GetClient().Get(testCtx, types.NamespacedName{
	// 			Name:      "migration",
	// 			Namespace: "hub2",
	// 		}, msa)
	// 		return apierrors.IsNotFound(err)
	// 	}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
	// })

	// It("should receive the migration resources from gh-migration topic", func() {
	// 	By("the cluster1 namespace should not exist")
	// 	Expect(mgr.GetClient().Get(testCtx, types.NamespacedName{Name: "cluster1"}, &corev1.Namespace{})).To(HaveOccurred())

	// 	By("send migration resources to the topic")
	// 	fromSyncer := syncers.NewManagedClusterMigrationFromSyncer(mgr.GetClient(), nil, transportConfig)
	// 	migrationProducer, err := producer.NewGenericProducer(transportConfig, transportConfig.KafkaCredential.MigrationTopic, nil)
	// 	Expect(err).NotTo(HaveOccurred())
	// 	fromSyncer.SetMigrationProducer(migrationProducer)
	// 	Expect(fromSyncer.SendSourceClusterMigrationResources(testCtx, string(migrationInstance.GetUID()),
	// 		[]string{"cluster1"}, "hub1", "hub2")).NotTo(HaveOccurred())

	// 	By("receive migration resources from the topic")
	// 	toSyncer := syncers.NewManagedClusterMigrationToSyncer(mgr.GetClient(), nil, transportConfig)
	// 	go func() {
	// 		Expect(toSyncer.StartMigrationConsumer(testCtx, string(migrationInstance.GetUID()))).NotTo(HaveOccurred())
	// 	}()

	// 	By("check the namespace is created by syncMigrationResources method")
	// 	Eventually(func() error {
	// 		ns := &corev1.Namespace{}
	// 		err := mgr.GetClient().Get(testCtx, types.NamespacedName{Name: "cluster1"}, ns)
	// 		if err != nil {
	// 			return err
	// 		}
	// 		return nil
	// 	}, 10*time.Second, 1*time.Millisecond).Should(Succeed())
	// })
})
