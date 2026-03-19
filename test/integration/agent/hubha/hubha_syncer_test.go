// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"sync/atomic"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	agentconfigs "github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/hubha"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// mockProducer captures events sent by active syncer
type mockProducer struct {
	events chan cloudevents.Event
	closed atomic.Bool
}

func newMockProducer() *mockProducer {
	return &mockProducer{
		events: make(chan cloudevents.Event, 100),
	}
}

func (m *mockProducer) SendEvent(ctx context.Context, evt cloudevents.Event) error {
	if m.closed.Load() {
		return nil // or return an error
	}

	select {
	case m.events <- evt:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil // Channel full, ignore
	}
}

func (m *mockProducer) Stop() {
	m.closed.Store(true)
	close(m.events)
}

func (m *mockProducer) Reconnect(config *transport.TransportInternalConfig, clusterName string) error {
	return nil
}

var _ = Describe("Hub HA Active Syncer Integration", func() {
	var (
		testNamespace = "default"
		producer      *mockProducer
		ctx           context.Context
		cancel        context.CancelFunc
	)

	BeforeEach(func() {
		producer = newMockProducer()
		ctx, cancel = context.WithCancel(context.Background())

		// Configure agent as active hub
		agentConfig := &agentconfigs.AgentConfig{
			LeafHubName:     "hub1",
			HubRole:         constants.GHHubRoleActive,
			StandbyHub:      "hub2",
			TransportConfig: transportConfig,
		}
		agentconfigs.SetAgentConfig(agentConfig)
	})

	AfterEach(func() {
		if cancel != nil {
			cancel()
		}

		if producer != nil {
			producer.Stop()
		}
	})

	It("should collect and send ConfigMaps to standby hub", func() {
		// Create test ConfigMap with required label
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm",
				Namespace: testNamespace,
				Labels: map[string]string{
					"hive.openshift.io/secret-type": "kubeconfig",
				},
			},
			Data: map[string]string{
				"key": "value",
			},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		// Start the syncer
		err := hubha.StartHubHAActiveSyncer(ctx, mgr, producer)
		Expect(err).NotTo(HaveOccurred())

		// Wait for event to be sent
		Eventually(func() bool {
			select {
			case evt := <-producer.events:
				// Verify event
				Expect(evt.Type()).To(Equal(constants.HubHAResourcesMsgKey))
				Expect(evt.Source()).To(Equal("hub1"))

				// Unmarshal bundle
				bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
				err := evt.DataAs(bundle)
				Expect(err).NotTo(HaveOccurred())

				// Check if our ConfigMap is in the bundle
				for _, obj := range bundle.Resync {
					if obj.GetKind() == "ConfigMap" && obj.GetName() == "test-cm" {
						return true
					}
				}
				return false
			case <-time.After(35 * time.Second): // Wait longer than sync interval (30s)
				return false
			}
		}, 40*time.Second, 1*time.Second).Should(BeTrue())

		// Cleanup
		Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
	})

	It("should filter out resources with velero exclude label", func() {
		ctx := context.Background()

		// Create ConfigMap with velero exclude label
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "excluded-cm",
				Namespace: testNamespace,
				Labels: map[string]string{
					"velero.io/exclude-from-backup": "true",
					"hive.openshift.io/secret-type": "kubeconfig",
				},
			},
			Data: map[string]string{
				"key": "value",
			},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		// Create ConfigMap without exclude label
		cm2 := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "included-cm",
				Namespace: testNamespace,
				Labels: map[string]string{
					"hive.openshift.io/secret-type": "kubeconfig",
				},
			},
			Data: map[string]string{
				"key": "value",
			},
		}
		Expect(k8sClient.Create(ctx, cm2)).To(Succeed())

		// Start the syncer
		err := hubha.StartHubHAActiveSyncer(ctx, mgr, producer)
		Expect(err).NotTo(HaveOccurred())

		// Wait for event
		Eventually(func() bool {
			select {
			case evt := <-producer.events:
				bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
				err := evt.DataAs(bundle)
				Expect(err).NotTo(HaveOccurred())

				hasExcluded := false
				hasIncluded := false

				for _, obj := range bundle.Resync {
					if obj.GetKind() == "ConfigMap" {
						if obj.GetName() == "excluded-cm" {
							hasExcluded = true
						}
						if obj.GetName() == "included-cm" {
							hasIncluded = true
						}
					}
				}

				// Should have included-cm but not excluded-cm
				return !hasExcluded && hasIncluded
			case <-time.After(35 * time.Second):
				return false
			}
		}, 40*time.Second, 1*time.Second).Should(BeTrue())

		// Cleanup
		Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
		Expect(k8sClient.Delete(ctx, cm2)).To(Succeed())
	})

	It("should collect and send Policy resources", func() {
		ctx := context.Background()

		// Create test Policy
		policy := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "policy.open-cluster-management.io/v1",
				"kind":       "Policy",
				"metadata": map[string]interface{}{
					"name":      "active-test-policy",
					"namespace": testNamespace,
				},
				"spec": map[string]interface{}{
					"remediationAction": "inform",
					"disabled":          false,
					"policy-templates": []interface{}{
						map[string]interface{}{
							"objectDefinition": map[string]interface{}{
								"apiVersion": "policy.open-cluster-management.io/v1",
								"kind":       "ConfigurationPolicy",
								"metadata": map[string]interface{}{
									"name": "active-config-policy",
								},
								"spec": map[string]interface{}{
									"severity": "low",
								},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policy)).To(Succeed())

		// Start the syncer
		err := hubha.StartHubHAActiveSyncer(ctx, mgr, producer)
		Expect(err).NotTo(HaveOccurred())

		// Wait for event with Policy
		Eventually(func() bool {
			select {
			case evt := <-producer.events:
				bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
				err := evt.DataAs(bundle)
				Expect(err).NotTo(HaveOccurred())

				for _, obj := range bundle.Resync {
					if obj.GetKind() == "Policy" && obj.GetName() == "active-test-policy" {
						return true
					}
				}
				return false
			case <-time.After(35 * time.Second):
				return false
			}
		}, 40*time.Second, 1*time.Second).Should(BeTrue())

		// Cleanup
		Expect(k8sClient.Delete(ctx, policy)).To(Succeed())
	})

	It("should collect and send Placement resources", func() {
		ctx := context.Background()

		// Create test Placement
		placement := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "cluster.open-cluster-management.io/v1beta1",
				"kind":       "Placement",
				"metadata": map[string]interface{}{
					"name":      "active-test-placement",
					"namespace": testNamespace,
				},
				"spec": map[string]interface{}{
					"clusterSets": []interface{}{"global"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, placement)).To(Succeed())

		// Start the syncer
		err := hubha.StartHubHAActiveSyncer(ctx, mgr, producer)
		Expect(err).NotTo(HaveOccurred())

		// Wait for event with Placement
		Eventually(func() bool {
			select {
			case evt := <-producer.events:
				bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
				err := evt.DataAs(bundle)
				Expect(err).NotTo(HaveOccurred())

				for _, obj := range bundle.Resync {
					if obj.GetKind() == "Placement" && obj.GetName() == "active-test-placement" {
						return true
					}
				}
				return false
			case <-time.After(35 * time.Second):
				return false
			}
		}, 40*time.Second, 1*time.Second).Should(BeTrue())

		// Cleanup
		Expect(k8sClient.Delete(ctx, placement)).To(Succeed())
	})

	It("should collect and send multiple resource types in bundle", func() {
		ctx := context.Background()

		// Create Policy
		policy := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "policy.open-cluster-management.io/v1",
				"kind":       "Policy",
				"metadata": map[string]interface{}{
					"name":      "bundle-policy",
					"namespace": testNamespace,
				},
				"spec": map[string]interface{}{
					"remediationAction": "enforce",
					"disabled":          false,
					"policy-templates": []interface{}{
						map[string]interface{}{
							"objectDefinition": map[string]interface{}{
								"apiVersion": "policy.open-cluster-management.io/v1",
								"kind":       "ConfigurationPolicy",
								"metadata": map[string]interface{}{
									"name": "bundle-config-policy",
								},
								"spec": map[string]interface{}{
									"severity": "low",
								},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policy)).To(Succeed())

		// Create Placement
		placement := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "cluster.open-cluster-management.io/v1beta1",
				"kind":       "Placement",
				"metadata": map[string]interface{}{
					"name":      "bundle-placement",
					"namespace": testNamespace,
				},
				"spec": map[string]interface{}{
					"clusterSets": []interface{}{"default"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, placement)).To(Succeed())

		// Create ConfigMap with label
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bundle-cm",
				Namespace: testNamespace,
				Labels: map[string]string{
					"hive.openshift.io/secret-type": "kubeconfig",
				},
			},
			Data: map[string]string{
				"key": "value",
			},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		// Start the syncer
		err := hubha.StartHubHAActiveSyncer(ctx, mgr, producer)
		Expect(err).NotTo(HaveOccurred())

		// Wait for event with all resources
		Eventually(func() bool {
			select {
			case evt := <-producer.events:
				bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
				err := evt.DataAs(bundle)
				Expect(err).NotTo(HaveOccurred())

				hasPolicy := false
				hasPlacement := false
				hasConfigMap := false

				for _, obj := range bundle.Resync {
					if obj.GetKind() == "Policy" && obj.GetName() == "bundle-policy" {
						hasPolicy = true
					}
					if obj.GetKind() == "Placement" && obj.GetName() == "bundle-placement" {
						hasPlacement = true
					}
					if obj.GetKind() == "ConfigMap" && obj.GetName() == "bundle-cm" {
						hasConfigMap = true
					}
				}

				return hasPolicy && hasPlacement && hasConfigMap
			case <-time.After(35 * time.Second):
				return false
			}
		}, 40*time.Second, 1*time.Second).Should(BeTrue())

		// Cleanup
		Expect(k8sClient.Delete(ctx, policy)).To(Succeed())
		Expect(k8sClient.Delete(ctx, placement)).To(Succeed())
		Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
	})
})

var _ = Describe("Hub HA Standby Syncer Integration", func() {
	var (
		testNamespace = "default"
		syncer        *hubha.HubHAStandbySyncer
	)

	BeforeEach(func() {
		syncer = hubha.NewHubHAStandbySyncer(k8sClient)

		// Configure agent as standby hub
		agentConfig := &agentconfigs.AgentConfig{
			LeafHubName:     "hub2",
			HubRole:         constants.GHHubRoleStandby,
			TransportConfig: transportConfig,
		}
		agentconfigs.SetAgentConfig(agentConfig)
	})

	It("should apply ConfigMap resources from active hub", func() {
		ctx := context.Background()

		// Create bundle with ConfigMap to apply
		bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
		bundle.Resync = []*unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "synced-cm",
						"namespace": testNamespace,
					},
					"data": map[string]interface{}{
						"synced": "true",
					},
				},
			},
		}

		// Create CloudEvent
		evt := cloudevents.NewEvent()
		evt.SetType(constants.HubHAResourcesMsgKey)
		evt.SetSource("hub1")
		err := evt.SetData(cloudevents.ApplicationJSON, bundle)
		Expect(err).NotTo(HaveOccurred())

		// Process event
		err = syncer.Sync(ctx, &evt)
		Expect(err).NotTo(HaveOccurred())

		// Verify ConfigMap was created
		cm := &corev1.ConfigMap{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      "synced-cm",
				Namespace: testNamespace,
			}, cm)
		}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

		Expect(cm.Data["synced"]).To(Equal("true"))

		// Cleanup
		Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
	})

	It("should apply Policy resources from active hub", func() {
		ctx := context.Background()

		// Create bundle with Policy
		bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
		bundle.Resync = []*unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "policy.open-cluster-management.io/v1",
					"kind":       "Policy",
					"metadata": map[string]interface{}{
						"name":      "test-policy",
						"namespace": testNamespace,
					},
					"spec": map[string]interface{}{
						"remediationAction": "inform",
						"disabled":          false,
						"policy-templates": []interface{}{
							map[string]interface{}{
								"objectDefinition": map[string]interface{}{
									"apiVersion": "policy.open-cluster-management.io/v1",
									"kind":       "ConfigurationPolicy",
									"metadata": map[string]interface{}{
										"name": "test-config-policy",
									},
									"spec": map[string]interface{}{
										"severity": "low",
									},
								},
							},
						},
					},
				},
			},
		}

		// Create CloudEvent
		evt := cloudevents.NewEvent()
		evt.SetType(constants.HubHAResourcesMsgKey)
		evt.SetSource("hub1")
		err := evt.SetData(cloudevents.ApplicationJSON, bundle)
		Expect(err).NotTo(HaveOccurred())

		// Process event
		err = syncer.Sync(ctx, &evt)
		Expect(err).NotTo(HaveOccurred())

		// Verify Policy was created
		policy := &unstructured.Unstructured{}
		policy.SetAPIVersion("policy.open-cluster-management.io/v1")
		policy.SetKind("Policy")
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-policy",
				Namespace: testNamespace,
			}, policy)
		}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

		// Verify policy spec
		disabled, _, _ := unstructured.NestedBool(policy.Object, "spec", "disabled")
		Expect(disabled).To(BeFalse())

		// Cleanup
		Expect(k8sClient.Delete(ctx, policy)).To(Succeed())
	})

	It("should apply Placement resources from active hub", func() {
		ctx := context.Background()

		// Create bundle with Placement
		bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
		bundle.Resync = []*unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "cluster.open-cluster-management.io/v1beta1",
					"kind":       "Placement",
					"metadata": map[string]interface{}{
						"name":      "test-placement",
						"namespace": testNamespace,
					},
					"spec": map[string]interface{}{
						"clusterSets": []interface{}{"global"},
						"predicates": []interface{}{
							map[string]interface{}{
								"requiredClusterSelector": map[string]interface{}{
									"labelSelector": map[string]interface{}{
										"matchLabels": map[string]interface{}{
											"environment": "production",
										},
									},
								},
							},
						},
					},
				},
			},
		}

		// Create CloudEvent
		evt := cloudevents.NewEvent()
		evt.SetType(constants.HubHAResourcesMsgKey)
		evt.SetSource("hub1")
		err := evt.SetData(cloudevents.ApplicationJSON, bundle)
		Expect(err).NotTo(HaveOccurred())

		// Process event
		err = syncer.Sync(ctx, &evt)
		Expect(err).NotTo(HaveOccurred())

		// Verify Placement was created
		placement := &unstructured.Unstructured{}
		placement.SetAPIVersion("cluster.open-cluster-management.io/v1beta1")
		placement.SetKind("Placement")
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-placement",
				Namespace: testNamespace,
			}, placement)
		}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

		// Verify placement spec
		clusterSets, _, _ := unstructured.NestedStringSlice(placement.Object, "spec", "clusterSets")
		Expect(clusterSets).To(ContainElement("global"))

		// Cleanup
		Expect(k8sClient.Delete(ctx, placement)).To(Succeed())
	})

	It("should apply PlacementBinding resources from active hub", func() {
		ctx := context.Background()

		// Create bundle with PlacementBinding
		bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
		bundle.Resync = []*unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "policy.open-cluster-management.io/v1",
					"kind":       "PlacementBinding",
					"metadata": map[string]interface{}{
						"name":      "test-placement-binding",
						"namespace": testNamespace,
					},
					"placementRef": map[string]interface{}{
						"apiGroup": "cluster.open-cluster-management.io",
						"kind":     "Placement",
						"name":     "test-placement",
					},
					"subjects": []interface{}{
						map[string]interface{}{
							"apiGroup": "policy.open-cluster-management.io",
							"kind":     "Policy",
							"name":     "test-policy",
						},
					},
				},
			},
		}

		// Create CloudEvent
		evt := cloudevents.NewEvent()
		evt.SetType(constants.HubHAResourcesMsgKey)
		evt.SetSource("hub1")
		err := evt.SetData(cloudevents.ApplicationJSON, bundle)
		Expect(err).NotTo(HaveOccurred())

		// Process event
		err = syncer.Sync(ctx, &evt)
		Expect(err).NotTo(HaveOccurred())

		// Verify PlacementBinding was created
		pb := &unstructured.Unstructured{}
		pb.SetAPIVersion("policy.open-cluster-management.io/v1")
		pb.SetKind("PlacementBinding")
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-placement-binding",
				Namespace: testNamespace,
			}, pb)
		}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

		// Verify binding references
		placementName, _, _ := unstructured.NestedString(pb.Object, "placementRef", "name")
		Expect(placementName).To(Equal("test-placement"))

		// Cleanup
		Expect(k8sClient.Delete(ctx, pb)).To(Succeed())
	})

	It("should apply multiple resource types in single bundle", func() {
		ctx := context.Background()

		// Create bundle with multiple resource types
		bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
		bundle.Resync = []*unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "multi-cm",
						"namespace": testNamespace,
					},
					"data": map[string]interface{}{
						"key": "value",
					},
				},
			},
			{
				Object: map[string]interface{}{
					"apiVersion": "policy.open-cluster-management.io/v1",
					"kind":       "Policy",
					"metadata": map[string]interface{}{
						"name":      "multi-policy",
						"namespace": testNamespace,
					},
					"spec": map[string]interface{}{
						"remediationAction": "enforce",
						"disabled":          false,
						"policy-templates": []interface{}{
							map[string]interface{}{
								"objectDefinition": map[string]interface{}{
									"apiVersion": "policy.open-cluster-management.io/v1",
									"kind":       "ConfigurationPolicy",
									"metadata": map[string]interface{}{
										"name": "multi-config-policy",
									},
									"spec": map[string]interface{}{
										"severity": "low",
									},
								},
							},
						},
					},
				},
			},
			{
				Object: map[string]interface{}{
					"apiVersion": "cluster.open-cluster-management.io/v1beta1",
					"kind":       "Placement",
					"metadata": map[string]interface{}{
						"name":      "multi-placement",
						"namespace": testNamespace,
					},
					"spec": map[string]interface{}{
						"clusterSets": []interface{}{"global"},
					},
				},
			},
		}

		// Create CloudEvent
		evt := cloudevents.NewEvent()
		evt.SetType(constants.HubHAResourcesMsgKey)
		evt.SetSource("hub1")
		err := evt.SetData(cloudevents.ApplicationJSON, bundle)
		Expect(err).NotTo(HaveOccurred())

		// Process event
		err = syncer.Sync(ctx, &evt)
		Expect(err).NotTo(HaveOccurred())

		// Verify all resources were created
		cm := &corev1.ConfigMap{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      "multi-cm",
				Namespace: testNamespace,
			}, cm)
		}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

		policy := &unstructured.Unstructured{}
		policy.SetAPIVersion("policy.open-cluster-management.io/v1")
		policy.SetKind("Policy")
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      "multi-policy",
				Namespace: testNamespace,
			}, policy)
		}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

		placement := &unstructured.Unstructured{}
		placement.SetAPIVersion("cluster.open-cluster-management.io/v1beta1")
		placement.SetKind("Placement")
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      "multi-placement",
				Namespace: testNamespace,
			}, placement)
		}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

		// Cleanup
		Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
		Expect(k8sClient.Delete(ctx, policy)).To(Succeed())
		Expect(k8sClient.Delete(ctx, placement)).To(Succeed())
	})

	It("should update existing resources from active hub", func() {
		ctx := context.Background()

		// Create existing ConfigMap
		existingCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "update-cm",
				Namespace: testNamespace,
			},
			Data: map[string]string{
				"old": "value",
			},
		}
		Expect(k8sClient.Create(ctx, existingCM)).To(Succeed())

		// Create bundle with updated ConfigMap
		bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
		bundle.Update = []*unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "update-cm",
						"namespace": testNamespace,
					},
					"data": map[string]interface{}{
						"new": "value",
					},
				},
			},
		}

		// Create CloudEvent
		evt := cloudevents.NewEvent()
		evt.SetType(constants.HubHAResourcesMsgKey)
		evt.SetSource("hub1")
		err := evt.SetData(cloudevents.ApplicationJSON, bundle)
		Expect(err).NotTo(HaveOccurred())

		// Process event
		err = syncer.Sync(ctx, &evt)
		Expect(err).NotTo(HaveOccurred())

		// Verify ConfigMap was updated
		cm := &corev1.ConfigMap{}
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "update-cm",
				Namespace: testNamespace,
			}, cm)
			if err != nil {
				return false
			}
			_, hasNew := cm.Data["new"]
			return hasNew
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

		Expect(cm.Data["new"]).To(Equal("value"))

		// Cleanup
		Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
	})

	It("should ignore events with wrong type", func() {
		ctx := context.Background()

		// Create event with wrong type
		evt := cloudevents.NewEvent()
		evt.SetType("WrongType")
		evt.SetSource("hub1")

		// Process event - should not error
		err := syncer.Sync(ctx, &evt)
		Expect(err).NotTo(HaveOccurred())
	})
})
