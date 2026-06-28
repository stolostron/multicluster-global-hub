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

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestIsMigrationDeployingEvent(t *testing.T) {
	deploying := utils.ToCloudEvent(string(enum.ManagedClusterMigrationType), "hub1", "hub2", nil)
	deploying.SetExtension(constants.CloudEventExtensionKeyMigrationStage, migrationv1alpha1.PhaseDeploying)

	initializing := utils.ToCloudEvent(string(enum.ManagedClusterMigrationType), constants.CloudEventGlobalHubClusterName, "hub2", nil)
	initializing.SetExtension(constants.CloudEventExtensionKeyMigrationStage, migrationv1alpha1.PhaseInitializing)

	if !IsMigrationDeployingEvent(&deploying) {
		t.Fatal("expected deploying migration event to be recognized")
	}
	if IsMigrationDeployingEvent(&initializing) {
		t.Fatal("expected non-deploying migration event to be rejected")
	}
}

func TestMigrationSourceAllowed(t *testing.T) {
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

	if !MigrationSourceAllowed(ctx, fakeClient, constants.CloudEventGlobalHubClusterName, "hub2") {
		t.Fatal("expected manager global-hub source to be allowed")
	}
	if !MigrationSourceAllowed(ctx, fakeClient, "hub1", "hub2") {
		t.Fatal("expected registered source hub to be allowed during deploying phase")
	}
	if MigrationSourceAllowed(ctx, fakeClient, "spoofed-hub", "hub2") {
		t.Fatal("expected spoofed source hub to be rejected")
	}
}

func TestIsMigrationDeployResourceAllowed(t *testing.T) {
	mc := &unstructured.Unstructured{}
	mc.SetGroupVersionKind(schema.GroupVersionKind{Group: clusterv1.GroupName, Version: "v1", Kind: "ManagedCluster"})
	mc.SetName("cluster1")

	clusterRole := &unstructured.Unstructured{}
	clusterRole.SetGroupVersionKind(rbacv1.SchemeGroupVersion.WithKind("ClusterRole"))
	clusterRole.SetName("admin")

	secret := &unstructured.Unstructured{}
	secret.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
	secret.SetNamespace("cluster1")
	secret.SetName("bootstrap")

	foreignSecret := secret.DeepCopy()
	foreignSecret.SetNamespace("kube-system")

	cases := []struct {
		name        string
		resource    *unstructured.Unstructured
		clusterName string
		allowed     bool
	}{
		{name: "managed cluster allowed", resource: mc, clusterName: "cluster1", allowed: true},
		{name: "cluster role denied", resource: clusterRole, clusterName: "cluster1", allowed: false},
		{name: "cluster secret allowed", resource: secret, clusterName: "cluster1", allowed: true},
		{name: "foreign namespace secret denied", resource: foreignSecret, clusterName: "cluster1", allowed: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsMigrationDeployResourceAllowed(tc.resource, tc.clusterName)
			if got != tc.allowed {
				t.Fatalf("IsMigrationDeployResourceAllowed() = %v, want %v", got, tc.allowed)
			}
		})
	}
}
