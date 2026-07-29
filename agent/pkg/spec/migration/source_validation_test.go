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
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	migrationbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestMigrationSourceAllowed(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	assert.NoError(t, migrationv1alpha1.AddToScheme(scheme))

	t.Run("allows global hub", func(t *testing.T) {
		assert.True(t, MigrationSourceAllowed(ctx, nil, constants.CloudEventGlobalHubClusterName, "hub2"))
	})

	t.Run("allows registered local migration", func(t *testing.T) {
		t.Cleanup(func() {
			localMigrationMu.Lock()
			localMigrations = make(map[string]localMigrationRecord)
			localMigrationMu.Unlock()
		})
		assert.NoError(t, EnsureLocalMigrationCR(ctx, nil, "hub2", &migrationbundle.MigrationTargetBundle{
			FromHub:                   "hub1",
			ManagedClusters:           []string{"c1"},
			ManagedServiceAccountName: "msa",
		}, migrationv1alpha1.PhaseDeploying))
		assert.True(t, MigrationSourceAllowed(ctx, nil, "hub1", "hub2"))
		assert.False(t, MigrationSourceAllowed(ctx, nil, "evil", "hub2"))
	})

	t.Run("allows migration CR on agent", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&migrationv1alpha1.ManagedClusterMigration{
			ObjectMeta: metav1.ObjectMeta{Name: "msa"},
			Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
				From: "hub1",
				To:   "hub2",
			},
			Status: migrationv1alpha1.ManagedClusterMigrationStatus{
				Phase: migrationv1alpha1.PhaseDeploying,
			},
		}).Build()
		assert.True(t, MigrationSourceAllowed(ctx, c, "hub1", "hub2"))
	})
}

func TestIsMigrationDeployResourceAllowed(t *testing.T) {
	mc := &unstructured.Unstructured{}
	mc.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	mc.SetName("cluster1")

	assert.True(t, IsMigrationDeployResourceAllowed(mc, "cluster1"))
	assert.False(t, IsMigrationDeployResourceAllowed(mc, "cluster2"))

	rbac := &unstructured.Unstructured{}
	rbac.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole",
	})
	assert.False(t, IsMigrationDeployResourceAllowed(rbac, "cluster1"))
}

func TestIsMigrationDeployingEvent(t *testing.T) {
	evt := cloudevents.NewEvent()
	evt.SetType(constants.MigrationTargetMsgKey)
	evt.SetSource("hub1")
	assert.True(t, IsMigrationDeployingEvent(&evt))

	globalEvt := cloudevents.NewEvent()
	globalEvt.SetType(constants.MigrationTargetMsgKey)
	globalEvt.SetSource(constants.CloudEventGlobalHubClusterName)
	assert.False(t, IsMigrationDeployingEvent(&globalEvt))
}
