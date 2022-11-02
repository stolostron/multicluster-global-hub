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

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/spf13/pflag"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	cfg              *rest.Config
	mockKafkaCluster *kafka.MockCluster
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

	// init mock kafka cluster
	mockKafkaCluster, err = kafka.NewMockCluster(1)
	if err != nil {
		panic(err)
	}

	if mockKafkaCluster == nil {
		panic(fmt.Errorf("empty mock kafka cluster!"))
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

	os.Exit(code)
}

func TestAgent(t *testing.T) {
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
			"--lease-duration",
			"15",
			"--renew-deadline",
			"10",
			"--retry-period",
			"2",
			"--leaf-hub-name",
			"hub1",
			"--transport-type",
			"kafka",
			"--kafka-bootstrap-server",
			mockKafkaCluster.BootstrapServers(),
		}, 0},
	}
	for _, tc := range cases {
		// this call is required because otherwise flags panics, if args are set between flag.Parse call
		pflag.CommandLine = pflag.NewFlagSet(tc.name, pflag.ExitOnError)
		// we need a value to set Args[0] to cause flag begins parsing at Args[1]
		os.Args = append([]string{tc.name}, tc.args...)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		actualExit := doMain(ctx, cfg)
		if tc.expectedExit != actualExit {
			t.Errorf("unexpected exit code for args: %v, expected: %v, got: %v",
				tc.args, tc.expectedExit, actualExit)
		}
	}
}
