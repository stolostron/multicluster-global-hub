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

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/spf13/pflag"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/test/integration/utils/testpostgres"
)

var (
	cfg              *rest.Config
	kubeClient       kubernetes.Interface
	mockKafkaCluster *kafka.MockCluster
	testPostgres     *testpostgres.TestPostgres
)

func TestMain(m *testing.M) {
	err := os.Setenv("POD_NAMESPACE", "default")
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

	err = testpostgres.InitDatabase(testPostgres.URI)
	if err != nil {
		panic(err)
	}

	// run testings
	code := m.Run()

	// stop mock kafka cluster
	mockKafkaCluster.Close()
	// stop testenv
	if err := testenv.Stop(); err != nil {
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
			"--kafka-ca-cert-path",
			"/path/to/ca.crt",
			"--kafka-client-cert-path",
			"/path/to/tls.crt",
			"--kafka-client-key-path",
			"/path/to/tls.key",
			"--transport-type",
			string(transport.Chan),
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
		actualExit := doMain(ctx, cfg)
		if tc.expectedExit != actualExit {
			t.Errorf("Unexpected exit code for args: %v, expected: %v, got: %v",
				tc.args, tc.expectedExit, actualExit)
		}
	}
}
