// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
		"hub2",
		"--terminating",
		"true",
	}

	code := doMain(context.Background(), cfg)
	if code != 0 {
		t.Fatalf("start doMain with error code %d", code)
	}
}
