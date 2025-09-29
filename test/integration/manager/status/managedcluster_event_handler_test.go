package status

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// go test ./test/integration/manager/status -v -ginkgo.focus "ManagedClusterEventHandler"
var _ = Describe("ManagedClusterEventHandler", Ordered, func() {
	hubName := "hub-managedcluster-event"
	clusterUUID := "13b2e003-2bdf-4c82-9bdf-f1aa7ccf608d"
	clusterClaimID := "13b2e003-2bdf-4c82-9bdf-f1aa7ccf607c"
	clusterName := "cluster-event-cluster1"

	It("should be able to sync managed cluster event in batch mode", func() {
		By("Create cluster event")
		version := eventversion.NewVersion()
		version.Incr()
		data := event.ManagedClusterEventBundle{}
		data = append(data, &models.ManagedClusterEvent{
			EventNamespace:      "managed-cluster1",
			EventName:           "managed-cluster1.17cd5c3642c43a8a",
			ClusterID:           clusterUUID,
			LeafHubName:         hubName,
			ClusterName:         clusterName,
			Message:             "The managed cluster (managed-cluster1) cannot connect to the hub cluster.",
			Reason:              "AvailableUnknown",
			ReportingController: "registration-controller",
			ReportingInstance:   "registration-controller-cluster-manager-registration-controller-6794cf54d9-j7lgm",
			EventType:           "Warning",
			CreatedAt:           time.Now(),
		})

		evt := ToCloudEvent(hubName, string(enum.ManagedClusterEventType), version, data)

		By("Sync event with transport")
		err := producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())

		By("Check the leaf hubs table")
		Eventually(func() error {
			db := database.GetGorm()
			items := []models.ManagedClusterEvent{}
			if err := db.Find(&items).Error; err != nil {
				return err
			}

			count := 0
			for _, item := range items {
				fmt.Println(">> ", item.ClusterName, item.ClusterID, item.EventName, item.CreatedAt)
				if clusterUUID == item.ClusterID {
					count++
				}
			}
			if count > 0 {
				return nil
			}
			return fmt.Errorf("not found expected resource on the table")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should be able to update managed cluster id", func() {
		By("Create cluster record")
		cluster := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName,
				Namespace: "default",
				UID:       types.UID(clusterUUID),
			},

			Status: clusterv1.ManagedClusterStatus{
				ClusterClaims: []clusterv1.ManagedClusterClaim{
					{
						Name:  constants.ClusterIdClaimName,
						Value: clusterClaimID,
					},
				},
			},
		}
		payload, err := json.Marshal(cluster)
		Expect(err).Should(Succeed())
		db := database.GetGorm()
		db.Create(&models.ManagedCluster{
			LeafHubName: hubName,
			ClusterID:   clusterClaimID,
			Error:       database.ErrorNone,
			Payload:     payload,
		})

		By("Check the cluster_id is updated by the trigger")
		Eventually(func() error {
			db := database.GetGorm()
			items := []models.ManagedClusterEvent{}
			if err := db.Find(&items).Error; err != nil {
				return err
			}

			count := 0
			for _, item := range items {
				fmt.Println(">> ", item.ClusterName, item.ClusterID, item.EventName, item.CreatedAt)
				if clusterClaimID == item.ClusterID {
					count++
				}
			}
			if count > 0 {
				return nil
			}
			return fmt.Errorf("not found expected resource on the table")
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	// Test the single event mode
	It("should be able to sync managed cluster event in single mode", func() {
		By("Create cluster event")
		version := eventversion.NewVersion()
		version.Incr()
		clusterEvent := &models.ManagedClusterEvent{
			EventNamespace:      "managed-cluster1",
			EventName:           "managed-cluster1.17cd5c3642c43a8c",
			ClusterID:           clusterUUID,
			LeafHubName:         hubName,
			ClusterName:         clusterName,
			Message:             "The managed cluster (managed-cluster1) cannot connect to the hub cluster.",
			Reason:              "AvailableUnknown",
			ReportingController: "registration-controller2",
			ReportingInstance:   "registration-controller-cluster-manager-registration-controller-6794cf54d9-j7lgg",
			EventType:           "Warning",
			CreatedAt:           time.Now(),
		}
		data := event.ManagedClusterEventBundle{}
		data = append(data, clusterEvent)
		evt := ToCloudEvent(hubName, string(enum.ManagedClusterEventType), version, data)
		evt.Extensions()[constants.CloudEventExtensionSendMode] = string(constants.EventSendModeSingle)

		By("Sync event with transport")
		err := producer.SendEvent(ctx, *evt)
		Expect(err).Should(Succeed())

		By("Check the leaf hubs table")
		Eventually(func() error {
			db := database.GetGorm()
			items := []models.ManagedClusterEvent{}
			if err := db.Where("event_name = ?", clusterEvent.EventName).Find(&items).Error; err != nil {
				return err
			}

			if len(items) != 1 {
				fmt.Println(">>>>>>>>>>>>>>>>>>> items", items)
				return fmt.Errorf("expected 1 event, got %d", len(items))
			}
			// verify the other filed
			if items[0].ReportingInstance != clusterEvent.ReportingInstance {
				return fmt.Errorf("expected reporting instance %s, got %s", clusterEvent.ReportingInstance, items[0].ReportingInstance)
			}
			if items[0].ReportingController != clusterEvent.ReportingController {
				return fmt.Errorf("expected reporting controller %s, got %s", clusterEvent.ReportingController, items[0].ReportingController)
			}

			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})
