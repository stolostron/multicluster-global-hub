// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/placement"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/subscription"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type CreateBundleFunc func() bundle.ManagerBundle

var msgIDBundleCreateFuncMap = map[string]CreateBundleFunc{
	constants.ManagedClustersMsgKey:         cluster.NewManagerManagedClusterBundle,
	constants.ComplianceMsgKey:              grc.NewManagerComplianceBundle,
	constants.CompleteComplianceMsgKey:      grc.NewManagerCompleteComplianceBundle,
	constants.DeltaComplianceMsgKey:         grc.NewManagerDeltaComplianceBundle,
	constants.SubscriptionStatusMsgKey:      subscription.NewManagerSubscriptionStatusesBundle,
	constants.SubscriptionReportMsgKey:      subscription.NewManagerSubscriptionReportsBundle,
	constants.PlacementRuleMsgKey:           placement.NewManagerPlacementRulesBundle,
	constants.PlacementMsgKey:               placement.NewManagerPlacementsBundle,
	constants.PlacementDecisionMsgKey:       placement.NewManagerPlacementDecisionsBundle,
	constants.HubClusterInfoMsgKey:          cluster.NewManagerHubClusterInfoBundle,
	constants.LocalPolicySpecMsgKey:         grc.NewManagerLocalPolicyBundle,
	constants.LocalPolicyHistoryEventMsgKey: grc.NewManagerLocalReplicatedPolicyEventBundle,
}

var _ = Describe("Agent Status Controller", Ordered, func() {
	testGlobalPolicyOriginUID := "test-globalpolicy-uid"
	testGlobalPolicy := &policyv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-global-policy-1",
			Namespace: "default",
			Annotations: map[string]string{
				constants.OriginOwnerReferenceAnnotation: testGlobalPolicyOriginUID,
			},
		},
		Spec: policyv1.PolicySpec{
			Disabled:        false,
			PolicyTemplates: []*policyv1.PolicyTemplate{},
		},
		Status: policyv1.PolicyStatus{},
	}

	testLocalPolicy := &policyv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-local-policy-spec-1",
			Namespace: "default",
		},
		Spec: policyv1.PolicySpec{
			Disabled:        false,
			PolicyTemplates: []*policyv1.PolicyTemplate{},
		},
	}

	It("Should be able to sync local root policy spec", func() {
		By("Create local policies")
		Expect(kubeClient.Create(ctx, testLocalPolicy)).ToNot(HaveOccurred())

		By("Check the local policies bundle can be read from kafka consumer")
		Eventually(func() error {
			message := <-consumer.MessageChan()
			statusBundle, err := getStatusBundle(message, constants.LocalPolicySpecMsgKey)
			fmt.Printf("========== received %s with statusBundle: %v\n", message.Key, statusBundle)

			printBundle(statusBundle)
			if err != nil {
				return err
			}

			resourceBundle, ok := statusBundle.(*grc.LocalPolicyBundle)
			if !ok {
				return errors.New("unexpected received bundle type")
			}

			policies := resourceBundle.GetObjects()
			if len(policies) != 1 {
				return fmt.Errorf("unexpected object number in received bundle, want 1, got %d\n", len(policies))
			}
			receivedPolicy := policies[0].(*policyv1.Policy)
			if testLocalPolicy.GetName() != receivedPolicy.GetName() ||
				testLocalPolicy.GetNamespace() != receivedPolicy.GetNamespace() {
				return fmt.Errorf("========== object not equal, want %v, got %v\n", testLocalPolicy.GetName(), receivedPolicy.GetName())
			}

			return nil
		}, 30*time.Second, 1*time.Second).Should(Succeed())
	})

	// It("should be able to sync local replicas policy event", func() {
	// 	By("Create namespace and cluster for replicas policy")
	// 	err := kubeClient.Create(ctx, &corev1.Namespace{
	// 		ObjectMeta: metav1.ObjectMeta{
	// 			Name: "cluster1",
	// 		},
	// 	}, &client.CreateOptions{})
	// 	Expect(err).Should(Succeed())

	// 	cluster := &clusterv1.ManagedCluster{
	// 		ObjectMeta: metav1.ObjectMeta{
	// 			Name: "cluster1",
	// 		},
	// 	}
	// 	Expect(kubeClient.Create(ctx, cluster, &client.CreateOptions{})).Should(Succeed())
	// 	cluster.Status = clusterv1.ManagedClusterStatus{
	// 		ClusterClaims: []clusterv1.ManagedClusterClaim{
	// 			{
	// 				Name:  "id.k8s.io",
	// 				Value: "3f406177-34b2-4852-88dd-ff2809680336",
	// 			},
	// 		},
	// 	}
	// 	Expect(kubeClient.Status().Update(ctx, cluster)).Should(Succeed())

	// 	By("Create replicas policy")
	// 	testLocalReplicasPolicy := &policyv1.Policy{
	// 		ObjectMeta: metav1.ObjectMeta{
	// 			Name:      "test-local-policy-spec-1",
	// 			Namespace: "cluster1",
	// 			Labels: map[string]string{
	// 				constants.PolicyEventRootPolicyNameLabelKey: fmt.Sprintf("%s.%s", "default", "test-local-policy-spec-1"),
	// 				constants.PolicyEventClusterNameLabelKey:    "cluster1",
	// 			},
	// 		},
	// 		Spec: policyv1.PolicySpec{
	// 			Disabled:        false,
	// 			PolicyTemplates: []*policyv1.PolicyTemplate{},
	// 		},
	// 	}
	// 	Expect(kubeClient.Create(ctx, testLocalReplicasPolicy)).ToNot(HaveOccurred())

	// 	testLocalReplicasPolicy.Status = policyv1.PolicyStatus{
	// 		ComplianceState: policyv1.NonCompliant,
	// 		Details: []*policyv1.DetailsPerTemplate{
	// 			{
	// 				TemplateMeta: metav1.ObjectMeta{
	// 					Name: "test-local-policy-template",
	// 				},
	// 				History: []policyv1.ComplianceHistory{
	// 					{
	// 						EventName:     "openshift-acm-policies.backplane-mobb-sp.176a8f372323ecad",
	// 						LastTimestamp: metav1.Now(),
	// 						Message: `Compliant; notification - clusterrolebindings [backplane-mobb-c0] found
	// 	as specified, therefore this Object template is compliant`,
	// 					},
	// 				},
	// 			},
	// 		},
	// 	}
	// 	Expect(kubeClient.Status().Update(ctx, testLocalReplicasPolicy)).Should(Succeed())
	// 	fmt.Println("== update replicas policy events", testLocalReplicasPolicy.Namespace, testLocalReplicasPolicy.Name)

	// 	By("Check the local policy events bundle can be read from kafka consumer")
	// 	Eventually(func() error {
	// 		message := <-consumer.MessageChan()
	// 		statusBundle, err := getStatusBundle(message, constants.LocalPolicyHistoryEventMsgKey)
	// 		fmt.Printf("========== received %s with statusBundle: %v\n", message.Key, statusBundle)

	// 		printBundle(statusBundle)
	// 		if err != nil {
	// 			return err
	// 		}

	// 		resourceBundle, ok := statusBundle.(*grc.LocalReplicatedPolicyEventBundle)
	// 		if !ok {
	// 			return errors.New("unexpected received bundle type")
	// 		}

	// 		events := resourceBundle.GetObjects()
	// 		if len(events) == 0 {
	// 			return fmt.Errorf("haven't get the local policy events")
	// 		}
	// 		return nil
	// 	}, 30*time.Second, 1*time.Second).Should(Succeed())
	// })

	It("should be able to sync managed clusters", func() {
		By("Create managed clusters in testing managed hub")
		testMangedCluster := &clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-mc-1",
				Labels: map[string]string{
					"cloud":  "Other",
					"vendor": "Other",
				},
				Annotations: map[string]string{
					"cloud":  "Other",
					"vendor": "Other",
				},
			},
			Spec: clusterv1.ManagedClusterSpec{
				HubAcceptsClient:     true,
				LeaseDurationSeconds: 60,
			},
		}
		Expect(kubeClient.Create(ctx, testMangedCluster)).Should(Succeed())

		By("Check the managed cluster status bundle can be read from cloudevents consumer")
		Eventually(func() error {
			message := <-consumer.MessageChan()

			statusBundle, err := getStatusBundle(message, constants.ManagedClustersMsgKey)
			if err != nil {
				return err
			}
			fmt.Printf("========== received %s with statusBundle: %v\n", message.Key, statusBundle)
			printBundle(statusBundle)

			managedClustersStatusBundle, ok := statusBundle.(*cluster.ManagedClusterBundle)
			if !ok {
				return errors.New("unexpected received bundle type, want ManagedClustersStatusBundle")
			}

			managedClusterObjs := managedClustersStatusBundle.GetObjects()
			if len(managedClusterObjs) < 1 {
				return fmt.Errorf("unexpected object number in received bundle, want >= 1, got %d\n", len(managedClusterObjs))
			}

			for _, obj := range managedClusterObjs {
				managedCluster, _ := obj.(*clusterv1.ManagedCluster)
				if testMangedCluster.GetName() == managedCluster.GetName() &&
					testMangedCluster.GetNamespace() == managedCluster.GetNamespace() &&
					apiequality.Semantic.DeepDerivative(testMangedCluster.Spec, managedCluster.Spec) {
					return nil
				}
			}

			return fmt.Errorf("object not equal, want %v, got %v\n", testMangedCluster.Spec, managedClusterObjs)
		}, 30*time.Second, 1*time.Second).Should(Succeed())
	})

	It("should be able to sync global policies", func() {
		By("Create global policy in testing managed hub")
		Expect(kubeClient.Create(ctx, testGlobalPolicy)).ToNot(HaveOccurred())

		By("Update global policy status in testing managed hub")
		testGlobalPolicyCopy := testGlobalPolicy.DeepCopy()
		testGlobalPolicy.Status = policyv1.PolicyStatus{
			ComplianceState: policyv1.NonCompliant,
			Placement: []*policyv1.Placement{
				{
					PlacementBinding: "test-policy-placement",
					PlacementRule:    "test-policy-placement",
				},
			},
			Status: []*policyv1.CompliancePerClusterStatus{
				{
					ClusterName:      "hub1-mc1",
					ClusterNamespace: "hub1-mc1",
					ComplianceState:  policyv1.Compliant,
				},
				{
					ClusterName:      "hub1-mc2",
					ClusterNamespace: "hub1-mc2",
					ComplianceState:  policyv1.NonCompliant,
				},
				{
					ClusterName:      "hub1-mc3",
					ClusterNamespace: "hub1-mc3",
					ComplianceState:  policyv1.NonCompliant,
				},
			},
		}
		Expect(kubeClient.Status().Patch(ctx, testGlobalPolicy,
			client.MergeFrom(testGlobalPolicyCopy))).ToNot(HaveOccurred())

		By("Check the policy bundles is synced")
		var clustersPerPolicyBundle *grc.ComplianceBundle
		var completeComplianceStatusBundle *grc.CompleteComplianceBundle
		Eventually(func() error {
			message := <-consumer.MessageChan()
			fmt.Printf("========== received %s \n", message.Key)

			complianceTransportKey := fmt.Sprintf("%s.%s", config.GetLeafHubName(), constants.ComplianceMsgKey)
			completeTransportKey := fmt.Sprintf("%s.%s", config.GetLeafHubName(), constants.CompleteComplianceMsgKey)

			var statusBundle bundle.ManagerBundle
			var err error
			var ok bool
			if message.Key == complianceTransportKey {
				statusBundle, err = getStatusBundle(message, constants.ComplianceMsgKey)
				printBundle(statusBundle)

				clustersPerPolicyBundle, ok = statusBundle.(*grc.ComplianceBundle)
				if !ok {
					return fmt.Errorf("unexpected received bundle type, want *grc.ComplianceBundle")
				}
			}

			if message.Key == completeTransportKey {
				statusBundle, err = getStatusBundle(message, constants.CompleteComplianceMsgKey)
				printBundle(statusBundle)

				completeComplianceStatusBundle, ok = statusBundle.(*grc.CompleteComplianceBundle)
				if !ok {
					return fmt.Errorf("unexpected received bundle type, want *grc.CompleteComplianceBundle")
				}
			}

			if err != nil {
				return err
			}

			if clustersPerPolicyBundle == nil {
				return fmt.Errorf("waiting clustersPerPolicyBundle")
			}

			if completeComplianceStatusBundle == nil {
				return fmt.Errorf("waiting completeComplianceStatusBundle")
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(Succeed())

		By("Check the clustersPerPolicyBundle")
		policyGenericComplianceStatusObjs := clustersPerPolicyBundle.GetObjects()
		Expect(len(policyGenericComplianceStatusObjs)).Should(Equal(1))

		completeComplianceObjs := completeComplianceStatusBundle.GetObjects()
		Expect(len(completeComplianceObjs)).Should(Equal(1))

		policyGenericComplianceStatus := policyGenericComplianceStatusObjs[0].(*base.GenericCompliance)
		fmt.Println(policyGenericComplianceStatus)

		Expect(policyGenericComplianceStatus.PolicyID).Should(Equal(testGlobalPolicyOriginUID))

		Expect(len(policyGenericComplianceStatus.CompliantClusters)).Should(Equal(1))
		Expect(policyGenericComplianceStatus.CompliantClusters[0]).Should(Equal("hub1-mc1"))
		Expect(len(policyGenericComplianceStatus.NonCompliantClusters)).Should(Equal(2))
		Expect(policyGenericComplianceStatus.NonCompliantClusters[0]).Should(Equal("hub1-mc2"))
		Expect(policyGenericComplianceStatus.NonCompliantClusters[1]).Should(Equal("hub1-mc3"))
		Expect(len(policyGenericComplianceStatus.UnknownComplianceClusters)).Should(Equal(0))

		// By("Check the policyCompleteComplianceStatusBundle")
		// policyCompleteComplianceStatusObjs := policyCompleteComplianceStatusBundle.GetObjects()
		// Expect(len(policyCompleteComplianceStatusObjs)).Should(Equal(1))

		// policyCompleteComplianceStatus := policyCompleteComplianceStatusObjs[0].(*status.PolicyCompleteComplianceStatus)
		// fmt.Println(policyCompleteComplianceStatus)

		// Expect(policyCompleteComplianceStatus.PolicyID).Should(Equal(testGlobalPolicyOriginUID))

		// Expect(len(policyCompleteComplianceStatus.NonCompliantClusters)).Should(Equal(2))
		// Expect(len(policyCompleteComplianceStatus.UnknownComplianceClusters)).Should(Equal(0))
		// Expect(policyCompleteComplianceStatus.NonCompliantClusters[0]).Should(Equal("hub1-mc2"))
		// Expect(policyCompleteComplianceStatus.NonCompliantClusters[1]).Should(Equal("hub1-mc3"))
	})

	// TODO: consider to support delta bundle with cloudevents
	// It("Should be able to sync global policy compliance via policy complete compliance bundle when policy
	// compliance status is updated", func() {
	// 	By("Update global policy compliance status in testing managed hub")
	// 	testGlobalPolicyCopy := testGlobalPolicy.DeepCopy()
	// 	testGlobalPolicy.Status = policyv1.PolicyStatus{
	// 		ComplianceState: policyv1.NonCompliant,
	// 		Placement: []*policyv1.Placement{
	// 			{
	// 				PlacementBinding: "test-policy-placement",
	// 				PlacementRule:    "test-policy-placement",
	// 			},
	// 		},
	// 		Status: []*policyv1.CompliancePerClusterStatus{
	// 			{
	// 				ClusterName:      "hub1-mc1",
	// 				ClusterNamespace: "hub1-mc1",
	// 				ComplianceState:  policyv1.Compliant,
	// 			},
	// 			{
	// 				ClusterName:      "hub1-mc2",
	// 				ClusterNamespace: "hub1-mc2",
	// 				ComplianceState:  policyv1.Compliant,
	// 			},
	// 			{
	// 				ClusterName:      "hub1-mc3",
	// 				ClusterNamespace: "hub1-mc3",
	// 				ComplianceState:  policyv1.NonCompliant,
	// 			},
	// 		},
	// 	}
	// 	Expect(kubeClient.Status().Patch(ctx, testGlobalPolicy,
	// 		client.MergeFrom(testGlobalPolicyCopy))).ToNot(HaveOccurred())

	// 	By("Check the global policy delta compliance bundle with updated compliance status can be read from kafka consumer")
	// 	Eventually(func() bool {
	// 		msg, _ := <-kafkaConsumer.GetMessageChan()
	// 		partition, offset, msgID, receivedBundle, err := processKafkaMessage(msg)
	// 		if err != nil {
	// 			return false
	// 		}
	// 		fmt.Printf("========== received msgID: %s\n", msgID)
	// 		fmt.Printf("========== received bundle: %v\n", receivedBundle)
	// 		if msgID != constants.PolicyDeltaComplianceMsgKey {
	// 			fmt.Printf("========== unexpected msgID, want %s, got: %s\n",
	// 				constants.PolicyDeltaComplianceMsgKey, msgID)
	// 			return false
	// 		}
	// 		if msgID == constants.PolicyDeltaComplianceMsgKey {
	// 			topicPartition := kafka.TopicPartition{
	// 				Topic:     &statusTopic,
	// 				Partition: partition,
	// 				Offset:    offset,
	// 			}
	// 			if _, err := kafkaConsumer.Consumer().CommitOffsets([]kafka.TopicPartition{
	// 				topicPartition,
	// 			}); err != nil {
	// 				fmt.Printf("========== failed to commit kaka message: %v\n", err)
	// 				return false
	// 			}
	// 			policyDeltaComplianceStatusBundle, ok := receivedBundle.(*statusbundle.DeltaComplianceStatusBundle)
	// 			if !ok {
	// 				fmt.Printf("========== unexpected received bundle type, want DeltaComplianceStatusBundle\n")
	// 				return false
	// 			}
	// 			policyDeltaComplianceStatusObjs := policyDeltaComplianceStatusBundle.GetObjects()
	// 			if len(policyDeltaComplianceStatusObjs) != 1 {
	// 				fmt.Printf("========== unexpected object number in received bundle, want 1, got %d\n",
	// 					len(policyDeltaComplianceStatusObjs))
	// 				return false
	// 			}
	// 			policyGenericComplianceStatus := policyDeltaComplianceStatusObjs[0].(*status.PolicyGenericComplianceStatus)
	// 			if len(policyGenericComplianceStatus.CompliantClusters) != 1 ||
	// 				len(policyGenericComplianceStatus.NonCompliantClusters) != 0 ||
	// 				len(policyGenericComplianceStatus.UnknownComplianceClusters) != 0 {
	// 				fmt.Printf("========== unexpected compliance status: %v\n", policyGenericComplianceStatus)
	// 				return false
	// 			}
	// 			// if policyGenericComplianceStatus.NonCompliantClusters[0] != "hub1-mc3" {
	// 			// 	fmt.Printf("========== unexpected noncompliance cluster name: %v\n", policyGenericComplianceStatus)
	// 			// 	return false
	// 			// }
	// 		}
	// 		return true
	// 	}, 30*time.Second, 1*time.Second).Should(BeTrue())
	// })
	It("should be able to sync global placementrules", func() {
		By("Create global placementrule in testing managed hub")
		testGlobalPlacementRuleOriginUID := "test-globalplacementrule-uid"
		testGlobalPlacementRule := &placementrulev1.PlacementRule{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-globalplacementrule-1",
				Namespace:    "default",
				Annotations: map[string]string{
					constants.OriginOwnerReferenceAnnotation: testGlobalPlacementRuleOriginUID,
				},
			},
			Spec: placementrulev1.PlacementRuleSpec{},
		}
		Expect(kubeClient.Create(ctx, testGlobalPlacementRule)).ToNot(HaveOccurred())

		By("Check the global placementrule status bundle can be read from kafka consumer")
		Eventually(func() error {
			message := <-consumer.MessageChan()

			statusBundle, err := getStatusBundle(message, constants.PlacementRuleMsgKey)
			if err != nil {
				return err
			}
			fmt.Printf("========== received %s with statusBundle: %v\n", message.Key, statusBundle)

			placementRulesStatusBundle, ok := statusBundle.(*placement.PlacementRulesBundle)
			if !ok {
				return errors.New("unexpected received bundle type, want PlacementRulesBundle")
			}
			placementRuleObjs := placementRulesStatusBundle.GetObjects()
			if len(placementRuleObjs) != 1 {
				return fmt.Errorf("unexpected object number in received bundle, want 1, got %d\n", len(placementRuleObjs))
			}

			placementRule := placementRuleObjs[0].(*placementrulev1.PlacementRule)
			if testGlobalPlacementRule.GetName() != placementRule.GetName() ||
				testGlobalPlacementRule.GetNamespace() != placementRule.GetNamespace() ||
				!apiequality.Semantic.DeepDerivative(testGlobalPlacementRule.Spec, placementRule.Spec) {
				return fmt.Errorf("object not equal, want %v, got %v\n",
					testGlobalPlacementRule.Spec, placementRule.Spec)
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(Succeed())
	})

	It("Should be able to sync global placements", func() {
		By("Create global placement in testing managed hub")
		testGlobalPlacementOriginUID := "test-globalplacement-uid"
		testGlobalPlacement := &clusterv1beta1.Placement{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-globalplacement-1",
				Namespace: "default",
				Annotations: map[string]string{
					constants.OriginOwnerReferenceAnnotation: testGlobalPlacementOriginUID,
				},
			},
			Spec: clusterv1beta1.PlacementSpec{},
		}
		Expect(kubeClient.Create(ctx, testGlobalPlacement)).ToNot(HaveOccurred())

		By("Check the global placement status bundle can be read from kafka consumer")
		Eventually(func() error {
			message := <-consumer.MessageChan()

			statusBundle, err := getStatusBundle(message, constants.PlacementMsgKey)
			if err != nil {
				return err
			}
			fmt.Printf("========== received %s with statusBundle: %v\n", message.Key, statusBundle)

			placementsStatusBundle, ok := statusBundle.(*placement.PlacementsBundle)
			if !ok {
				return errors.New("unexpected received bundle type, want PlacementsBundle")
			}

			placementObjs := placementsStatusBundle.GetObjects()
			if len(placementObjs) != 1 {
				return fmt.Errorf("unexpected object number in received bundle, want 1, got %d\n", len(placementObjs))
			}
			placement := placementObjs[0].(*clusterv1beta1.Placement)
			if testGlobalPlacement.GetName() != placement.GetName() ||
				testGlobalPlacement.GetNamespace() != placement.GetNamespace() {
				// testGlobalPlacement.GetNamespace() != placement.GetNamespace() ||
				// !apiequality.Semantic.DeepDerivative(testGlobalPlacement.Spec, placement.Spec) {
				return fmt.Errorf("========== object not equal, want %v, got %v\n",
					testGlobalPlacement.Spec, placement.Spec)
			}

			return nil
		}, 30*time.Second, 1*time.Second).Should(Succeed())
	})

	It("should be able to sync placementdecisions", func() {
		By("Create placementdecision in testing managed hub")
		testPlacementDecision := &clusterv1beta1.PlacementDecision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-placementdecision-1",
				Namespace: "default",
			},
			Status: clusterv1beta1.PlacementDecisionStatus{},
		}
		Expect(kubeClient.Create(ctx, testPlacementDecision)).ToNot(HaveOccurred())

		By("Check the placementdecision status bundle can be read from kafka consumer")
		Eventually(func() error {
			message := <-consumer.MessageChan()
			statusBundle, err := getStatusBundle(message, constants.PlacementDecisionMsgKey)
			if err != nil {
				return err
			}
			fmt.Printf("========== received %s with statusBundle: %v\n", message.Key, statusBundle)

			placementDecisionsStatusBundle, ok := statusBundle.(*placement.PlacementDecisionsBundle)
			if !ok {
				return errors.New("unexpected received bundle type, want PlacementDecisionsBundle")
			}
			placementDecisionObjs := placementDecisionsStatusBundle.GetObjects()
			if len(placementDecisionObjs) != 1 {
				return fmt.Errorf("unexpected object number in received bundle, want 1, got %d\n", len(placementDecisionObjs))
			}
			placementDecision := placementDecisionObjs[0].(*clusterv1beta1.PlacementDecision)
			if testPlacementDecision.GetName() != placementDecision.GetName() ||
				testPlacementDecision.GetNamespace() != placementDecision.GetNamespace() ||
				!apiequality.Semantic.DeepDerivative(testPlacementDecision.Status, placementDecision.Status) {
				return fmt.Errorf("object not equal, want %v, got %v\n",
					testPlacementDecision.Status, placementDecision.Status)
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(Succeed())
	})

	It("should be able to sync subscriptionreports", func() {
		By("Create subscriptionreport in testing managed hub")
		testSubscriptionReport := &appsv1alpha1.SubscriptionReport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-subscriptionreport-1",
				Namespace: "default",
			},
			ReportType: "Application",
			Summary: appsv1alpha1.SubscriptionReportSummary{
				Deployed:          "1",
				InProgress:        "0",
				Failed:            "0",
				PropagationFailed: "0",
				Clusters:          "1",
			},
			Results: []*appsv1alpha1.SubscriptionReportResult{
				{
					Source: "hub1-mc1",
					Result: "deployed",
				},
			},
			Resources: []*corev1.ObjectReference{
				{
					Kind:       "Deployment",
					Namespace:  "default",
					Name:       "nginx-sample",
					APIVersion: "apps/v1",
				},
			},
		}
		Expect(kubeClient.Create(ctx, testSubscriptionReport)).ToNot(HaveOccurred())

		By("Check the subscriptionreport status bundle can be read from kafka consumer")
		Eventually(func() error {
			message := <-consumer.MessageChan()
			statusBundle, err := getStatusBundle(message, constants.SubscriptionReportMsgKey)
			if err != nil {
				return err
			}
			fmt.Printf("========== received %s with statusBundle: %v\n", message.Key, statusBundle)

			subscriptionReportsStatusBundle, ok := statusBundle.(*subscription.SubscriptionReportsBundle)
			if !ok {
				return errors.New("unexpected received bundle type, want PlacementDecisionsBundle")
			}
			subscriptionReportObjs := subscriptionReportsStatusBundle.GetObjects()
			if len(subscriptionReportObjs) != 1 {
				return fmt.Errorf("unexpected object number in received bundle, want 1, got %d\n", len(subscriptionReportObjs))
			}

			subscriptionReport := subscriptionReportObjs[0].(*appsv1alpha1.SubscriptionReport)
			if testSubscriptionReport.GetName() != subscriptionReport.GetName() ||
				testSubscriptionReport.GetNamespace() != subscriptionReport.GetNamespace() ||
				!apiequality.Semantic.DeepDerivative(testSubscriptionReport.ReportType, subscriptionReport.ReportType) ||
				!apiequality.Semantic.DeepDerivative(testSubscriptionReport.Summary, subscriptionReport.Summary) ||
				!apiequality.Semantic.DeepDerivative(testSubscriptionReport.Results, subscriptionReport.Results) ||
				!apiequality.Semantic.DeepDerivative(testSubscriptionReport.Resources, subscriptionReport.Resources) {
				return fmt.Errorf("object not equal, want %v, got %v\n",
					testSubscriptionReport, subscriptionReport)
			}

			return nil
		}, 30*time.Second, 1*time.Second).Should(Succeed())
	})
})

func getStatusBundle(message *transport.Message, key string) (bundle.ManagerBundle, error) {
	expectedMessageID := fmt.Sprintf("%s.%s", leafHubName, key)
	if message.Key != expectedMessageID {
		return nil, fmt.Errorf("expected messageID %s but got %s", expectedMessageID, message.Key)
	}

	receivedBundle := msgIDBundleCreateFuncMap[key]()
	if err := json.Unmarshal(message.Payload, receivedBundle); err != nil {
		return nil, err
	}
	return receivedBundle, nil
}

func printBundle(obj interface{}) {
	b, _ := json.Marshal(obj)
	fmt.Println(string(b))
}
