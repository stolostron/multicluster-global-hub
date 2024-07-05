// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var (
	cfg        *rest.Config
	kubeClient kubernetes.Interface
	// mockKafkaCluster *kafka.MockCluster
)

func TestMain(m *testing.M) {
	var err error
	err = os.Setenv("POD_NAMESPACE", "default")
	if err != nil {
		panic(err)
	}
	err = os.Setenv("AGENT_TESTING", "true")
	if err != nil {
		panic(err)
	}

	// start testenv
	testenv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "test", "manifest", "crd"),
		},
		ErrorIfCRDPathMissing: true,
	}

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

	if _, err := kubeClient.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.GHAgentNamespace,
		},
	}, metav1.CreateOptions{}); err != nil {
		panic(err)
	}

	// run testings
	code := m.Run()

	// // stop mock kafka cluster
	// mockKafkaCluster.Close()

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

func TestAgentFlag(t *testing.T) {
	// the testing manipuates the os.Args to set them up for the testcases
	// after this testing the initial args will be restored
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	cases := []struct {
		name         string
		args         []string
		expectedExit int
	}{
		{"agent-cleanup", []string{
			"--leaf-hub-name",
			"hub1",
			"--terminating",
			"true",
		}, 0},
		{"agent", []string{
			"--pod-namespace",
			"default",
			"--leaf-hub-name",
			"hub1",
			"--kafka-consumer-id",
			"hub1",
			"--lease-duration",
			"15",
			"--renew-deadline",
			"10",
			"--retry-period",
			"2",
			"--transport-type",
			string(transport.Chan),
			// "--kafka-bootstrap-server",
			// mockKafkaCluster.BootstrapServers(),
		}, 0},
	}
	for _, tc := range cases {
		// this call is required because otherwise flags panics, if args are set between flag.Parse call
		pflag.CommandLine = pflag.NewFlagSet(tc.name, pflag.ExitOnError)
		flag.CommandLine = flag.NewFlagSet(tc.name, flag.ExitOnError)
		// we need a value to set Args[0] to cause flag begins parsing at Args[1]
		os.Args = append([]string{tc.name}, tc.args...)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		agentConfig := parseFlags() // add and parse agent flags
		if !pflag.Parsed() {
			t.Error("agent flags should be parsed, but actually not")
		}

		agentConfig.TransportConfig.KafkaConfig.EnableTLS = false
		if agentConfig.Terminating {
			actualExit := doTermination(ctx, cfg)
			if tc.expectedExit != actualExit {
				t.Errorf("unexpected exit code for args: %v, expected: %v, got: %v",
					tc.args, tc.expectedExit, actualExit)
			}
		} else {
			actualExit := doMain(ctx, cfg, agentConfig)
			if tc.expectedExit != actualExit {
				t.Errorf("unexpected exit code for args: %v, expected: %v, got: %v",
					tc.args, tc.expectedExit, actualExit)
			}
		}
	}
}

func initMockAgentConfig() *config.AgentConfig {
	return &config.AgentConfig{
		PodNameSpace:   "default",
		ElectionConfig: &commonobjects.LeaderElectionConfig{},
		MetricsAddress: "0",
		TransportConfig: &transport.TransportConfig{
			TransportType: string(transport.Chan),
		},
	}
}

func TestHasMCHCRDWithoutCR(t *testing.T) {
	testenv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "test", "manifest", "crd"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testenv.Start()
	assert.Nil(t, err)
	// stop testenv
	defer func() {
		err := testenv.Stop()
		assert.Nil(t, err)
	}()
	_, err = createManager(context.Background(), cfg, initMockAgentConfig())
	assert.Nil(t, err)
}

func TestHasMCHCRDCR(t *testing.T) {
	testenv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "test", "manifest", "crd"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testenv.Start()
	assert.Nil(t, err)

	defer func() {
		err := testenv.Stop()
		assert.Nil(t, err)
	}()

	// generate the client based off of the config
	dynamicClient, err := dynamic.NewForConfig(cfg)
	assert.Nil(t, err)

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "operator.open-cluster-management.io/v1",
			"kind":       "MultiClusterHub",
			"metadata": map[string]interface{}{
				"name":      "test",
				"namespace": "default",
			},
		},
	}

	resource := schema.GroupVersionResource{
		Group:   "operator.open-cluster-management.io",
		Version: "v1", Resource: "multiclusterhubs",
	}
	_, err = dynamicClient.Resource(resource).Namespace("default").
		Create(context.TODO(), obj, metav1.CreateOptions{})
	assert.Nil(t, err)

	_, err = createManager(context.Background(), cfg, initMockAgentConfig())
	assert.Nil(t, err)
}

func TestAgentConfig(t *testing.T) {
	agentConfig := &config.AgentConfig{
		LeafHubName:      "hub1",
		SpecWorkPoolSize: 4,
		TransportConfig: &transport.TransportConfig{
			KafkaConfig: &transport.KafkaConfig{
				ProducerConfig: &transport.KafkaProducerConfig{
					MessageSizeLimitKB: 561,
				},
				EnableTLS: true,
				Topics:    &transport.ClusterTopic{},
			},
		},
		Standalone: true,
	}
	err := os.Setenv(config.KAFKA_BOOTSTRAP_SERVERS, "http:test.com")
	assert.Nil(t, err)
	err = os.Setenv(config.STATUS_TOPIC, "status")
	assert.Nil(t, err)
	err = os.Setenv(config.EVENT_TOPIC, "event")
	assert.Nil(t, err)

	err = completeConfig(agentConfig)
	assert.Nil(t, err)

	assert.Equal(t, "http:test.com", agentConfig.TransportConfig.KafkaConfig.BootstrapServer)
	assert.Equal(t, "status", agentConfig.TransportConfig.KafkaConfig.Topics.StatusTopic)
	assert.Equal(t, "event", agentConfig.TransportConfig.KafkaConfig.Topics.EventTopic)
}
