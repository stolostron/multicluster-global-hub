package spec

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/hubha"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ = Describe("HubStatusSyncer", func() {
	var (
		cluster1Name = "test-cluster-1"
		cluster2Name = "test-cluster-2"
		cluster3Name = "test-cluster-3"
		activeHub    = "active-hub-1"
	)

	BeforeEach(func() {
		// Create test ManagedClusters with different initial hubAcceptsClient values
		cluster1 := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: cluster1Name,
			},
			Spec: clusterv1.ManagedClusterSpec{
				HubAcceptsClient: true, // Will change to false when active hub is healthy
			},
		}
		Expect(runtimeClient.Create(ctx, cluster1)).To(Succeed())

		cluster2 := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: cluster2Name,
			},
			Spec: clusterv1.ManagedClusterSpec{
				HubAcceptsClient: false, // Will change to true when active hub goes down
			},
		}
		Expect(runtimeClient.Create(ctx, cluster2)).To(Succeed())

		cluster3 := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: cluster3Name,
			},
			Spec: clusterv1.ManagedClusterSpec{
				HubAcceptsClient: true,
			},
		}
		Expect(runtimeClient.Create(ctx, cluster3)).To(Succeed())
	})

	AfterEach(func() {
		// Clean up test ManagedClusters
		cluster1 := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: cluster1Name,
			},
		}
		_ = runtimeClient.Delete(ctx, cluster1)

		cluster2 := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: cluster2Name,
			},
		}
		_ = runtimeClient.Delete(ctx, cluster2)

		cluster3 := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: cluster3Name,
			},
		}
		_ = runtimeClient.Delete(ctx, cluster3)
	})

	It("should update hubAcceptsClient when active hub goes inactive (failover)", func() {
		// Send hub status update: active hub goes inactive (failover scenario)
		update := hubha.HubStatusUpdate{
			HubName:         activeHub,
			Status:          constants.HubStatusInactive,
			ManagedClusters: []string{cluster1Name, cluster2Name},
		}
		payloadBytes, err := json.Marshal(update)
		Expect(err).NotTo(HaveOccurred())

		evt := utils.ToCloudEvent(constants.HubStatusUpdateMsgKey, constants.CloudEventGlobalHubClusterName, leafHubName, payloadBytes)
		err = genericProducer.SendEvent(ctx, evt)
		Expect(err).NotTo(HaveOccurred())

		// Verify hubAcceptsClient is set to true for affected clusters (failover)
		Eventually(func() error {
			mc1 := &clusterv1.ManagedCluster{}
			if err := runtimeClient.Get(ctx, client.ObjectKey{Name: cluster1Name}, mc1); err != nil {
				return err
			}
			if !mc1.Spec.HubAcceptsClient {
				return fmt.Errorf("cluster1 hubAcceptsClient should be true (failover), got false")
			}

			mc2 := &clusterv1.ManagedCluster{}
			if err := runtimeClient.Get(ctx, client.ObjectKey{Name: cluster2Name}, mc2); err != nil {
				return err
			}
			if !mc2.Spec.HubAcceptsClient {
				return fmt.Errorf("cluster2 hubAcceptsClient should be true (failover), got false")
			}

			// cluster3 should not be affected
			mc3 := &clusterv1.ManagedCluster{}
			if err := runtimeClient.Get(ctx, client.ObjectKey{Name: cluster3Name}, mc3); err != nil {
				return err
			}
			if !mc3.Spec.HubAcceptsClient {
				return fmt.Errorf("cluster3 hubAcceptsClient should remain true, got false")
			}

			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should update hubAcceptsClient when active hub becomes active again", func() {
		// First set clusters to failover state (hubAcceptsClient=true)
		update := hubha.HubStatusUpdate{
			HubName:         activeHub,
			Status:          constants.HubStatusInactive,
			ManagedClusters: []string{cluster1Name, cluster2Name},
		}
		payloadBytes, err := json.Marshal(update)
		Expect(err).NotTo(HaveOccurred())

		evt := utils.ToCloudEvent(constants.HubStatusUpdateMsgKey, constants.CloudEventGlobalHubClusterName, leafHubName, payloadBytes)
		err = genericProducer.SendEvent(ctx, evt)
		Expect(err).NotTo(HaveOccurred())

		// Wait for failover to complete
		Eventually(func() error {
			mc1 := &clusterv1.ManagedCluster{}
			if err := runtimeClient.Get(ctx, client.ObjectKey{Name: cluster1Name}, mc1); err != nil {
				return err
			}
			if !mc1.Spec.HubAcceptsClient {
				return fmt.Errorf("waiting for failover - cluster1 hubAcceptsClient should be true")
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		// Now send active status - hub recovered
		update.Status = constants.HubStatusActive
		payloadBytes, err = json.Marshal(update)
		Expect(err).NotTo(HaveOccurred())

		evt = utils.ToCloudEvent(constants.HubStatusUpdateMsgKey, constants.CloudEventGlobalHubClusterName, leafHubName, payloadBytes)
		err = genericProducer.SendEvent(ctx, evt)
		Expect(err).NotTo(HaveOccurred())

		// Verify hubAcceptsClient is set to false (normal state - hub is healthy)
		Eventually(func() error {
			mc1 := &clusterv1.ManagedCluster{}
			if err := runtimeClient.Get(ctx, client.ObjectKey{Name: cluster1Name}, mc1); err != nil {
				return err
			}
			if mc1.Spec.HubAcceptsClient {
				return fmt.Errorf("cluster1 hubAcceptsClient should be false (hub recovered), got true")
			}

			mc2 := &clusterv1.ManagedCluster{}
			if err := runtimeClient.Get(ctx, client.ObjectKey{Name: cluster2Name}, mc2); err != nil {
				return err
			}
			if mc2.Spec.HubAcceptsClient {
				return fmt.Errorf("cluster2 hubAcceptsClient should be false (hub recovered), got true")
			}

			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should handle non-existent clusters gracefully", func() {
		// Send hub status update for clusters that don't exist
		update := hubha.HubStatusUpdate{
			HubName:         activeHub,
			Status:          constants.HubStatusInactive,
			ManagedClusters: []string{"non-existent-cluster-1", "non-existent-cluster-2"},
		}
		payloadBytes, err := json.Marshal(update)
		Expect(err).NotTo(HaveOccurred())

		evt := utils.ToCloudEvent(constants.HubStatusUpdateMsgKey, constants.CloudEventGlobalHubClusterName, leafHubName, payloadBytes)
		err = genericProducer.SendEvent(ctx, evt)
		Expect(err).NotTo(HaveOccurred())

		// Should not crash or error - just log and skip
		// Wait a bit to ensure processing happened
		time.Sleep(2 * time.Second)

		// Verify existing clusters are unaffected
		mc1 := &clusterv1.ManagedCluster{}
		Expect(runtimeClient.Get(ctx, client.ObjectKey{Name: cluster1Name}, mc1)).To(Succeed())
		Expect(mc1.Spec.HubAcceptsClient).To(BeTrue()) // Should still be true from BeforeEach
	})

	It("should handle multiple clusters in one message", func() {
		// Send hub status update for all three clusters
		update := hubha.HubStatusUpdate{
			HubName:         activeHub,
			Status:          constants.HubStatusInactive,
			ManagedClusters: []string{cluster1Name, cluster2Name, cluster3Name},
		}
		payloadBytes, err := json.Marshal(update)
		Expect(err).NotTo(HaveOccurred())

		evt := utils.ToCloudEvent(constants.HubStatusUpdateMsgKey, constants.CloudEventGlobalHubClusterName, leafHubName, payloadBytes)
		err = genericProducer.SendEvent(ctx, evt)
		Expect(err).NotTo(HaveOccurred())

		// Verify all three clusters are updated
		Eventually(func() error {
			for _, clusterName := range []string{cluster1Name, cluster2Name, cluster3Name} {
				mc := &clusterv1.ManagedCluster{}
				if err := runtimeClient.Get(ctx, client.ObjectKey{Name: clusterName}, mc); err != nil {
					return err
				}
				if !mc.Spec.HubAcceptsClient {
					return fmt.Errorf("%s hubAcceptsClient should be true (failover), got false", clusterName)
				}
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should ignore hub status update when it's not for standby hub", func() {
		// Configure this agent as active hub (not standby)
		// The hub status syncer should only be registered for standby hubs
		// This test verifies the registration logic in spec.go

		// For now, we can't easily test this without modifying agent config
		// and restarting the controller. This is more of an architectural
		// validation that the syncer is only registered for standby hubs.
		// The registration logic in agent/pkg/spec/spec.go handles this.
		Skip("Skipping registration test - requires agent config modification")
	})
})
