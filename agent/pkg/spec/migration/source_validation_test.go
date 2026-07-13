// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	migrationbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestIsMigrationDeployingEvent(t *testing.T) {
	t.Run("nil event", func(t *testing.T) {
		if IsMigrationDeployingEvent(nil) {
			t.Fatal("expected nil event to be rejected")
		}
	})

	t.Run("non-migration type", func(t *testing.T) {
		wrongType := utils.ToCloudEvent("Policy", "hub1", "hub2", nil)
		if IsMigrationDeployingEvent(&wrongType) {
			t.Fatal("expected non-migration event type to be rejected")
		}
	})

	t.Run("deploying phase", func(t *testing.T) {
		deploying := utils.ToCloudEvent(string(enum.ManagedClusterMigrationType), "hub1", "hub2", nil)
		deploying.SetExtension(constants.CloudEventExtensionKeyMigrationStage, migrationv1alpha1.PhaseDeploying)
		if !IsMigrationDeployingEvent(&deploying) {
			t.Fatal("expected deploying migration event to be recognized")
		}
	})

	t.Run("non-deploying phase", func(t *testing.T) {
		initializing := utils.ToCloudEvent(
			string(enum.ManagedClusterMigrationType),
			constants.CloudEventGlobalHubClusterName,
			"hub2",
			nil,
		)
		initializing.SetExtension(constants.CloudEventExtensionKeyMigrationStage, migrationv1alpha1.PhaseInitializing)
		if IsMigrationDeployingEvent(&initializing) {
			t.Fatal("expected non-deploying migration event to be rejected")
		}
	})
}

func resetLocalMigrationState() {
	localMigrationMu.Lock()
	defer localMigrationMu.Unlock()
	localMigrations = make(map[string]localMigrationRecord)
}

func TestMigrationSourceAllowed(t *testing.T) {
	resetLocalMigrationState()
	t.Cleanup(resetLocalMigrationState)
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
	deployingClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(migrationCR).
		WithStatusSubresource(migrationCR).
		Build()

	wrongPhaseCR := migrationCR.DeepCopy()
	wrongPhaseCR.Name = "migration-2"
	wrongPhaseCR.Status.Phase = migrationv1alpha1.PhaseRegistering
	initializingClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(wrongPhaseCR).
		WithStatusSubresource(wrongPhaseCR).
		Build()

	ctx := context.Background()

	cases := []struct {
		name        string
		client      client.Client
		source      string
		target      string
		wantAllowed bool
		setup       func()
	}{
		{
			name:        "manager global-hub source",
			client:      deployingClient,
			source:      constants.CloudEventGlobalHubClusterName,
			target:      "hub2",
			wantAllowed: true,
		},
		{
			name:        "registered source hub during deploying",
			client:      deployingClient,
			source:      "hub1",
			target:      "hub2",
			wantAllowed: true,
		},
		{
			name:        "spoofed source hub",
			client:      deployingClient,
			source:      "spoofed-hub",
			target:      "hub2",
			wantAllowed: false,
		},
		{
			name:        "unregistered target hub",
			client:      deployingClient,
			source:      "hub1",
			target:      "hub3",
			wantAllowed: false,
		},
		{
			name:   "in-memory registered source hub during deploying",
			client: nil,
			setup: func() {
				if err := EnsureLocalMigrationCR(ctx, nil, "hub2", sampleMigrationTargetBundle(), migrationv1alpha1.PhaseDeploying); err != nil {
					t.Fatalf("EnsureLocalMigrationCR() error = %v", err)
				}
			},
			source:      "hub1",
			target:      "hub2",
			wantAllowed: true,
		},
		{
			name:        "nil client without in-memory registration",
			client:      nil,
			source:      "hub1",
			target:      "hub2",
			wantAllowed: false,
		},
		{
			name: "registered source hub during initializing",
			client: func() client.Client {
				initializingCR := migrationCR.DeepCopy()
				initializingCR.Name = "migration-3"
				initializingCR.Status.Phase = migrationv1alpha1.PhaseInitializing
				return fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(initializingCR).
					WithStatusSubresource(initializingCR).
					Build()
			}(),
			source:      "hub1",
			target:      "hub2",
			wantAllowed: true,
		},
		{
			name:        "wrong migration phase",
			client:      initializingClient,
			source:      "hub1",
			target:      "hub2",
			wantAllowed: false,
		},
		{
			name:        "empty source",
			client:      deployingClient,
			source:      "",
			target:      "hub2",
			wantAllowed: false,
		},
		{
			name:        "empty target hub",
			client:      deployingClient,
			source:      "hub1",
			target:      "",
			wantAllowed: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetLocalMigrationState()
			if tc.setup != nil {
				tc.setup()
			}
			got := MigrationSourceAllowed(ctx, tc.client, tc.source, tc.target)
			if got != tc.wantAllowed {
				t.Fatalf("MigrationSourceAllowed(%q, %q) = %v, want %v", tc.source, tc.target, got, tc.wantAllowed)
			}
		})
	}
}

func TestIsMigrationDeployResourceAllowed_AllowedGVKs(t *testing.T) {
	namespace := &unstructured.Unstructured{}
	namespace.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Namespace"))
	namespace.SetName("cluster1")

	manifestWork := &unstructured.Unstructured{}
	manifestWork.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "work.open-cluster-management.io", Version: "v1", Kind: "ManifestWork",
	})
	manifestWork.SetNamespace("cluster1")
	manifestWork.SetName("work1")

	configMap := &unstructured.Unstructured{}
	configMap.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	configMap.SetNamespace("cluster1")
	configMap.SetName("extra-manifests")

	foreignManifestWork := manifestWork.DeepCopy()
	foreignManifestWork.SetNamespace("kube-system")

	foreignNamespace := namespace.DeepCopy()
	foreignNamespace.SetName("other-cluster")

	cases := []struct {
		name        string
		resource    *unstructured.Unstructured
		clusterName string
		allowed     bool
	}{
		{name: "namespace allowed", resource: namespace, clusterName: "cluster1", allowed: true},
		{name: "manifest work allowed", resource: manifestWork, clusterName: "cluster1", allowed: true},
		{name: "configmap allowed", resource: configMap, clusterName: "cluster1", allowed: true},
		{name: "foreign manifest work denied", resource: foreignManifestWork, clusterName: "cluster1", allowed: false},
		{name: "foreign namespace denied", resource: foreignNamespace, clusterName: "cluster1", allowed: false},
		{name: "nil resource denied", resource: nil, clusterName: "cluster1", allowed: false},
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

func TestIsMigrationDeployResourceAllowed(t *testing.T) {
	mc := &unstructured.Unstructured{}
	mc.SetGroupVersionKind(schema.GroupVersionKind{Group: clusterv1.GroupName, Version: "v1", Kind: "ManagedCluster"})
	mc.SetName("cluster1")

	foreignMC := mc.DeepCopy()
	foreignMC.SetName("other-cluster")

	kac := &unstructured.Unstructured{}
	kac.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "agent.open-cluster-management.io", Version: "v1", Kind: "KlusterletAddonConfig",
	})
	kac.SetNamespace("cluster1")
	kac.SetName("cluster1")

	foreignKAC := kac.DeepCopy()
	foreignKAC.SetName("other-cluster")

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
		{name: "foreign managed cluster denied", resource: foreignMC, clusterName: "cluster1", allowed: false},
		{name: "klusterlet addon config allowed", resource: kac, clusterName: "cluster1", allowed: true},
		{name: "foreign klusterlet addon config denied", resource: foreignKAC, clusterName: "cluster1", allowed: false},
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

func sampleMigrationTargetBundle() *migrationbundle.MigrationTargetBundle {
	return &migrationbundle.MigrationTargetBundle{
		FromHub:                   "hub1",
		ManagedServiceAccountName: "migration-hub1-cluster1",
		ManagedClusters:           []string{"cluster1"},
	}
}

func TestEnsureLocalMigrationCR(t *testing.T) {
	resetLocalMigrationState()
	t.Cleanup(resetLocalMigrationState)

	ctx := context.Background()
	event := sampleMigrationTargetBundle()
	if err := EnsureLocalMigrationCR(ctx, nil, "hub2", event, migrationv1alpha1.PhaseDeploying); err != nil {
		t.Fatalf("EnsureLocalMigrationCR() error = %v", err)
	}
	if !MigrationSourceAllowed(ctx, nil, "hub1", "hub2") {
		t.Fatal("expected in-memory migration state to authorize source hub")
	}
}

func TestEnsureLocalMigrationCR_UpdatesExistingCR(t *testing.T) {
	resetLocalMigrationState()
	t.Cleanup(resetLocalMigrationState)

	ctx := context.Background()
	event := sampleMigrationTargetBundle()
	if err := EnsureLocalMigrationCR(ctx, nil, "hub2", event, migrationv1alpha1.PhaseInitializing); err != nil {
		t.Fatalf("EnsureLocalMigrationCR() error = %v", err)
	}
	if err := EnsureLocalMigrationCR(ctx, nil, "hub2", event, migrationv1alpha1.PhaseDeploying); err != nil {
		t.Fatalf("EnsureLocalMigrationCR() error = %v", err)
	}
	if !MigrationSourceAllowed(ctx, nil, "hub1", "hub2") {
		t.Fatal("expected updated in-memory migration state to authorize source hub")
	}
}

func TestEnsureLocalMigrationCR_ReusesExistingNamespace(t *testing.T) {
	resetLocalMigrationState()
	t.Cleanup(resetLocalMigrationState)

	ctx := context.Background()
	if err := EnsureLocalMigrationCR(ctx, nil, "hub2", sampleMigrationTargetBundle(), migrationv1alpha1.PhaseInitializing); err != nil {
		t.Fatalf("EnsureLocalMigrationCR() error = %v", err)
	}
	if !MigrationSourceAllowed(ctx, nil, "hub1", "hub2") {
		t.Fatal("expected in-memory migration state to authorize source hub")
	}
}

func TestEnsureLocalMigrationCR_InvalidInputs(t *testing.T) {
	resetLocalMigrationState()
	t.Cleanup(resetLocalMigrationState)

	ctx := context.Background()
	event := sampleMigrationTargetBundle()

	cases := []struct {
		name    string
		target  string
		event   *migrationbundle.MigrationTargetBundle
		wantErr string
	}{
		{
			name:    "nil event",
			target:  "hub2",
			event:   nil,
			wantErr: "invalid migration CR inputs",
		},
		{
			name:    "empty target hub",
			target:  "",
			event:   event,
			wantErr: "invalid migration CR inputs",
		},
		{
			name:   "missing managed service account name",
			target: "hub2",
			event: &migrationbundle.MigrationTargetBundle{
				FromHub:         "hub1",
				ManagedClusters: []string{"cluster1"},
			},
			wantErr: "managedServiceAccountName is required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := EnsureLocalMigrationCR(ctx, nil, tc.target, tc.event, migrationv1alpha1.PhaseDeploying)
			if err == nil || err.Error() == "" {
				t.Fatal("expected error")
			}
			if tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestEnsureLocalMigrationCR_NoOpWithoutFromHubOrClusters(t *testing.T) {
	resetLocalMigrationState()
	t.Cleanup(resetLocalMigrationState)

	ctx := context.Background()
	cases := []*migrationbundle.MigrationTargetBundle{
		{
			ManagedServiceAccountName: "migration-hub1-cluster1",
			ManagedClusters:           []string{"cluster1"},
		},
		{
			FromHub:                   "hub1",
			ManagedServiceAccountName: "migration-hub1-cluster1",
		},
	}

	for i, event := range cases {
		if err := EnsureLocalMigrationCR(ctx, nil, "hub2", event, migrationv1alpha1.PhaseDeploying); err != nil {
			t.Fatalf("case %d: expected no-op, got error %v", i, err)
		}
	}

	if MigrationSourceAllowed(ctx, nil, "hub1", "hub2") {
		t.Fatal("expected no in-memory migration state to be recorded")
	}
}

func TestDeleteLocalMigrationCR(t *testing.T) {
	resetLocalMigrationState()
	t.Cleanup(resetLocalMigrationState)

	ctx := context.Background()
	if err := EnsureLocalMigrationCR(ctx, nil, "hub2", sampleMigrationTargetBundle(), migrationv1alpha1.PhaseDeploying); err != nil {
		t.Fatalf("EnsureLocalMigrationCR() error = %v", err)
	}

	if err := DeleteLocalMigrationCR(ctx, nil, "migration-hub1-cluster1"); err != nil {
		t.Fatalf("DeleteLocalMigrationCR() error = %v", err)
	}
	if MigrationSourceAllowed(ctx, nil, "hub1", "hub2") {
		t.Fatal("expected migration state to be deleted")
	}

	if err := DeleteLocalMigrationCR(ctx, nil, "missing-migration"); err != nil {
		t.Fatalf("deleting missing migration state should succeed, got %v", err)
	}
	if err := DeleteLocalMigrationCR(ctx, nil, ""); err != nil {
		t.Fatalf("empty migration name should be ignored, got %v", err)
	}
}

func TestMigrationSourceAllowed_ListError(t *testing.T) {
	resetLocalMigrationState()
	t.Cleanup(resetLocalMigrationState)

	ctx := context.Background()
	clientWithoutScheme := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	if MigrationSourceAllowed(ctx, clientWithoutScheme, "hub1", "hub2") {
		t.Fatal("expected list failure to reject migration source")
	}
}

func TestIsMigrationDeployResourceAllowed_EmptyClusterName(t *testing.T) {
	secret := &unstructured.Unstructured{}
	secret.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
	secret.SetNamespace("cluster1")
	secret.SetName("bootstrap")
	if IsMigrationDeployResourceAllowed(secret, "") {
		t.Fatal("expected empty cluster name to be rejected")
	}
}
