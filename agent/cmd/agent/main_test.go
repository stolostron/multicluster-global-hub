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
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
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
