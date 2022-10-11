// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"os"
	"testing"
)

func TestParseFlags(t *testing.T) {
	os.Args = []string{
		"manager",
		"--process-database-url",
		"test.example.com:5432",
		"--transport-bridge-database-url",
		"test.example.com:5432",
	}
	t.Run("manager flags parse testing", func(t *testing.T) {
		managerConfig, err := parseFlags()
		if err != nil {
			t.Fatalf("flags parse error - %v", err)
		}
		if managerConfig.nonK8sAPIServerConfig.ServerBasePath != "/global-hub-api/v1" {
			t.Fatalf("unexpected non-k8s-api server base path] - got '%s', want '/global-hub-api/v1'",
				managerConfig.nonK8sAPIServerConfig.ServerBasePath)
		}
	})
}
