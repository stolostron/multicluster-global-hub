// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package transporter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/test/pkg/kafka"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	chnv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	cfg            *rest.Config
	kubeClient     kubernetes.Interface
	runtimeClient  client.Client
	readyCondition = "Ready"
	trueCondition  = "True"
	bootServer     = "kafka-kafka-bootstrap.multicluster-global-hub.svc:9092"
)

func TestMain(m *testing.M) {
	testenv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "pkg", "testdata", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}

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

	// simulate to create a cluster named: hub1
	clusterName := "hub1"

	// user
	userName := trans.GenerateUserName(clusterName)
	assert.Equal(t, fmt.Sprintf("%s-kafka-user", clusterName), userName)
	err = trans.CreateUser(userName)
	assert.Nil(t, err)
	err = trans.DeleteUser(userName)
	assert.Nil(t, err)

	// topic
	clusterTopic := trans.GenerateClusterTopic(clusterName)
	assert.Equal(t, "globalhub.spec", clusterTopic.SpecTopic)
	assert.Equal(t, "event", clusterTopic.EventTopic)
	assert.Equal(t, fmt.Sprintf("globalhub.status.%s", clusterName), clusterTopic.StatusTopic)

	err = trans.CreateTopic(clusterTopic)
	assert.Nil(t, err)
	err = trans.DeleteTopic(clusterTopic)
	assert.Nil(t, err)

	// test block
	_, err = NewStrimziTransporter(runtimeClient, mgh, WithWaitReady(true))
	assert.Nil(t, err)
}
