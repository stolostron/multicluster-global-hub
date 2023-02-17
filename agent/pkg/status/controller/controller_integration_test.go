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

	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var msgIDBundleCreateFuncMap = map[string]status.CreateBundleFunction{
	constants.ControlInfoMsgKey:              statusbundle.NewControlInfoBundle,
	constants.ManagedClustersMsgKey:          statusbundle.NewManagedClustersStatusBundle,
	constants.ClustersPerPolicyMsgKey:        statusbundle.NewClustersPerPolicyBundle,
	constants.PolicyCompleteComplianceMsgKey: statusbundle.NewCompleteComplianceStatusBundle,
	constants.PolicyDeltaComplianceMsgKey:    statusbundle.NewDeltaComplianceStatusBundle,
	constants.SubscriptionStatusMsgKey:       statusbundle.NewSubscriptionStatusesBundle,
	constants.SubscriptionReportMsgKey:       statusbundle.NewSubscriptionReportsBundle,
	constants.PlacementRuleMsgKey:            statusbundle.NewPlacementRulesBundle,
	constants.PlacementMsgKey:                statusbundle.NewPlacementsBundle,
	constants.PlacementDecisionMsgKey:        statusbundle.NewPlacementDecisionsBundle,
}

var _ = Describe("Agent Status Controller", Ordered, func() {
	var testGlobalPolicyOriginUID string
	var testGlobalPolicy *policyv1.Policy
	BeforeAll(func() {
		testGlobalPolicyOriginUID = "test-globalpolicy-uid"
		testGlobalPolicy = &policyv1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-globalpolicy-1",
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
	})

	It("should be able to sync control-info", func() {
		By("Create configmap that contains the global-hub configurations")
		Expect(kubeClient.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.GHConfigCMName,
				Namespace: constants.GHSystemNamespace,
				Annotations: map[string]string{
					constants.OriginOwnerReferenceAnnotation: "testing",
				},
			},
			Data: map[string]string{"aggregationLevel": "full", "enableLocalPolicies": "true"},
		})).Should(Succeed())

		By("Check the control-info bundle can be read from cloudevents consumer")
		Eventually(func() error {
			message := <-consumer.MessageChan()

			statusBundle, err := getStatusBundle(message, constants.ControlInfoMsgKey)
			if err != nil {
				return err
			}
			fmt.Printf("========== received %s with statusBundle: %v\n", message.ID, statusBundle)

			_, ok := statusBundle.(*statusbundle.ControlInfoBundle)
			if !ok {
				return errors.New("unexpected received bundle type, want ControlInfoBundle")
			}
			// Expect(message.Payload).Should(ContainSubstring(globalHubConfigMapUID))
			return nil
		}, 30*time.Second, 1*time.Second).Should(Succeed())
	})

	It("should be able to sync managed clusters", func() {
		By("Create managed clusters in testing regional hub")
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
			fmt.Printf("========== received %s with statusBundle: %v\n", message.ID, statusBundle)

			managedClustersStatusBundle, ok := statusBundle.(*statusbundle.ManagedClustersStatusBundle)
			if !ok {
				return errors.New("unexpected received bundle type, want ManagedClustersStatusBundle")
			}

			managedClusterObjs := managedClustersStatusBundle.GetObjects()
			if len(managedClusterObjs) != 1 {
				return fmt.Errorf("unexpected object number in received bundle, want 1, got %d\n", len(managedClusterObjs))
			}

			managedCluster := managedClusterObjs[0].(*clusterv1.ManagedCluster)
			if testMangedCluster.GetName() != managedCluster.GetName() ||
				testMangedCluster.GetNamespace() != managedCluster.GetNamespace() ||
				!apiequality.Semantic.DeepDerivative(testMangedCluster.Spec, managedCluster.Spec) {
				return fmt.Errorf("object not equal, want %v, got %v\n", testMangedCluster.Spec, managedCluster.Spec)
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(Succeed())
	})

	It("should be able to sync global policies", func() {
		By("Create global policy in testing regional hub")
		Expect(kubeClient.Create(ctx, testGlobalPolicy)).ToNot(HaveOccurred())

		By("Update global policy status in testing regional hub")
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
		var clustersPerPolicyBundle *statusbundle.ClustersPerPolicyBundle
		var policyCompleteComplianceStatusBundle *statusbundle.CompleteComplianceStatusBundle
		Eventually(func() bool {
			message := <-consumer.MessageChan()

			statusBundle, err := getStatusBundle(message, constants.ClustersPerPolicyMsgKey)
			if err == nil {
				fmt.Printf("========== received %s with statusBundle: %v\n", message.ID, statusBundle)
				ok := false
				clustersPerPolicyBundle, ok = statusBundle.(*statusbundle.ClustersPerPolicyBundle)
				if !ok {
					fmt.Printf("unexpected received bundle type, want ClustersPerPolicyBundle")
				}
			}
			statusBundle, err = getStatusBundle(message, constants.PolicyCompleteComplianceMsgKey)
			if err == nil {
				fmt.Printf("========== received %s with statusBundle: %v\n", message.ID, statusBundle)
				ok := false
				policyCompleteComplianceStatusBundle, ok = statusBundle.(*statusbundle.CompleteComplianceStatusBundle)
				if !ok {
					fmt.Printf("unexpected received bundle type, want ClustersPerPolicyBundle")
				}
			}
			return clustersPerPolicyBundle != nil && policyCompleteComplianceStatusBundle != nil
		}, 30*time.Second, 1*time.Second).Should(BeTrue())

		By("Check the clustersPerPolicyBundle")
		policyGenericComplianceStatusObjs := clustersPerPolicyBundle.GetObjects()
		Expect(len(policyGenericComplianceStatusObjs)).Should(Equal(1))

		policyGenericComplianceStatus := policyGenericComplianceStatusObjs[0].(*status.PolicyGenericComplianceStatus)
		fmt.Println(policyGenericComplianceStatus)

		Expect(policyGenericComplianceStatus.PolicyID).Should(Equal(testGlobalPolicyOriginUID))

		Expect(len(policyGenericComplianceStatus.CompliantClusters)).Should(Equal(1))
		Expect(policyGenericComplianceStatus.CompliantClusters[0]).Should(Equal("hub1-mc1"))
		Expect(len(policyGenericComplianceStatus.NonCompliantClusters)).Should(Equal(2))
		Expect(policyGenericComplianceStatus.NonCompliantClusters[0]).Should(Equal("hub1-mc2"))
		Expect(policyGenericComplianceStatus.NonCompliantClusters[1]).Should(Equal("hub1-mc3"))
		Expect(len(policyGenericComplianceStatus.UnknownComplianceClusters)).Should(Equal(0))

		By("Check the policyCompleteComplianceStatusBundle")
		policyCompleteComplianceStatusObjs := policyCompleteComplianceStatusBundle.GetObjects()
		Expect(len(policyCompleteComplianceStatusObjs)).Should(Equal(1))

		policyCompleteComplianceStatus := policyCompleteComplianceStatusObjs[0].(*status.PolicyCompleteComplianceStatus)
		fmt.Println(policyCompleteComplianceStatus)

		Expect(policyCompleteComplianceStatus.PolicyID).Should(Equal(testGlobalPolicyOriginUID))

		Expect(len(policyCompleteComplianceStatus.NonCompliantClusters)).Should(Equal(2))
		Expect(len(policyCompleteComplianceStatus.UnknownComplianceClusters)).Should(Equal(0))
		Expect(policyCompleteComplianceStatus.NonCompliantClusters[0]).Should(Equal("hub1-mc2"))
		Expect(policyCompleteComplianceStatus.NonCompliantClusters[1]).Should(Equal("hub1-mc3"))
	})

	// TODO: consider to support delta bundle with cloudevents
	// It("Should be able to sync global policy compliance via policy complete compliance bundle when policy
	// compliance status is updated", func() {
	// 	By("Update global policy compliance status in testing regional hub")
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
		By("Create global placementrule in testing regional hub")
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
			fmt.Printf("========== received %s with statusBundle: %v\n", message.ID, statusBundle)

			placementRulesStatusBundle, ok := statusBundle.(*statusbundle.PlacementRulesBundle)
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
				return fmt.Errorf("object not equal, want %v, got %v\n", testGlobalPlacementRule.Spec, placementRule.Spec)
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(Succeed())
	})

	It("Should be able to sync global placements", func() {
		By("Create global placement in testing regional hub")
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
			fmt.Printf("========== received %s with statusBundle: %v\n", message.ID, statusBundle)

			placementsStatusBundle, ok := statusBundle.(*statusbundle.PlacementsBundle)
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
				return fmt.Errorf("========== object not equal, want %v, got %v\n", testGlobalPlacement.Spec, placement.Spec)
			}

			return nil
		}, 30*time.Second, 1*time.Second).Should(Succeed())
	})

	It("should be able to sync placementdecisions", func() {
		By("Create placementdecision in testing regional hub")
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
			fmt.Printf("========== received %s with statusBundle: %v\n", message.ID, statusBundle)

			placementDecisionsStatusBundle, ok := statusBundle.(*statusbundle.PlacementDecisionsBundle)
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
				return fmt.Errorf("object not equal, want %v, got %v\n", testPlacementDecision.Status, placementDecision.Status)
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(Succeed())
	})

	It("should be able to sync subscriptionreports", func() {
		By("Create subscriptionreport in testing regional hub")
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
			fmt.Printf("========== received %s with statusBundle: %v\n", message.ID, statusBundle)

			subscriptionReportsStatusBundle, ok := statusBundle.(*statusbundle.SubscriptionReportsBundle)
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
				return fmt.Errorf("object not equal, want %v, got %v\n", testSubscriptionReport, subscriptionReport)
			}

			return nil
		}, 30*time.Second, 1*time.Second).Should(Succeed())
	})
})

func getStatusBundle(message *transport.Message, key string) (status.Bundle, error) {
	expectedMessageID := fmt.Sprintf("%s.%s", leafHubName, key)
	if message.ID != expectedMessageID {
		return nil, fmt.Errorf("expected messageID %s but got %s", expectedMessageID, message.ID)
	}

	receivedBundle := msgIDBundleCreateFuncMap[key]()
	if err := json.Unmarshal(message.Payload, receivedBundle); err != nil {
		return nil, err
	}
	return receivedBundle, nil
}
