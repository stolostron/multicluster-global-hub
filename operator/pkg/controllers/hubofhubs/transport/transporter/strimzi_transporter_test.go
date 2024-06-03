// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package transporter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	chnv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/test/pkg/kafka"
)

var (
	cfg           *rest.Config
	kubeClient    kubernetes.Interface
	runtimeClient client.Client
	bootServer    = "kafka-kafka-bootstrap.multicluster-global-hub.svc:9092"
)

func TestMain(m *testing.M) {
	testenv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "..", "..", "pkg", "testdata", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}
	config.SetKafkaResourceReady(true)

	var err error
	cfg, err = testenv.Start()
	if err != nil {
		panic(err)
	}

	if cfg == nil {
		panic(fmt.Errorf("empty kubeconfig!"))
	}

	kubeClient, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}

	subv1alpha1.AddToScheme(scheme.Scheme)
	kafkav1beta2.AddToScheme(scheme.Scheme)
	chnv1.AddToScheme(scheme.Scheme)

	runtimeClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		panic(err)
	}

	code := m.Run()

	// stop testenv
	err = testenv.Stop()
	if err != nil {
		time.Sleep(4 * time.Second)
	}
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	if err = testenv.Stop(); err != nil {
		panic(err)
	}

	os.Exit(code)
}

func TestStrimziTransporter(t *testing.T) {
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mgh",
			Namespace: "default",
		},
		Spec: v1alpha4.MulticlusterGlobalHubSpec{
			DataLayer: v1alpha4.DataLayerConfig{},
		},
	}
	trans, err := NewStrimziTransporter(
		runtimeClient,
		mgh,
		WithCommunity(false),
		WithNamespacedName(types.NamespacedName{
			Name:      KafkaClusterName,
			Namespace: "default",
		}),
		WithSubName("test-sub"),
		WithWaitReady(false),
	)
	assert.Nil(t, err)
	assert.NotNil(t, trans)

	err = runtimeClient.Get(context.Background(), types.NamespacedName{
		Name:      "test-sub",
		Namespace: "default",
	}, &subv1alpha1.Subscription{})
	assert.Nil(t, err)

	err = kafka.UpdateKafkaClusterReady(runtimeClient, "default")
	assert.Nil(t, err)

	err = trans.kafkaClusterReady()
	assert.Nil(t, err)

	kafka := &kafkav1beta2.Kafka{}
	err = runtimeClient.Get(context.TODO(), types.NamespacedName{
		Namespace: "default",
		Name:      KafkaClusterName,
	}, kafka)
	assert.Nil(t, err)
	assert.Equal(t, bootServer, *kafka.Status.Listeners[0].BootstrapServers)

	customCPURequest := "1m"
	customCPULimit := "2m"
	customMemoryRequest := "1Mi"
	customMemoryLimit := "2Mi"

	mgh.Spec.AdvancedConfig = &v1alpha4.AdvancedConfig{
		Kafka: &v1alpha4.CommonSpec{
			Resources: &v1alpha4.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceName(corev1.ResourceCPU):    resource.MustParse(customCPULimit),
					corev1.ResourceName(corev1.ResourceMemory): resource.MustParse(customMemoryLimit),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceName(corev1.ResourceMemory): resource.MustParse(customMemoryRequest),
					corev1.ResourceName(corev1.ResourceCPU):    resource.MustParse(customCPURequest),
				},
			},
		},
		Zookeeper: &v1alpha4.CommonSpec{
			Resources: &v1alpha4.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceName(corev1.ResourceCPU):    resource.MustParse(customCPULimit),
					corev1.ResourceName(corev1.ResourceMemory): resource.MustParse(customMemoryLimit),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceName(corev1.ResourceMemory): resource.MustParse(customMemoryRequest),
					corev1.ResourceName(corev1.ResourceCPU):    resource.MustParse(customCPURequest),
				},
			},
		},
	}
	mgh.Spec.ImagePullSecret = "mgh-image-pull"

	err, updated := trans.createUpdateKafkaCluster(mgh)
	assert.Nil(t, err)
	assert.True(t, updated)

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
	err, updated = trans.createUpdateKafkaCluster(mgh)
	assert.Nil(t, err)
	assert.True(t, updated)

	err, updated = trans.createUpdateKafkaCluster(mgh)
	assert.Nil(t, err)
	assert.False(t, updated)

	mgh.Spec.ImagePullSecret = "mgh-image-pull-update"
	err, updated = trans.createUpdateKafkaCluster(mgh)
	assert.Nil(t, err)
	assert.True(t, updated)

	kafka = &kafkav1beta2.Kafka{}
	err = runtimeClient.Get(context.TODO(), types.NamespacedName{
		Namespace: "default",
		Name:      KafkaClusterName,
	}, kafka)
	assert.Nil(t, err)
	assert.NotEmpty(t, kafka.Spec.Kafka.Template.Pod.Affinity.NodeAffinity)
	assert.NotEmpty(t, kafka.Spec.Kafka.Template.Pod.Tolerations)
	assert.NotEmpty(t, kafka.Spec.Kafka.Template.Pod.ImagePullSecrets)

	assert.Equal(t, string(kafka.Spec.Kafka.Resources.Requests.Raw), `{"cpu":"1m","memory":"1Mi"}`)
	assert.Equal(t, string(kafka.Spec.Kafka.Resources.Limits.Raw), `{"cpu":"2m","memory":"2Mi"}`)
	assert.NotEmpty(t, kafka.Spec.Zookeeper.Template.Pod.Affinity.NodeAffinity)
	assert.NotEmpty(t, kafka.Spec.Zookeeper.Template.Pod.Tolerations)
	assert.NotEmpty(t, kafka.Spec.Zookeeper.Template.Pod.ImagePullSecrets)

	assert.Equal(t, string(kafka.Spec.Zookeeper.Resources.Requests.Raw), `{"cpu":"1m","memory":"1Mi"}`)
	assert.Equal(t, string(kafka.Spec.Zookeeper.Resources.Limits.Raw), `{"cpu":"2m","memory":"2Mi"}`)
	assert.NotEmpty(t, kafka.Spec.EntityOperator.Template.Pod.Affinity.NodeAffinity)
	assert.NotEmpty(t, kafka.Spec.EntityOperator.Template.Pod.Tolerations)
	assert.NotEmpty(t, kafka.Spec.EntityOperator.Template.Pod.ImagePullSecrets)

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

	err, updated = trans.createUpdateKafkaCluster(mgh)
	assert.Nil(t, err)
	assert.True(t, updated)

	kafka = &kafkav1beta2.Kafka{}
	err = runtimeClient.Get(context.TODO(), types.NamespacedName{
		Namespace: "default",
		Name:      KafkaClusterName,
	}, kafka)

	entityOperatorToleration, _ := json.Marshal(kafka.Spec.EntityOperator.Template.Pod.Tolerations)
	kafkaToleration, _ := json.Marshal(kafka.Spec.Kafka.Template.Pod.Tolerations)
	zookeeperToleration, _ := json.Marshal(kafka.Spec.Zookeeper.Template.Pod.Tolerations)
	entityOperatorNodeAffinity, _ := json.Marshal(kafka.Spec.EntityOperator.Template.Pod.Affinity.NodeAffinity)
	kafkaNodeAffinity, _ := json.Marshal(kafka.Spec.Kafka.Template.Pod.Affinity.NodeAffinity)
	zookeeperNodeAffinity, _ := json.Marshal(kafka.Spec.Zookeeper.Template.Pod.Affinity.NodeAffinity)

	toleration := `[{"effect":"NoSchedule","key":"node.kubernetes.io/not-ready","operator":"Exists"},{"effect":"NoSchedule","key":"node-role.kubernetes.io/worker","operator":"Exists"}]`
	assert.Nil(t, err)
	assert.Equal(t, string(entityOperatorToleration), toleration)
	assert.Equal(t, string(kafkaToleration), toleration)
	assert.Equal(t, string(zookeeperToleration), toleration)
	// cannot compare the string, because the order is random
	assert.Contains(t, string(entityOperatorNodeAffinity), `node-role.kubernetes.io/worker`)
	assert.Contains(t, string(entityOperatorNodeAffinity), `topology.kubernetes.io/zone`)
	assert.Contains(t, string(kafkaNodeAffinity), `node-role.kubernetes.io/worker`)
	assert.Contains(t, string(kafkaNodeAffinity), `topology.kubernetes.io/zone`)
	assert.Contains(t, string(zookeeperNodeAffinity), `node-role.kubernetes.io/worker`)
	assert.Contains(t, string(zookeeperNodeAffinity), `topology.kubernetes.io/zone`)

	// simulate to create a cluster named: hub1
	clusterName := "hub1"

	// user
	userName := trans.GenerateUserName(clusterName)
	assert.Equal(t, fmt.Sprintf("%s-kafka-user", clusterName), userName)
	err = trans.CreateAndUpdateUser(userName)
	assert.Nil(t, err)

	// topic
	clusterTopic := trans.GenerateClusterTopic(clusterName)
	assert.Equal(t, "spec", clusterTopic.SpecTopic)
	assert.Equal(t, "event", clusterTopic.EventTopic)
	assert.Equal(t, fmt.Sprintf(StatusTopicTemplate, clusterName), clusterTopic.StatusTopic)

	err = trans.CreateAndUpdateTopic(clusterTopic)
	assert.Nil(t, err)

	// grant readable permission
	err = trans.GrantRead(userName, "spec")
	assert.Nil(t, err)

	err = trans.GrantRead(userName, "spec")
	assert.Nil(t, err)

	kafkaUser := &kafkav1beta2.KafkaUser{}
	err = runtimeClient.Get(context.TODO(), types.NamespacedName{
		Name:      userName,
		Namespace: "default",
	}, kafkaUser)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(kafkaUser.Spec.Authorization.Acls))
	// p, _ := json.MarshalIndent(kafkaUser, "", " ")
	// fmt.Println(string(p))

	// grant writable permission
	err = trans.GrantWrite(userName, "event")
	assert.Nil(t, err)

	err = trans.GrantWrite(userName, "event")
	assert.Nil(t, err)

	err = trans.GrantRead(userName, StatusTopicRegex)
	assert.Nil(t, err)

	kafkaUser = &kafkav1beta2.KafkaUser{}
	err = runtimeClient.Get(context.TODO(), types.NamespacedName{
		Name:      userName,
		Namespace: "default",
	}, kafkaUser)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(kafkaUser.Spec.Authorization.Acls))
	// p, _ := json.MarshalIndent(kafkaUser, "", " ")
	// fmt.Println(string(p))

	// delete user and topic
	err = trans.DeleteUser(userName)
	assert.Nil(t, err)

	err = trans.DeleteTopic(clusterTopic)
	assert.Nil(t, err)

	// test block
	_, err = NewStrimziTransporter(runtimeClient, mgh, WithWaitReady(true))
	assert.Nil(t, err)
}
