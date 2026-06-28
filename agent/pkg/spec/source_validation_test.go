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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestSpecEventSourceAllowed(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = migrationv1alpha1.AddToScheme(scheme)

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
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(migrationCR).WithStatusSubresource(migrationCR).Build()
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
}
