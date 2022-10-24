// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/helper"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var cfg *rest.Config

func TestMain(m *testing.M) {
	// start testEnv
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "pkg", "testdata", "crds"),
		},
	}
	var err error
	err = os.Setenv("POD_NAMESPACE", "default")
	if err != nil {
		panic(err)
	}

	cfg, err = testEnv.Start()
	if err != nil {
		panic(err)
	}

	fmt.Println("Hello World")

	// run testings
	code := m.Run()

	// stop testEnv
	err = testEnv.Stop()
	if err != nil {
		panic(err)
	}

	os.Exit(code)
}

func TestGetControllerManager(t *testing.T) {
	os.Args = []string{
		"agent",
		"--lease-duration",
		"137",
		"--renew-deadline",
		"107",
		"--leaf-hub-name",
		"hub1",
		"--terminating",
		"true",
	}
	agentConfig, err := helper.NewConfigManager()
	if err != nil {
		t.Fatalf("failed to parseFlags %v", err)
	}
	if agentConfig.ElectionConfig.LeaseDuration != 137 {
		t.Fatalf("expect --lease-duration(%d) == %d", agentConfig.ElectionConfig.LeaseDuration, 137)
	}
	if agentConfig.ElectionConfig.RenewDeadline != 107 {
		t.Fatalf("expect --renew-deadline(%d) == %d", agentConfig.ElectionConfig.RenewDeadline, 107)
	}
	if !agentConfig.Terminating {
		t.Fatalf("expect--terminating(%t) == %t", agentConfig.Terminating, true)
	}
	if err != nil {
		t.Fatalf("failed to parseFlags %v", err)
	}
	_, err = getControllerManager(cfg, agentConfig)
	if err != nil {
		t.Fatalf("failed to getControllerManager %v", err)
	}
}

// func TestAgent(t *testing.T) {
// 	// the testing manipuates the os.Args to set them up for the testcases
// 	// after this testing the initial args will be restored
// 	oldArgs := os.Args
// 	defer func() { os.Args = oldArgs }()

// 	cases := []struct {
// 		name                 string
// 		args                 []string
// 		leaderElectionConfig *corev1.ConfigMap
// 		expectedExit         int
// 	}{
// 		{"flag set with leader-election disabled", []string{
// 			"-leader-election",
// 			"false",
// 			"-metrics-bind-address",
// 			":18080",
// 			"-health-probe-bind-address",
// 			":18081",
// 		}, nil, 0},
// 		{"flag set with leader-election enabled", []string{
// 			"-leader-election",
// 			"-metrics-bind-address",
// 			":18080",
// 			"-health-probe-bind-address",
// 			":18081",
// 		}, nil, 0},
// 		{"flag set with customized leader-election configuration", []string{
// 			"-leader-election",
// 			"-metrics-bind-address",
// 			":18080",
// 			"-health-probe-bind-address",
// 			":18081",
// 		}, &corev1.ConfigMap{
// 			ObjectMeta: metav1.ObjectMeta{
// 				Name:      commonconstants.ControllerLeaderElectionConfig,
// 				Namespace: config.GetDefaultNamespace(),
// 			},
// 			Data: map[string]string{"leaseDuration": "138", "renewDeadline": "107", "retryPeriod": "26"},
// 		}, 0},
// 	}
// 	for _, tc := range cases {
// 		// this call is required because otherwise flags panics, if args are set between flag.Parse call
// 		flag.CommandLine = flag.NewFlagSet(tc.name, flag.ExitOnError)
// 		// we need a value to set Args[0] to cause flag begins parsing at Args[1]
// 		os.Args = append([]string{tc.name}, tc.args...)
// 		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
// 		defer cancel()
// 		if tc.leaderElectionConfig != nil {
// 			if _, err := kubeClient.CoreV1().ConfigMaps(
// 				tc.leaderElectionConfig.GetNamespace()).Create(ctx,
// 				tc.leaderElectionConfig, metav1.CreateOptions{}); err != nil {
// 				t.Errorf("failed to create leader election configmap: %v", err)
// 			}
// 		}
// 		actualExit := doMain(ctx, cfg)
// 		if tc.expectedExit != actualExit {
// 			t.Errorf("unexpected exit code for args: %v, expected: %v, got: %v",
// 				tc.args, tc.expectedExit, actualExit)
// 		}
// 	}
// }
