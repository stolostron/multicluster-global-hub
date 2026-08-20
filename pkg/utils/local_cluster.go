// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"context"
	"fmt"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// IsLocalManagedCluster reports whether obj is the hub's local ManagedCluster.
func IsLocalManagedCluster(obj client.Object) bool {
	if obj == nil {
		return false
	}
	labels := obj.GetLabels()
	if labels != nil && labels[constants.LocalClusterName] == "true" {
		return true
	}
	return obj.GetName() == constants.LocalClusterName
}

// ResolveLocalClusterManagedClusterName returns the ManagedCluster name for the
// cluster labeled local-cluster=true. When no such cluster exists, it falls back
// to constants.LocalClusterName for environments where the local cluster is
// literally named "local-cluster" (e.g. KinD e2e).
func ResolveLocalClusterManagedClusterName(ctx context.Context, c client.Client) (string, error) {
	if c == nil {
		return "", fmt.Errorf("client is required")
	}

	mcList := &clusterv1.ManagedClusterList{}
	if err := c.List(ctx, mcList, client.MatchingLabels{
		constants.LocalClusterName: "true",
	}); err != nil {
		return "", fmt.Errorf("failed to list local-cluster ManagedClusters: %w", err)
	}

	switch len(mcList.Items) {
	case 0:
		return constants.LocalClusterName, nil
	case 1:
		return mcList.Items[0].Name, nil
	default:
		return "", fmt.Errorf(
			"found %d ManagedClusters labeled local-cluster=true, expected at most one",
			len(mcList.Items),
		)
	}
}
