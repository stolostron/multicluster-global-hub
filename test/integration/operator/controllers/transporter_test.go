package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/shared"
	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatortrans "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter/protocol"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	testutils "github.com/stolostron/multicluster-global-hub/test/integration/utils"
)

// go test ./test/integration/operator -ginkgo.focus "transporter" -v
var _ = Describe("transporter", Ordered, func() {
	var mgh *v1alpha4.MulticlusterGlobalHub
	var namespace string
	mghName := "test-mgh"
	BeforeAll(func() {
		namespace = fmt.Sprintf("namespace-%s", rand.String(6))
		config.SetMGHNamespacedName(types.NamespacedName{Namespace: namespace, Name: mghName})
		// mgh
		Expect(runtimeClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})).To(Succeed())
		mgh = &v1alpha4.MulticlusterGlobalHub{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mghName,
				Namespace: namespace,
			},
			Spec: v1alpha4.MulticlusterGlobalHubSpec{
				EnableMetrics: true,
				DataLayerSpec: v1alpha4.DataLayerSpec{
					Kafka: v1alpha4.KafkaSpec{
						KafkaTopics: v1alpha4.KafkaTopics{
							SpecTopic:   "gh-spec",
							StatusTopic: "gh-status.*",
						},
					},
					Postgres: v1alpha4.PostgresSpec{
						Retention: "2y",
					},
				},
			},
		}
		Expect(runtimeClient.Create(ctx, mgh)).To(Succeed())
		Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)).To(Succeed())
	})

	It("should generate the transport connection in BYO case", func() {
		// transport
		err := CreateTestSecretTransport(runtimeClient, mgh.Namespace)
		Expect(err).To(Succeed())

		// update the transport protocol configuration
		Eventually(func() error {
			err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)
			if err != nil {
				return err
			}
			err = config.SetTransportConfig(ctx, runtimeClient, mgh)
			return err
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		// verify the type
		Expect(config.TransporterProtocol()).To(Equal(transport.SecretTransporter))
		Expect(config.GetSpecTopic()).To(Equal("gh-spec"))
		Expect(config.GetRawStatusTopic()).To(Equal("gh-status"))

		reconciler := operatortrans.NewTransportReconciler(runtimeManager, config.OLMVersionV0)

		Eventually(func() error {
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: mgh.Namespace,
					Name:      mgh.Name,
				},
			})
			if err != nil {
				return err
			}

			ready := config.IsTransportConfigReady(ctx, mgh.Namespace, runtimeClient)
			if !ready {
				return fmt.Errorf("the transport config should be ready")
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		// delete the transport secret
		err = runtimeClient.Delete(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.GHTransportSecretName,
				Namespace: mgh.Namespace,
			},
		})
		Expect(err).To(Succeed())
	})

	It("should generate the transport connection in strimzi transport", func() {
		config.SetTransporter(nil)
		// the crd resources is ready
		err := testutils.CreateTransportCSV(runtimeClient, ctx, "strimzi-kafka-operator", mgh.Namespace)
		Expect(err).To(Succeed())

		// Reset status topic for Strimzi mode (may have been changed by BYO test)
		Eventually(func() error {
			if err := runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh); err != nil {
				return err
			}
			mgh.Spec.DataLayerSpec.Kafka.KafkaTopics.StatusTopic = "gh-status.*"
			return runtimeClient.Update(ctx, mgh)
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		// update the transport protocol configuration, topic
		err = config.SetMulticlusterGlobalHubConfig(ctx, mgh, nil, nil)
		Expect(err).To(Succeed())
		err = config.SetTransportConfig(ctx, runtimeClient, mgh)
		Expect(err).To(Succeed())

		Expect(config.TransporterProtocol()).To(Equal(transport.StrimziTransporter))
		Expect(config.GetSpecTopic()).To(Equal("gh-spec"))
		Expect(config.GetRawStatusTopic()).To(Equal("gh-status.*"))

		reconciler := operatortrans.NewTransportReconciler(runtimeManager, config.OLMVersionV0)

		// blocking until get the connection
		go func() {
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: mgh.Namespace,
					Name:      mgh.Name,
				},
			})
			for err != nil {
				fmt.Println("reconciler error, retrying ...", err.Error())
				time.Sleep(1 * time.Second)

				_ = config.SetMulticlusterGlobalHubConfig(ctx, mgh, nil, nil)
				_, err = reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: mgh.Namespace,
						Name:      mgh.Name,
					},
				})
			}
		}()

		// the subscription
		Eventually(func() error {
			sub, err := operatorutils.GetSubscriptionByName(ctx, runtimeClient, namespace, protocol.DefaultKafkaSubName)
			if err != nil {
				return err
			}
			if sub == nil {
				return fmt.Errorf("should get the subscription %s", protocol.DefaultKafkaSubName)
			}

			return nil
		}, 20*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		// the kafka cluster
		Eventually(func() error {
			kafka := &kafkav1beta2.Kafka{}
			err := runtimeClient.Get(ctx, types.NamespacedName{
				Name:      protocol.KafkaClusterName,
				Namespace: mgh.Namespace,
			}, kafka)
			if err != nil {
				return err
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		// NetworkPolicies are rendered alongside the kafka manifests; verify both are created
		Eventually(func() error {
			np := &networkingv1.NetworkPolicy{}
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      protocol.KafkaClusterName,
				Namespace: mgh.Namespace,
			}, np)
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		Eventually(func() error {
			np := &networkingv1.NetworkPolicy{}
			return runtimeClient.Get(ctx, types.NamespacedName{
				Name:      "strimzi-cluster-operator",
				Namespace: mgh.Namespace,
			}, np)
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		// update the kafka resource to make it ready
		err = UpdateKafkaClusterReady(ctx, runtimeClient, mgh.Namespace)
		Expect(err).To(Succeed())

		// verify the metrics resources and pod monitor
		Eventually(func() error {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka-metrics",
					Namespace: mgh.Namespace,
				},
			}
			err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(cm), cm)
			if err != nil {
				return err
			}
			podMonitor := &promv1.PodMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka-resources-metrics",
					Namespace: mgh.Namespace,
				},
			}
			err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(podMonitor), podMonitor)
			if err != nil {
				return err
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())

		Eventually(func() error {
			// get the conn by transporter
			tran := config.GetTransporter()
			agentConn, err := tran.GetConnCredential("hub1")
			if err != nil {
				return err
			}
			if agentConn == nil {
				return fmt.Errorf("the strimzi connection for hub1 should not be nil")
			}
			return nil
		}, 20*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})

	It("should pass the strimzi transport configuration", func() {
		// Ensure transport config is set (may have been cleared by previous test)
		err := config.SetTransportConfig(ctx, runtimeClient, mgh)
		Expect(err).To(Succeed())

		trans := protocol.NewStrimziTransporter(
			runtimeManager,
			mgh,
			protocol.WithCommunity(false),
			protocol.WithOLMVersion(config.OLMVersionV0),
			protocol.WithNamespacedName(types.NamespacedName{
				Name:      protocol.KafkaClusterName,
				Namespace: mgh.Namespace,
			}),
		)

		customCPURequest := "1m"
		customMemoryRequest := "1Mi"
		mgh.Spec.AdvancedSpec = &v1alpha4.AdvancedSpec{
			Kafka: &v1alpha4.CommonSpec{
				Resources: &shared.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceName(corev1.ResourceMemory): resource.MustParse(customMemoryRequest),
						corev1.ResourceName(corev1.ResourceCPU):    resource.MustParse(customCPURequest),
					},
				},
			},
		}
		mgh.Spec.ImagePullSecret = "mgh-image-pull"

		err, updated := trans.CreateUpdateKafkaCluster(mgh)
		Expect(err).To(Succeed())
		Expect(updated).To(BeTrue())

		mgh.Spec.NodeSelector = map[string]string{
			"node-role.kubernetes.io/worker": "",
		}
		mgh.Spec.Tolerations = []corev1.Toleration{
			{
				Key:      "node-role.kubernetes.io/worker",
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			},
		}
		Eventually(func() error {
			err, _ = trans.CreateUpdateKafkaCluster(mgh)
			return err
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

		mgh.Spec.ImagePullSecret = "mgh-image-pull-update"
		Eventually(func() error {
			err, _ = trans.CreateUpdateKafkaCluster(mgh)
			if err != nil {
				return err
			}
			cluster := &kafkav1beta2.Kafka{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: mgh.Namespace,
					Name:      protocol.KafkaClusterName,
				},
			}
			err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)
			if err != nil {
				return err
			}
			pullSecrets := cluster.Spec.Kafka.Template.Pod.ImagePullSecrets
			if len(pullSecrets) == 0 {
				return fmt.Errorf("should update the image pull secret")
			}
			if *pullSecrets[0].Name != mgh.Spec.ImagePullSecret {
				return fmt.Errorf("should get the image pull secret %s, but got %s", mgh.Spec.ImagePullSecret,
					*pullSecrets[0].Name)
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

		kafka := &kafkav1beta2.Kafka{}
		err = runtimeClient.Get(ctx, types.NamespacedName{
			Namespace: mgh.Namespace,
			Name:      protocol.KafkaClusterName,
		}, kafka)
		Expect(err).To(Succeed())

		Expect(kafka.Spec.Kafka.Template.Pod.Affinity.NodeAffinity).NotTo(BeNil())
		Expect(kafka.Spec.Kafka.Template.Pod.Tolerations).NotTo(BeEmpty())
		Expect(kafka.Spec.Kafka.Template.Pod.ImagePullSecrets).NotTo(BeEmpty())

		Expect(string(kafka.Spec.Kafka.Resources.Requests.Raw)).To(Equal(`{"cpu":"1m","memory":"1Mi"}`))

		Expect(kafka.Spec.EntityOperator.Template.Pod.Affinity.NodeAffinity).NotTo(BeNil())
		Expect(kafka.Spec.EntityOperator.Template.Pod.Tolerations).NotTo(BeEmpty())
		Expect(kafka.Spec.EntityOperator.Template.Pod.ImagePullSecrets).NotTo(BeEmpty())

		mgh.Spec.NodeSelector = map[string]string{
			"node-role.kubernetes.io/worker": "",
			"topology.kubernetes.io/zone":    "east1",
		}
		mgh.Spec.Tolerations = []corev1.Toleration{
			{
				Key:      "node.kubernetes.io/not-ready",
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			},
			{
				Key:      "node-role.kubernetes.io/worker",
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			},
		}
		Eventually(func() error {
			err, updated = trans.CreateUpdateKafkaCluster(mgh)
			if err != nil {
				return err
			}
			if !updated {
				return fmt.Errorf("the kafka cluster should updated")
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

		kafka = &kafkav1beta2.Kafka{}
		err = runtimeClient.Get(ctx, types.NamespacedName{
			Namespace: mgh.Namespace,
			Name:      protocol.KafkaClusterName,
		}, kafka)
		Expect(err).To(Succeed())

		entityOperatorToleration, _ := json.Marshal(kafka.Spec.EntityOperator.Template.Pod.Tolerations)
		kafkaToleration, _ := json.Marshal(kafka.Spec.Kafka.Template.Pod.Tolerations)
		entityOperatorNodeAffinity, _ := json.Marshal(kafka.Spec.EntityOperator.Template.Pod.Affinity.NodeAffinity)
		kafkaNodeAffinity, _ := json.Marshal(kafka.Spec.Kafka.Template.Pod.Affinity.NodeAffinity)
		toleration := `[{"effect":"NoSchedule","key":"node.kubernetes.io/not-ready","operator":"Exists"},{"effect":"NoSchedule","key":"node-role.kubernetes.io/worker","operator":"Exists"}]`

		Expect(string(entityOperatorToleration)).To(Equal(toleration))
		Expect(string(kafkaToleration)).To(Equal(toleration))

		// cannot compare the string, because the order is random
		Expect(string(entityOperatorNodeAffinity)).To(ContainSubstring("node-role.kubernetes.io/worker"))
		Expect(string(entityOperatorNodeAffinity)).To(ContainSubstring("topology.kubernetes.io/zone"))
		Expect(string(kafkaNodeAffinity)).To(ContainSubstring("node-role.kubernetes.io/worker"))
		Expect(string(kafkaNodeAffinity)).To(ContainSubstring("topology.kubernetes.io/zone"))

		// simulate to create a cluster named: hub1
		clusterName := "hub1"

		// Initialize transporter state (topicPartitionReplicas, etc.) before calling EnsureTopic
		Eventually(func() error {
			needRequeue, err := trans.EnsureKafka()
			if err != nil {
				return err
			}
			if needRequeue {
				return fmt.Errorf("EnsureKafka requires requeue")
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(Succeed())

		// user - round 1
		userName, err := trans.EnsureUser(clusterName)
		Expect(err).To(Succeed())
		Expect(config.GetKafkaUserName(clusterName)).To(Equal(userName))

		kafkaUser := &kafkav1beta2.KafkaUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: mgh.Namespace,
			},
		}

		err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(kafkaUser), kafkaUser)
		Expect(err).To(Succeed())

		// topic: create
		clusterTopic, err := trans.EnsureTopic(clusterName)
		Expect(err).To(Succeed())
		Expect("gh-spec").To(Equal(clusterTopic.SpecTopic))
		Expect(config.GetStatusTopic(clusterName)).To(Equal(clusterTopic.StatusTopic))
		Expect(config.GetMigrationTopic()).To(Equal(clusterTopic.MigrationTopic))

		// topic: update
		_, err = trans.EnsureTopic(clusterName)
		if !errors.IsAlreadyExists(err) {
			Expect(err).To(Succeed())
		}

		err = trans.Prune(clusterName)
		Expect(err).To(Succeed())
	})

	It("should reconcile managed-hub KafkaUser ACLs", func() {
		const (
			clusterName         = "hub1"
			consumerGroupPrefix = "testprefix-"
		)

		Eventually(func() error {
			if err := runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh); err != nil {
				return err
			}
			mgh.Spec.DataLayerSpec.Kafka.ConsumerGroupPrefix = consumerGroupPrefix
			return runtimeClient.Update(ctx, mgh)
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed(),
			"update MulticlusterGlobalHub with the consumer-group prefix")

		err := config.SetMulticlusterGlobalHubConfig(ctx, mgh, nil, nil)
		Expect(err).To(Succeed(), "set managed-hub configuration with the consumer-group prefix")
		err = config.SetTransportConfig(ctx, runtimeClient, mgh)
		Expect(err).To(Succeed(), "set transporter configuration")

		trans := protocol.NewStrimziTransporter(
			runtimeManager,
			mgh,
			protocol.WithCommunity(false),
			protocol.WithOLMVersion(config.OLMVersionV0),
			protocol.WithNamespacedName(types.NamespacedName{
				Name:      protocol.KafkaClusterName,
				Namespace: mgh.Namespace,
			}),
		)

		Eventually(func() error {
			needRequeue, err := trans.EnsureKafka()
			if err != nil {
				return err
			}
			if needRequeue {
				return fmt.Errorf("EnsureKafka requires requeue")
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(Succeed(), "ensure Kafka cluster is ready for managed-hub ACL reconciliation")

		userName, err := trans.EnsureUser(clusterName)
		Expect(err).To(Succeed(), "ensure the managed-hub KafkaUser")
		Expect(config.GetKafkaUserName(clusterName)).To(Equal(userName),
			"EnsureUser should return the configured KafkaUser name")

		kafkaUser := &kafkav1beta2.KafkaUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: mgh.Namespace,
			},
		}
		err = runtimeClient.Get(ctx, client.ObjectKeyFromObject(kafkaUser), kafkaUser)
		Expect(err).To(Succeed(), "get the reconciled managed-hub KafkaUser")

		expectManagedHubKafkaUserACLs(kafkaUser, clusterName, consumerGroupPrefix)
	})

	AfterAll(func() {
		Eventually(func() error {
			if err := testutils.DeleteMgh(ctx, runtimeClient, mgh); err != nil {
				return err
			}
			return deleteNamespace(namespace)
		}, 30*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred())
	})
})

func expectManagedHubKafkaUserACLs(
	kafkaUser *kafkav1beta2.KafkaUser,
	clusterName string,
	consumerGroupPrefix string,
) {
	Expect(len(kafkaUser.Spec.Authorization.Acls)).To(Equal(4), "managed hub KafkaUser should have four ACL entries")

	aclByTopic := map[string][]kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{}
	consumerGroupACLs := []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{}
	for _, acl := range kafkaUser.Spec.Authorization.Acls {
		switch acl.Resource.Type {
		case kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic:
			Expect(acl.Resource.Name).NotTo(BeNil(), "topic ACL must name its resource")
			Expect(acl.Resource.PatternType).NotTo(BeNil(), "topic ACL must use a pattern type")
			Expect(*acl.Resource.PatternType).To(
				Equal(kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral),
				"topic ACL must use literal pattern matching",
			)
			aclByTopic[*acl.Resource.Name] = acl.Operations
		case kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeGroup:
			consumerGroupACLs = append(consumerGroupACLs, acl)
		}
	}

	specOps := aclByTopic["gh-spec"]
	Expect(specOps).To(ConsistOf(
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemDescribe,
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
	), "gh-spec ACL should grant Describe and Read only")
	Expect(specOps).NotTo(ContainElement(kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemWrite),
		"gh-spec ACL must not grant Write to managed hubs")

	migrationOps := aclByTopic[config.GetMigrationTopic()]
	Expect(migrationOps).To(ConsistOf(
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemDescribe,
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
	), "gh-migration ACL should grant Describe and Read for managed hub consumers")

	statusOps := aclByTopic[config.GetStatusTopic(clusterName)]
	Expect(statusOps).To(Equal([]kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemWrite,
	}), "status topic ACL should grant Write only to the hub status topic")

	Expect(consumerGroupACLs).To(HaveLen(1), "managed hub should have one consumer-group ACL")
	Expect(consumerGroupACLs[0].Resource.Type).To(
		Equal(kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeGroup),
		"consumer-group ACL must target a group resource",
	)
	Expect(consumerGroupACLs[0].Resource.Name).NotTo(BeNil(), "consumer-group ACL must name its group")
	Expect(consumerGroupACLs[0].Resource.PatternType).NotTo(BeNil(), "consumer-group ACL must use a pattern type")
	Expect(*consumerGroupACLs[0].Resource.PatternType).To(
		Equal(kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral),
		"consumer-group ACL must use literal pattern matching",
	)
	expectedGroupID := config.GetConsumerGroupID(consumerGroupPrefix, clusterName)
	Expect(*consumerGroupACLs[0].Resource.Name).To(Equal(expectedGroupID),
		"consumer-group ACL must include the configured consumer group prefix")
	Expect(*consumerGroupACLs[0].Resource.Name).NotTo(Equal("*"),
		"consumer-group ACL must not use wildcard group authorization")
	Expect(consumerGroupACLs[0].Operations).To(ConsistOf(
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
	), "consumer-group ACL should grant Read only")
}

func UpdateKafkaClusterReady(ctx context.Context, c client.Client, ns string) error {
	kafkaVersion := "4.1.0"
	kafkaClusterName := "kafka"
	globalHubKafkaUser := "global-hub-kafka-user"
	clientCAKeySecret := "kafka-clients-ca"
	clientCACertSecret := "kafka-clients-ca-cert"

	readyCondition := "Ready"
	trueCondition := "True"
	bootServer := "kafka-kafka-bootstrap.multicluster-global-hub.svc:9092"
	statusClusterId := "MXpoZsJTRD2DDiVUh3Rsqg"

	statusKafkaCluster := &kafkav1beta2.Kafka{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      kafkaClusterName,
		},
		Spec: &kafkav1beta2.KafkaSpec{
			Kafka: kafkav1beta2.KafkaSpecKafka{
				Listeners: []kafkav1beta2.KafkaSpecKafkaListenersElem{
					{
						Name: "tls",
						Port: 9093,
						Type: "nodeport",
					},
				},
				Config: &apiextensions.JSON{Raw: []byte(`{
"default.replication.factor": 3
}`)},
				Version: &kafkaVersion,
			},
		},
		Status: &kafkav1beta2.KafkaStatus{
			ClusterId: &statusClusterId,
			Listeners: []kafkav1beta2.KafkaStatusListenersElem{
				{
					BootstrapServers: &bootServer,
					Certificates: []string{
						"cert",
					},
				},
			},
			Conditions: []kafkav1beta2.KafkaStatusConditionsElem{
				{
					Type:   &readyCondition,
					Status: &trueCondition,
				},
			},
		},
	}

	if err := wait.PollUntilContextTimeout(ctx, 1*time.Second, 1*time.Minute, true, func(pollCtx context.Context) (bool, error) {
		existkafkaCluster := &kafkav1beta2.Kafka{}
		err := c.Get(pollCtx, types.NamespacedName{
			Name:      kafkaClusterName,
			Namespace: ns,
		}, existkafkaCluster)
		if err != nil {
			if errors.IsNotFound(err) {
				if e := c.Create(pollCtx, statusKafkaCluster); e != nil {
					klog.Errorf("Failed to create kafka cluster, error: %v", e)
					return false, nil
				}
			} else {
				klog.Errorf("Failed to get Kafka cluster, error:%v", err)
			}
			return false, nil
		}
		existkafkaCluster.Status = &kafkav1beta2.KafkaStatus{
			Listeners: []kafkav1beta2.KafkaStatusListenersElem{
				{
					BootstrapServers: &bootServer,
					Certificates: []string{
						"cert",
					},
				},
			},
			Conditions: []kafkav1beta2.KafkaStatusConditionsElem{
				{
					Type:   &readyCondition,
					Status: &trueCondition,
				},
			},
		}
		err = c.Status().Update(pollCtx, existkafkaCluster)
		if err != nil {
			klog.Errorf("Failed to update Kafka cluster, error:%v", err)
			return false, nil
		}
		return true, nil
	}); err != nil {
		return err
	}

	err := createSecret(ctx, c, ns, globalHubKafkaUser, map[string][]byte{
		"user.crt": []byte("usercrt"),
		"user.key": []byte("userkey"),
	})
	if err != nil {
		return err
	}

	err = createSecret(ctx, c, ns, clientCAKeySecret, map[string][]byte{
		"ca.key": []byte("cakey"),
	})
	if err != nil {
		return err
	}

	err = createSecret(ctx, c, ns, clientCACertSecret, map[string][]byte{
		"ca.crt": []byte("cacert"),
	})
	if err != nil {
		return err
	}
	return nil
}

func createSecret(ctx context.Context, c client.Client, ns, name string, data map[string][]byte) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
	}
	err := c.Get(ctx, client.ObjectKeyFromObject(secret), secret)
	if errors.IsNotFound(err) {
		secret.Data = data
		if err := c.Create(ctx, secret); err != nil {
			return fmt.Errorf("create secret %s/%s: %w", ns, name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get secret %s/%s: %w", ns, name, err)
	}
	secret.Data = data
	if err := c.Update(ctx, secret); err != nil {
		return fmt.Errorf("update secret %s/%s: %w", ns, name, err)
	}
	return nil
}
