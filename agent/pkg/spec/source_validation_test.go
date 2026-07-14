// Copyright Contributors to the Open Cluster Management project.
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

package spec

import (
	"context"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func activeHubManagedCluster(name string) *clusterv1.ManagedCluster {
	return &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				constants.GHHubRoleLabelKey: constants.GHHubRoleActive,
			},
		},
	}
}

func TestSpecEventSourceAllowed(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = migrationv1alpha1.AddToScheme(scheme)
	_ = clusterv1.Install(scheme)

	migrationCR := &migrationv1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{Name: "migration-1"},
		Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
			From: "hub1",
			To:   "hub2",
		},
		Status: migrationv1alpha1.ManagedClusterMigrationStatus{
			Phase: migrationv1alpha1.PhaseDeploying,
		},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(migrationCR, activeHubManagedCluster("hub1")).
		WithStatusSubresource(migrationCR).
		Build()
	ctx := context.Background()

	agentConfig := &configs.AgentConfig{
		LeafHubName: "hub2",
	}
	agentConfig.SetHubRole(constants.GHHubRoleStandby)

	policyEvent := utils.ToCloudEvent("Policy", constants.CloudEventGlobalHubClusterName, "hub2", nil)
	spoofedPolicyEvent := utils.ToCloudEvent("Policy", "spoofed-hub", "hub2", nil)

	deployingEvent := utils.ToCloudEvent(string(enum.ManagedClusterMigrationType), "hub1", "hub2", nil)
	deployingEvent.SetExtension(constants.CloudEventExtensionKeyMigrationStage, migrationv1alpha1.PhaseDeploying)

	haResourcesEvent := utils.ToCloudEvent(constants.HubHAResourcesMsgKey, "hub1", "hub2", nil)
	haConfigEvent := utils.ToCloudEvent(constants.HAConfigMsgKey, "local-cluster", "hub2", nil)

	if !specEventSourceAllowed(ctx, fakeClient, agentConfig, &policyEvent, "hub2") {
		t.Fatal("expected trusted manager policy event to be allowed")
	}
	if specEventSourceAllowed(ctx, fakeClient, agentConfig, &spoofedPolicyEvent, "hub2") {
		t.Fatal("expected spoofed policy source to be rejected")
	}
	if !specEventSourceAllowed(ctx, fakeClient, agentConfig, &deployingEvent, "hub2") {
		t.Fatal("expected registered migration deploying source to be allowed")
	}
	if !specEventSourceAllowed(ctx, fakeClient, agentConfig, &haResourcesEvent, "hub2") {
		t.Fatal("expected active hub HA resource event to be allowed on standby hub")
	}
	if !specEventSourceAllowed(ctx, fakeClient, agentConfig, &haConfigEvent, "hub2") {
		t.Fatal("expected HA config event addressed to leaf hub to be allowed")
	}

	spoofedHAEvent := utils.ToCloudEvent(constants.HubHAResourcesMsgKey, "spoofed-hub", "hub2", nil)
	if specEventSourceAllowed(ctx, fakeClient, agentConfig, &spoofedHAEvent, "hub2") {
		t.Fatal("expected spoofed HA resource source to be rejected")
	}

	activeHubConfig := &configs.AgentConfig{LeafHubName: "hub2"}
	activeHubConfig.SetHubRole(constants.GHHubRoleActive)
	if specEventSourceAllowed(ctx, fakeClient, activeHubConfig, &haResourcesEvent, "hub2") {
		t.Fatal("expected HA resource event to be rejected on active hub")
	}

	standbyWithPeer := &configs.AgentConfig{LeafHubName: "hub2"}
	standbyWithPeer.SetHubRole(constants.GHHubRoleStandby)
	standbyWithPeer.SetStandbyHub("hub1")
	selfSourcedHAConfig := utils.ToCloudEvent(constants.HAConfigMsgKey, "hub2", "hub2", nil)
	if specEventSourceAllowed(ctx, fakeClient, standbyWithPeer, &selfSourcedHAConfig, "hub2") {
		t.Fatal("expected HA config event with source equal to subject to be rejected")
	}

	unexpectedStandbyHAConfig := utils.ToCloudEvent(constants.HAConfigMsgKey, "hub3", "hub2", nil)
	if specEventSourceAllowed(ctx, fakeClient, standbyWithPeer, &unexpectedStandbyHAConfig, "hub2") {
		t.Fatal("expected HA config event from unexpected standby hub to be rejected")
	}
}

func TestSpecEventSourceAllowed_NilInputs(t *testing.T) {
	ctx := context.Background()
	agentConfig := &configs.AgentConfig{LeafHubName: "hub2"}

	if specEventSourceAllowed(ctx, nil, agentConfig, nil, "hub2") {
		t.Fatal("expected nil event to be rejected")
	}
	if specEventSourceAllowed(ctx, nil, nil, &cloudevents.Event{}, "hub2") {
		t.Fatal("expected nil agent config to be rejected")
	}
}

func TestHubHAResourceSourceAllowed(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clusterv1.Install(scheme)
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(activeHubManagedCluster("hub1")).
		Build()
	ctx := context.Background()

	standby := &configs.AgentConfig{LeafHubName: "hub2"}
	standby.SetHubRole(constants.GHHubRoleStandby)

	if !hubHAResourceSourceAllowed(ctx, fakeClient, standby, "hub1", "hub2") {
		t.Fatal("expected peer active hub source to be allowed")
	}
	if hubHAResourceSourceAllowed(ctx, fakeClient, standby, "spoofed-hub", "hub2") {
		t.Fatal("expected unexpected leaf hub source to be rejected")
	}
	if hubHAResourceSourceAllowed(ctx, fakeClient, standby, "", "hub2") {
		t.Fatal("expected empty source to be rejected")
	}
	if hubHAResourceSourceAllowed(ctx, fakeClient, standby, constants.CloudEventGlobalHubClusterName, "hub2") {
		t.Fatal("expected global-hub source to be rejected for HA resources")
	}
	if hubHAResourceSourceAllowed(ctx, fakeClient, standby, "hub2", "hub2") {
		t.Fatal("expected self source to be rejected")
	}
	if hubHAResourceSourceAllowed(ctx, fakeClient, standby, "hub1", "hub3") {
		t.Fatal("expected subject mismatch to be rejected")
	}

	active := &configs.AgentConfig{LeafHubName: "hub2"}
	active.SetHubRole(constants.GHHubRoleActive)
	if hubHAResourceSourceAllowed(ctx, fakeClient, active, "hub1", "hub2") {
		t.Fatal("expected active hub role to reject HA resource events")
	}

	clientWithoutActiveHub := fake.NewClientBuilder().WithScheme(scheme).Build()
	if hubHAResourceSourceAllowed(ctx, clientWithoutActiveHub, standby, "hub1", "hub2") {
		t.Fatal("expected missing active hub label to reject HA resource events")
	}
}

func TestExpectedActiveHubName(t *testing.T) {
	ctx := context.Background()

	if name, ok := expectedActiveHubName(ctx, nil); ok || name != "" {
		t.Fatal("expected nil client to fail lookup")
	}

	scheme := runtime.NewScheme()
	_ = clusterv1.Install(scheme)
	clientWithoutScheme := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	if _, ok := expectedActiveHubName(ctx, clientWithoutScheme); ok {
		t.Fatal("expected list failure without cluster scheme")
	}

	clientWithoutActiveHub := fake.NewClientBuilder().WithScheme(scheme).Build()
	if _, ok := expectedActiveHubName(ctx, clientWithoutActiveHub); ok {
		t.Fatal("expected missing active hub to fail lookup")
	}

	multipleActiveHubs := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(activeHubManagedCluster("hub1"), activeHubManagedCluster("hub2")).
		Build()
	if _, ok := expectedActiveHubName(ctx, multipleActiveHubs); ok {
		t.Fatal("expected multiple active hubs to fail lookup")
	}

	singleActiveHub := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(activeHubManagedCluster("hub1")).
		Build()
	name, ok := expectedActiveHubName(ctx, singleActiveHub)
	if !ok || name != "hub1" {
		t.Fatalf("expected active hub hub1, got name=%q ok=%v", name, ok)
	}
}

func TestSpecEventSourceAllowed_GlobalHubSourceBypassesTypeChecks(t *testing.T) {
	ctx := context.Background()
	agentConfig := &configs.AgentConfig{LeafHubName: "hub2"}

	unknownType := utils.ToCloudEvent("UnknownType", constants.CloudEventGlobalHubClusterName, "hub2", nil)
	if !specEventSourceAllowed(ctx, nil, agentConfig, &unknownType, "hub2") {
		t.Fatal("expected global-hub source to be allowed regardless of event type")
	}
}

func TestHaConfigSourceAllowed(t *testing.T) {
	cfg := &configs.AgentConfig{LeafHubName: "hub2"}
	cfg.SetStandbyHub("hub1")

	if !haConfigSourceAllowed(cfg, constants.CloudEventGlobalHubClusterName, "hub2") {
		t.Fatal("expected manager global-hub HA config source to be allowed")
	}
	if !haConfigSourceAllowed(cfg, "hub1", "hub2") {
		t.Fatal("expected configured standby hub source to be allowed")
	}
	if haConfigSourceAllowed(cfg, "hub3", "hub2") {
		t.Fatal("expected unexpected standby hub source to be rejected")
	}
	if haConfigSourceAllowed(cfg, "", "hub2") {
		t.Fatal("expected empty source to be rejected")
	}
	if haConfigSourceAllowed(cfg, "hub2", "hub3") {
		t.Fatal("expected subject mismatch to be rejected")
	}

	cfgWithoutPeer := &configs.AgentConfig{LeafHubName: "hub2"}
	if !haConfigSourceAllowed(cfgWithoutPeer, "any-peer", "hub2") {
		t.Fatal("expected peer source to be allowed when standby hub is not configured")
	}
}
