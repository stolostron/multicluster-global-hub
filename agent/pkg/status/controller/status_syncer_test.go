// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller_test

import (
	"fmt"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var _ = Describe("Agent Status Syncer", Ordered, func() {
	It("Should be able to sync control-info", func() {
		By("Create configmap that contains the global-hub configurations")
		Expect(kubeClient.Create(ctx, globalHubConfigConfigMap)).Should(Succeed())

		By("Check the control-info bundle can be read from kafka consumer")
		Eventually(func() bool {
			msg, _ := <-kafkaConsumer.GetMessageChan()
			partition, offset, msgID, receivedBundle, err := processKafkaMessage(msg)
			if err != nil {
				return false
			}
			fmt.Printf("========== received msgID: %s\n", msgID)
			fmt.Printf("========== received bundle: %v\n", receivedBundle)
			if msgID != constants.ControlInfoMsgKey {
				return false
			}
			if msgID == constants.ControlInfoMsgKey {
				topicPartition := kafka.TopicPartition{
					Topic:     &statusTopic,
					Partition: partition,
					Offset:    offset,
				}
				if _, err := kafkaConsumer.Consumer().CommitOffsets([]kafka.TopicPartition{
					topicPartition,
				}); err != nil {
					return false
				}
				controlInfoBundle, ok := receivedBundle.(*statusbundle.ControlInfoBundle)
				if !ok {
					return false
				}
				if controlInfoBundle.GetLeafHubName() != leafHubName {
					return false
				}
			}
			return true
		}, 30*time.Second, 1*time.Second).Should(BeTrue())
	})

	It("Should be able to sync managed clusters", func() {
		By("Create managed clusters in testing regional hub")
		Expect(kubeClient.Create(ctx, testMangedCluster)).NotTo(HaveOccurred())

		By("Check the managed cluster status bundle can be read from kafka consumer")
		Eventually(func() bool {
			msg, _ := <-kafkaConsumer.GetMessageChan()
			partition, offset, msgID, receivedBundle, err := processKafkaMessage(msg)
			if err != nil {
				return false
			}
			fmt.Printf("========== received msgID: %s\n", msgID)
			fmt.Printf("========== received bundle: %v\n", receivedBundle)
			if msgID != constants.ManagedClustersMsgKey {
				fmt.Printf("========== unexpected msgID, want %s, got: %s\n", constants.ManagedClustersMsgKey, msgID)
				return false
			}
			if msgID == constants.ManagedClustersMsgKey {
				topicPartition := kafka.TopicPartition{
					Topic:     &statusTopic,
					Partition: partition,
					Offset:    offset,
				}
				if _, err := kafkaConsumer.Consumer().CommitOffsets([]kafka.TopicPartition{
					topicPartition,
				}); err != nil {
					fmt.Printf("========== failed to commit kaka message: %v\n", err)
					return false
				}
				managedClustersStatusBundle, ok := receivedBundle.(*statusbundle.ManagedClustersStatusBundle)
				if !ok {
					fmt.Printf("========== unexpected received bundle type, want ManagedClustersStatusBundle\n")
					return false
				}
				managedClusterObjs := managedClustersStatusBundle.GetObjects()
				if len(managedClusterObjs) != 1 {
					fmt.Printf("========== unexpected object number in received bundle, want 1, got %d\n", len(managedClusterObjs))
					return false
				}
				managedCluster := managedClusterObjs[0].(*clusterv1.ManagedCluster)
				if testMangedCluster.GetName() != managedCluster.GetName() ||
					testMangedCluster.GetNamespace() != managedCluster.GetNamespace() ||
					!apiequality.Semantic.DeepDerivative(testMangedCluster.Spec, managedCluster.Spec) {
					fmt.Printf("========== object not equal, want %v, got %v\n",
						testMangedCluster.Spec, managedCluster.Spec)
					return false
				}
			}
			return true
		}, 30*time.Second, 1*time.Second).Should(BeTrue())
	})

	It("Should be able to sync global policies", func() {
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

		By("Check the global policy status bundle can be read from kafka consumer")
		Eventually(func() bool {
			msg, _ := <-kafkaConsumer.GetMessageChan()
			partition, offset, msgID, receivedBundle, err := processKafkaMessage(msg)
			if err != nil {
				return false
			}
			fmt.Printf("========== received msgID: %s\n", msgID)
			fmt.Printf("========== received bundle: %v\n", receivedBundle)
			if msgID != constants.ClustersPerPolicyMsgKey {
				fmt.Printf("========== unexpected msgID, want %s, got: %s\n",
					constants.ClustersPerPolicyMsgKey, msgID)
				return false
			}
			if msgID == constants.ClustersPerPolicyMsgKey {
				topicPartition := kafka.TopicPartition{
					Topic:     &statusTopic,
					Partition: partition,
					Offset:    offset,
				}
				if _, err := kafkaConsumer.Consumer().CommitOffsets([]kafka.TopicPartition{
					topicPartition,
				}); err != nil {
					fmt.Printf("========== failed to commit kaka message: %v\n", err)
					return false
				}
				clustersPerPolicyBundle, ok := receivedBundle.(*statusbundle.ClustersPerPolicyBundle)
				if !ok {
					fmt.Printf("========== unexpected received bundle type, want ClustersPerPolicyBundle\n")
					return false
				}
				policyGenericComplianceStatusObjs := clustersPerPolicyBundle.GetObjects()
				if len(policyGenericComplianceStatusObjs) != 1 {
					fmt.Printf("========== unexpected object number in received bundle, want 1, got %d\n", len(policyGenericComplianceStatusObjs))
					return false
				}
				policyGenericComplianceStatus := policyGenericComplianceStatusObjs[0].(*status.PolicyGenericComplianceStatus)
				if policyGenericComplianceStatus.PolicyID != testGlobalPolicyOriginUID ||
					len(policyGenericComplianceStatus.CompliantClusters) != 1 ||
					len(policyGenericComplianceStatus.NonCompliantClusters) != 2 ||
					len(policyGenericComplianceStatus.UnknownComplianceClusters) != 0 {
					fmt.Printf("========== unexpected compliance status: %v\n",
						policyGenericComplianceStatus.CompliantClusters)
					fmt.Printf("========== unexpected compliance status: %v\n",
						policyGenericComplianceStatus.NonCompliantClusters)
					fmt.Printf("========== unexpected compliance status: %v\n",
						policyGenericComplianceStatus.UnknownComplianceClusters)
					return false
				}
				if policyGenericComplianceStatus.CompliantClusters[0] != "hub1-mc1" {
					fmt.Printf("========== unexpected compliance cluster name: %v\n", policyGenericComplianceStatus)
					return false
				}
				if policyGenericComplianceStatus.NonCompliantClusters[0] != "hub1-mc2" {
					fmt.Printf("========== unexpected noncompliance cluster name: %v\n", policyGenericComplianceStatus)
					return false
				}
				if policyGenericComplianceStatus.NonCompliantClusters[1] != "hub1-mc3" {
					fmt.Printf("========== unexpected noncompliance cluster name: %v\n", policyGenericComplianceStatus)
					return false
				}
			}
			return true
		}, 30*time.Second, 1*time.Second).Should(BeTrue())

		By("Check the global policy complete compliance bundle can be read from kafka consumer")
		Eventually(func() bool {
			msg, _ := <-kafkaConsumer.GetMessageChan()
			partition, offset, msgID, receivedBundle, err := processKafkaMessage(msg)
			if err != nil {
				return false
			}
			fmt.Printf("========== received msgID: %s\n", msgID)
			fmt.Printf("========== received bundle: %v\n", receivedBundle)
			if msgID != constants.PolicyCompleteComplianceMsgKey {
				fmt.Printf("========== unexpected msgID, want %s, got: %s\n",
					constants.PolicyCompleteComplianceMsgKey, msgID)
				return false
			}
			if msgID == constants.PolicyCompleteComplianceMsgKey {
				topicPartition := kafka.TopicPartition{
					Topic:     &statusTopic,
					Partition: partition,
					Offset:    offset,
				}
				if _, err := kafkaConsumer.Consumer().CommitOffsets([]kafka.TopicPartition{
					topicPartition,
				}); err != nil {
					fmt.Printf("========== failed to commit kaka message: %v\n", err)
					return false
				}
				policyCompleteComplianceStatusBundle, ok :=
					receivedBundle.(*statusbundle.CompleteComplianceStatusBundle)
				if !ok {
					fmt.Printf("========== unexpected received bundle type, want CompleteComplianceStatusBundle\n")
					return false
				}
				policyCompleteComplianceStatusObjs := policyCompleteComplianceStatusBundle.GetObjects()
				if len(policyCompleteComplianceStatusObjs) != 1 {
					fmt.Printf("========== unexpected object number in received bundle, want 1, got %d\n",
						len(policyCompleteComplianceStatusObjs))
					return false
				}
				policyCompleteComplianceStatus := policyCompleteComplianceStatusObjs[0].(*status.PolicyCompleteComplianceStatus)
				if len(policyCompleteComplianceStatus.NonCompliantClusters) != 2 ||
					len(policyCompleteComplianceStatus.UnknownComplianceClusters) != 0 {
					fmt.Printf("========== unexpected compliance status: %v\n", policyCompleteComplianceStatus)
					return false
				}
				if policyCompleteComplianceStatus.NonCompliantClusters[0] != "hub1-mc2" {
					fmt.Printf("========== unexpected noncompliance cluster name: %v\n", policyCompleteComplianceStatus)
					return false
				}
				if policyCompleteComplianceStatus.NonCompliantClusters[1] != "hub1-mc3" {
					fmt.Printf("========== unexpected noncompliance cluster name: %v\n", policyCompleteComplianceStatus)
					return false
				}
			}
			return true
		}, 30*time.Second, 1*time.Second).Should(BeTrue())
	})

	It("Should be able to sync global policy compliance via policy complete compliance bundle when policy compliance status is updated", func() {
		By("Update global policy compliance status in testing regional hub")
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
					ComplianceState:  policyv1.Compliant,
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

		By("Check the global policy delta compliance bundle with updated compliance status can be read from kafka consumer")
		Eventually(func() bool {
			msg, _ := <-kafkaConsumer.GetMessageChan()
			partition, offset, msgID, receivedBundle, err := processKafkaMessage(msg)
			if err != nil {
				return false
			}
			fmt.Printf("========== received msgID: %s\n", msgID)
			fmt.Printf("========== received bundle: %v\n", receivedBundle)
			if msgID != constants.PolicyDeltaComplianceMsgKey {
				fmt.Printf("========== unexpected msgID, want %s, got: %s\n",
					constants.PolicyDeltaComplianceMsgKey, msgID)
				return false
			}
			if msgID == constants.PolicyDeltaComplianceMsgKey {
				topicPartition := kafka.TopicPartition{
					Topic:     &statusTopic,
					Partition: partition,
					Offset:    offset,
				}
				if _, err := kafkaConsumer.Consumer().CommitOffsets([]kafka.TopicPartition{
					topicPartition,
				}); err != nil {
					fmt.Printf("========== failed to commit kaka message: %v\n", err)
					return false
				}
				policyDeltaComplianceStatusBundle, ok :=
					receivedBundle.(*statusbundle.DeltaComplianceStatusBundle)
				if !ok {
					fmt.Printf("========== unexpected received bundle type, want DeltaComplianceStatusBundle\n")
					return false
				}
				policyDeltaComplianceStatusObjs := policyDeltaComplianceStatusBundle.GetObjects()
				if len(policyDeltaComplianceStatusObjs) != 1 {
					fmt.Printf("========== unexpected object number in received bundle, want 1, got %d\n",
						len(policyDeltaComplianceStatusObjs))
					return false
				}
				policyGenericComplianceStatus := policyDeltaComplianceStatusObjs[0].(*status.PolicyGenericComplianceStatus)
				if len(policyGenericComplianceStatus.CompliantClusters) != 1 ||
					len(policyGenericComplianceStatus.NonCompliantClusters) != 0 ||
					len(policyGenericComplianceStatus.UnknownComplianceClusters) != 0 {
					fmt.Printf("========== unexpected compliance status: %v\n", policyGenericComplianceStatus)
					return false
				}
				// if policyGenericComplianceStatus.NonCompliantClusters[0] != "hub1-mc3" {
				// 	fmt.Printf("========== unexpected noncompliance cluster name: %v\n", policyGenericComplianceStatus)
				// 	return false
				// }
			}
			return true
		}, 30*time.Second, 1*time.Second).Should(BeTrue())
	})

	It("Should be able to sync global placementrules", func() {
		By("Create global placementrule in testing regional hub")
		Expect(kubeClient.Create(ctx, testGlobalPlacementRule)).ToNot(HaveOccurred())

		By("Check the global placementrule status bundle can be read from kafka consumer")
		Eventually(func() bool {
			msg, _ := <-kafkaConsumer.GetMessageChan()
			partition, offset, msgID, receivedBundle, err := processKafkaMessage(msg)
			if err != nil {
				return false
			}
			fmt.Printf("========== received msgID: %s\n", msgID)
			fmt.Printf("========== received bundle: %v\n", receivedBundle)
			if msgID != constants.PlacementRuleMsgKey {
				fmt.Printf("========== unexpected msgID, want %s, got: %s\n", constants.PlacementRuleMsgKey, msgID)
				return false
			}
			if msgID == constants.PlacementRuleMsgKey {
				topicPartition := kafka.TopicPartition{
					Topic:     &statusTopic,
					Partition: partition,
					Offset:    offset,
				}
				if _, err := kafkaConsumer.Consumer().CommitOffsets([]kafka.TopicPartition{
					topicPartition,
				}); err != nil {
					fmt.Printf("========== failed to commit kaka message: %v\n", err)
					return false
				}
				placementRulesStatusBundle, ok := receivedBundle.(*statusbundle.PlacementRulesBundle)
				if !ok {
					fmt.Printf("========== unexpected received bundle type, want placementRulesStatusBundle\n")
					return false
				}
				placementRuleObjs := placementRulesStatusBundle.GetObjects()
				if len(placementRuleObjs) != 1 {
					fmt.Printf("========== unexpected object number in received bundle, want 1, got %d\n", len(placementRuleObjs))
					return false
				}
				placementRule := placementRuleObjs[0].(*placementrulev1.PlacementRule)
				if testGlobalPlacementRule.GetName() != placementRule.GetName() ||
					testGlobalPlacementRule.GetNamespace() != placementRule.GetNamespace() ||
					!apiequality.Semantic.DeepDerivative(testGlobalPlacementRule.Spec, placementRule.Spec) {
					fmt.Printf("========== object not equal, want %v, got %v\n",
						testGlobalPlacementRule.Spec, placementRule.Spec)
					return false
				}
			}
			return true
		}, 30*time.Second, 1*time.Second).Should(BeTrue())
	})

	It("Should be able to sync global placements", func() {
		By("Create global placement in testing regional hub")
		Expect(kubeClient.Create(ctx, testGlobalPlacement)).ToNot(HaveOccurred())

		By("Check the global placement status bundle can be read from kafka consumer")
		Eventually(func() bool {
			msg, _ := <-kafkaConsumer.GetMessageChan()
			partition, offset, msgID, receivedBundle, err := processKafkaMessage(msg)
			if err != nil {
				return false
			}
			fmt.Printf("========== received msgID: %s\n", msgID)
			fmt.Printf("========== received bundle: %v\n", receivedBundle)
			if msgID != constants.PlacementMsgKey {
				fmt.Printf("========== unexpected msgID, want %s, got: %s\n", constants.PlacementMsgKey, msgID)
				return false
			}
			if msgID == constants.PlacementMsgKey {
				topicPartition := kafka.TopicPartition{
					Topic:     &statusTopic,
					Partition: partition,
					Offset:    offset,
				}
				if _, err := kafkaConsumer.Consumer().CommitOffsets([]kafka.TopicPartition{
					topicPartition,
				}); err != nil {
					fmt.Printf("========== failed to commit kaka message: %v\n", err)
					return false
				}
				placementsStatusBundle, ok := receivedBundle.(*statusbundle.PlacementsBundle)
				if !ok {
					fmt.Printf("========== unexpected received bundle type, want PlacementsBundle\n")
					return false
				}
				placementObjs := placementsStatusBundle.GetObjects()
				if len(placementObjs) != 1 {
					fmt.Printf("========== unexpected object number in received bundle, want 1, got %d\n", len(placementObjs))
					return false
				}
				placement := placementObjs[0].(*clusterv1beta1.Placement)
				if testGlobalPlacement.GetName() != placement.GetName() ||
					testGlobalPlacement.GetNamespace() != placement.GetNamespace() {
					// testGlobalPlacement.GetNamespace() != placement.GetNamespace() ||
					// !apiequality.Semantic.DeepDerivative(testGlobalPlacement.Spec, placement.Spec) {
					fmt.Printf("========== object not equal, want %v, got %v\n",
						testGlobalPlacement.Spec, placement.Spec)
					return false
				}
			}
			return true
		}, 30*time.Second, 1*time.Second).Should(BeTrue())
	})

	It("Should be able to sync placementdecisions", func() {
		By("Create placementdecision in testing regional hub")
		Expect(kubeClient.Create(ctx, testPlacementDecision)).ToNot(HaveOccurred())

		By("Check the placementdecision status bundle can be read from kafka consumer")
		Eventually(func() bool {
			msg, _ := <-kafkaConsumer.GetMessageChan()
			partition, offset, msgID, receivedBundle, err := processKafkaMessage(msg)
			if err != nil {
				return false
			}
			fmt.Printf("========== received msgID: %s\n", msgID)
			fmt.Printf("========== received bundle: %v\n", receivedBundle)
			if msgID != constants.PlacementDecisionMsgKey {
				fmt.Printf("========== unexpected msgID, want %s, got: %s\n",
					constants.PlacementDecisionMsgKey, msgID)
				return false
			}
			if msgID == constants.PlacementDecisionMsgKey {
				topicPartition := kafka.TopicPartition{
					Topic:     &statusTopic,
					Partition: partition,
					Offset:    offset,
				}
				if _, err := kafkaConsumer.Consumer().CommitOffsets([]kafka.TopicPartition{
					topicPartition,
				}); err != nil {
					fmt.Printf("========== failed to commit kaka message: %v\n", err)
					return false
				}
				placementDecisionsStatusBundle, ok := receivedBundle.(*statusbundle.PlacementDecisionsBundle)
				if !ok {
					fmt.Printf("========== unexpected received bundle type, want PlacementDecisionsBundle\n")
					return false
				}
				placementDecisionObjs := placementDecisionsStatusBundle.GetObjects()
				if len(placementDecisionObjs) != 1 {
					fmt.Printf("========== unexpected object number in received bundle, want 1, got %d\n", len(placementDecisionObjs))
					return false
				}
				placementDecision := placementDecisionObjs[0].(*clusterv1beta1.PlacementDecision)
				if testPlacementDecision.GetName() != placementDecision.GetName() ||
					testPlacementDecision.GetNamespace() != placementDecision.GetNamespace() ||
					!apiequality.Semantic.DeepDerivative(testPlacementDecision.Status, placementDecision.Status) {
					fmt.Printf("========== object not equal, want %v, got %v\n",
						testPlacementDecision.Status, placementDecision.Status)
					return false
				}
			}
			return true
		}, 30*time.Second, 1*time.Second).Should(BeTrue())
	})

	It("Should be able to sync subscriptionreports", func() {
		By("Create subscriptionreport in testing regional hub")
		Expect(kubeClient.Create(ctx, testSubscriptionReport)).ToNot(HaveOccurred())

		By("Check the subscriptionreport status bundle can be read from kafka consumer")
		Eventually(func() bool {
			msg, _ := <-kafkaConsumer.GetMessageChan()
			partition, offset, msgID, receivedBundle, err := processKafkaMessage(msg)
			if err != nil {
				return false
			}
			fmt.Printf("========== received msgID: %s\n", msgID)
			fmt.Printf("========== received bundle: %v\n", receivedBundle)
			if msgID != constants.SubscriptionReportMsgKey {
				fmt.Printf("========== unexpected msgID, want %s, got: %s\n",
					constants.SubscriptionReportMsgKey, msgID)
				return false
			}
			if msgID == constants.SubscriptionReportMsgKey {
				topicPartition := kafka.TopicPartition{
					Topic:     &statusTopic,
					Partition: partition,
					Offset:    offset,
				}
				if _, err := kafkaConsumer.Consumer().CommitOffsets([]kafka.TopicPartition{
					topicPartition,
				}); err != nil {
					return false
				}
				subscriptionReportsStatusBundle, ok :=
					receivedBundle.(*statusbundle.SubscriptionReportsBundle)
				if !ok {
					fmt.Printf("========== unexpected received bundle type, want SubscriptionReportsBundle\n")
					return false
				}
				subscriptionReportObjs := subscriptionReportsStatusBundle.GetObjects()
				if len(subscriptionReportObjs) != 1 {
					fmt.Printf("========== unexpected object number in received bundle, want 1, got %d\n", len(subscriptionReportObjs))
					return false
				}
				subscriptionReport := subscriptionReportObjs[0].(*appsv1alpha1.SubscriptionReport)
				if testSubscriptionReport.GetName() != subscriptionReport.GetName() ||
					testSubscriptionReport.GetNamespace() != subscriptionReport.GetNamespace() ||
					!apiequality.Semantic.DeepDerivative(testSubscriptionReport.ReportType, subscriptionReport.ReportType) ||
					!apiequality.Semantic.DeepDerivative(testSubscriptionReport.Summary, subscriptionReport.Summary) ||
					!apiequality.Semantic.DeepDerivative(testSubscriptionReport.Results, subscriptionReport.Results) ||
					!apiequality.Semantic.DeepDerivative(testSubscriptionReport.Resources, subscriptionReport.Resources) {
					fmt.Printf("========== object not equal, want %v, got %v\n",
						testSubscriptionReport, subscriptionReport)
					return false
				}
			}
			return true
		}, 30*time.Second, 1*time.Second).Should(BeTrue())
	})
})
