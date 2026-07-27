// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"context"
	"fmt"
	"strings"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const globalHubStandbySubjectSeparator = "/"

// FormatGlobalHubStandbySubject returns the Kafka subject / standbyHub config value for the
// global-hub local standby agent. The prefix disambiguates the global hub local ManagedCluster
// from regional hubs that may use the same local cluster name (e.g. acm-local-cluster).
func FormatGlobalHubStandbySubject(localManagedClusterName string) string {
	if localManagedClusterName == "" {
		return ""
	}
	return constants.CloudEventGlobalHubClusterName + globalHubStandbySubjectSeparator + localManagedClusterName
}

// ResolveGlobalHubStandbySubject strips the global-hub prefix from subject when present.
func ResolveGlobalHubStandbySubject(subject string) (localManagedClusterName string, ok bool) {
	prefix := constants.CloudEventGlobalHubClusterName + globalHubStandbySubjectSeparator
	if !strings.HasPrefix(subject, prefix) {
		return "", false
	}
	name := strings.TrimPrefix(subject, prefix)
	if name == "" {
		return "", false
	}
	return name, true
}

// MatchesGlobalHubStandbySubject reports whether subject routes to the agent whose
// leaf-hub-name is leafHubName. Accepts both prefixed (global-hub/<name>) and legacy
// unprefixed subjects for backward compatibility during upgrades.
func MatchesGlobalHubStandbySubject(subject, leafHubName string) bool {
	if subject == leafHubName {
		return true
	}
	if localName, ok := ResolveGlobalHubStandbySubject(subject); ok {
		return localName == leafHubName
	}
	return false
}

// StandbyHubSourceMatches reports whether an HA config event source matches the configured
// standbyHub value, accounting for prefixed and unprefixed forms.
func StandbyHubSourceMatches(source, configuredStandbyHub string) bool {
	if source == configuredStandbyHub {
		return true
	}
	if localName, ok := ResolveGlobalHubStandbySubject(configuredStandbyHub); ok {
		return source == localName
	}
	if localName, ok := ResolveGlobalHubStandbySubject(source); ok {
		return localName == configuredStandbyHub
	}
	return false
}

// FindStandbyHubTarget resolves the standby hub identity for active Hub HA agents.
// cachedLocalClusterName is optional (from operator local-agent reconciler); pass "" when unavailable.
func FindStandbyHubTarget(ctx context.Context, c client.Client, cachedLocalClusterName string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("client is required")
	}

	localName, err := ResolveLocalClusterManagedClusterName(ctx, c)
	if err != nil {
		return "", fmt.Errorf("failed to resolve local ManagedCluster name: %w", err)
	}
	if localName == constants.LocalClusterName && cachedLocalClusterName != "" {
		localName = cachedLocalClusterName
	}

	// Hub HA standby runs on the global-hub local agent. When the local MC has a
	// non-default name, prefer it over hub-role=standby labels on other MCs (e.g. a stale
	// literal "local-cluster" MC on ACM 5.0 hub-self-managed setups).
	if localName != constants.LocalClusterName {
		return FormatGlobalHubStandbySubject(localName), nil
	}

	standbyMCs, err := listStandbyRoleManagedClusters(ctx, c)
	if err != nil {
		return "", err
	}

	switch len(standbyMCs) {
	case 0:
		return FormatGlobalHubStandbySubject(constants.LocalClusterName), nil
	case 1:
		name := standbyMCs[0].Name
		if name == constants.LocalClusterName {
			return FormatGlobalHubStandbySubject(name), nil
		}
		// Separate regional standby hub (legacy topology): subject is the MC name.
		return name, nil
	default:
		chosen := alphabeticallyFirstClusterName(standbyMCs)
		if chosen == constants.LocalClusterName {
			return FormatGlobalHubStandbySubject(chosen), nil
		}
		return chosen, nil
	}
}

// listStandbyRoleManagedClusters returns ManagedClusters labeled hub-role=standby.
func listStandbyRoleManagedClusters(ctx context.Context, c client.Client) ([]clusterv1.ManagedCluster, error) {
	list := &clusterv1.ManagedClusterList{}
	if err := c.List(ctx, list, client.MatchingLabels{
		constants.GHHubRoleLabelKey: constants.GHHubRoleStandby,
	}); err != nil {
		return nil, fmt.Errorf("failed to list ManagedClusters with standby role: %w", err)
	}
	return list.Items, nil
}

// alphabeticallyFirstClusterName returns the lexicographically smallest cluster name.
func alphabeticallyFirstClusterName(clusters []clusterv1.ManagedCluster) string {
	if len(clusters) == 0 {
		return ""
	}
	chosen := clusters[0].Name
	for _, mc := range clusters[1:] {
		if mc.Name < chosen {
			chosen = mc.Name
		}
	}
	return chosen
}
