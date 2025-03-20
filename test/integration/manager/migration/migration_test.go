package migration_test

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
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	bundleevent "github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	genericconsumer "github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ = Describe("migration", Ordered, func() {
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
						fmt.Println("received from event", fromEvent)
					case "hub2":
						toEvent = evt
						fmt.Println("received to event", toEvent)
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

	It("should get the from hub event created", func() {
		Eventually(func() error {
			payload := fromEvent.Data()
			if payload == nil {
				return fmt.Errorf("wait for the event sent to from hub")
			}

			Expect(fromEvent.Type()).To(Equal(constants.CloudEventTypeMigrationFrom))

			// handle migration.from cloud event
			managedClusterMigrationEvent := &bundleevent.ManagedClusterMigrationFromEvent{}
			if err := json.Unmarshal(payload, managedClusterMigrationEvent); err != nil {
				return err
			}

			Expect(managedClusterMigrationEvent.Stage).To(Equal(migrationv1alpha1.PhaseInitializing))
			Expect(managedClusterMigrationEvent.ToHub).To(Equal("hub2"))
			Expect(managedClusterMigrationEvent.ManagedClusters[0]).To(Equal("cluster1"))

			return nil
		}, 3*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	It("should switch the migration phase into migrating", func() {
		// sync the item from hub1 into database
		mcm := &models.ManagedClusterMigration{
			FromHub:     "hub1",
			ToHub:       "hub2",
			ClusterName: "cluster1",
			Payload:     []byte(`{}`),
			Stage:       migrationv1alpha1.PhaseInitializing,
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
				return fmt.Errorf("wait for the migration initializing to be ready")
			}

			utils.PrettyPrint(migrationInstance.Status)

			return nil
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
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
		}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
	})
})
