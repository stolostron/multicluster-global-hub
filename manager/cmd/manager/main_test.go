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
		"--leader-elect",
		"true",
		"--lease-duration",
		"123",
		"--renew-deadline",
		"456",
		"--retry-period",
		"789",
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
		if managerConfig.electionConfig.LeaderElection != true {
			t.Fatalf("unexpected electionConfig LeaderElection] - got '%t', want 'true'",
				managerConfig.electionConfig.LeaderElection)
		}

		if managerConfig.electionConfig.LeaseDuration != 123 {
			t.Fatalf("unexpected electionConfig LeaseDuration] - got '%d', want '123'",
				managerConfig.electionConfig.LeaseDuration)
		}
	})
}
