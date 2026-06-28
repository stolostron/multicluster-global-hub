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

package migration

import (
	"context"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cetypes "github.com/cloudevents/sdk-go/v2/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

var migrationDeployAllowedGVKs = map[schema.GroupVersionKind]struct{}{
	{Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster"}:      {},
	{Group: "agent.open-cluster-management.io", Version: "v1", Kind: "KlusterletAddonConfig"}: {},
	{Group: "", Version: "v1", Kind: "Secret"}:                                                {},
	{Group: "", Version: "v1", Kind: "Namespace"}:                                             {},
	{Group: "work.open-cluster-management.io", Version: "v1", Kind: "ManifestWork"}:           {},
}

// IsMigrationDeployingEvent reports whether evt is a migration resource deploying event.
func IsMigrationDeployingEvent(evt *cloudevents.Event) bool {
	if evt == nil || evt.Type() != string(enum.ManagedClusterMigrationType) {
		return false
	}

	stage, err := cetypes.ToString(evt.Extensions()[constants.CloudEventExtensionKeyMigrationStage])
	return err == nil && stage == migrationv1alpha1.PhaseDeploying
}

// MigrationSourceAllowed returns true when source may publish migration events to targetHub.
// Manager orchestration uses global-hub; source-hub deploying uses the registered from hub.
func MigrationSourceAllowed(ctx context.Context, c client.Client, source, targetHub string) bool {
	if source == constants.CloudEventGlobalHubClusterName {
		return true
	}
	if source == "" || targetHub == "" || c == nil {
		return false
	}

	list := &migrationv1alpha1.ManagedClusterMigrationList{}
	if err := c.List(ctx, list); err != nil {
		return false
	}

	for i := range list.Items {
		migration := &list.Items[i]
		if migration.Spec.From != source || migration.Spec.To != targetHub {
			continue
		}
		if migration.Status.Phase == migrationv1alpha1.PhaseDeploying {
			return true
		}
	}

	return false
}

// IsMigrationDeployResourceAllowed validates GVK and namespace for deploying-stage resources.
func IsMigrationDeployResourceAllowed(resource *unstructured.Unstructured, clusterName string) bool {
	if resource == nil {
		return false
	}

	gvk := resource.GroupVersionKind()
	if gvk.Group == "rbac.authorization.k8s.io" {
		return false
	}

	if _, ok := migrationDeployAllowedGVKs[gvk]; !ok {
		return false
	}

	if gvk.Kind == "Secret" && resource.GetNamespace() != clusterName {
		return false
	}

	return true
}
