// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/test/pkg/testpostgres"
)

var (
	cfg              *rest.Config
	kubeClient       kubernetes.Interface
	mockKafkaCluster *kafka.MockCluster
	testPostgres     *testpostgres.TestPostgres
)

func TestMain(m *testing.M) {
	var err error
	err = os.Setenv("POD_NAMESPACE", "default")
	if err != nil {
		panic(err)
	}
	err = os.Setenv("MANAGER_TESTING", "true")
	if err != nil {
		panic(err)
	}

	// start testenv
	testenv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "pkg", "testdata", "crds"),
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

	// init mock kafka cluster
	mockKafkaCluster, err = kafka.NewMockCluster(1)
	if err != nil {
		panic(err)
	}

	if mockKafkaCluster == nil {
		panic(fmt.Errorf("empty mock kafka cluster!"))
	}

	// init test postgres
	testPostgres, err = testpostgres.NewTestPostgres()
	if err != nil {
		panic(err)
	}

	// run testings
	code := m.Run()

	// stop mock kafka cluster
	mockKafkaCluster.Close()

	// stop testenv
	err = testenv.Stop()
	if err != nil {
		panic(err)
	}
	if err := testPostgres.Stop(); err != nil {
		panic(err)
	}
	os.Exit(code)
}

func TestManager(t *testing.T) {
	// the testing manipuates the os.Args to set them up for the testcases
	// after this testing the initial args will be restored
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	cases := []struct {
		name         string
		args         []string
		expectedExit int
	}{
		{"manager", []string{
			"--manager-namespace",
			"default",
			"--cluster-api-url",
			"",
			"--process-database-url",
			testPostgres.URI,
			"--transport-bridge-database-url",
			testPostgres.URI,
			"--lease-duration",
			"15",
			"--renew-deadline",
			"10",
			"--retry-period",
			"2",
			"--kafka-bootstrap-server",
			mockKafkaCluster.BootstrapServers(),
			"--transport-type",
			string(transport.Chan),
		}, 1},
	}
	for _, tc := range cases {
		// this call is required because otherwise flags panics, if args are set between flag.Parse call
		pflag.CommandLine = pflag.NewFlagSet(tc.name, pflag.ExitOnError)
		// we need a value to set Args[0] to cause flag begins parsing at Args[1]
		os.Args = append([]string{tc.name}, tc.args...)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := kubeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: constants.GHSystemNamespace,
			},
		}, metav1.CreateOptions{}); err != nil {
			t.Errorf("failed to create global hub system namespace: %v", err)
		}
		if _, err := kubeClient.CoreV1().ConfigMaps(constants.GHSystemNamespace).Create(ctx,
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.GHConfigCMName,
					Namespace: constants.GHSystemNamespace,
					Annotations: map[string]string{
						constants.OriginOwnerReferenceAnnotation: "testing",
					},
				},
				Data: map[string]string{"aggregationLevel": "full", "enableLocalPolicies": "true"},
			}, metav1.CreateOptions{}); err != nil {
			t.Errorf("failed to create global hub configuration configmap: %v", err)
		}
		actualExit := doMain(ctx, cfg)
		if tc.expectedExit != actualExit {
			t.Errorf("unexpected exit code for args: %v, expected: %v, got: %v",
				tc.args, tc.expectedExit, actualExit)
		}
	}
}
