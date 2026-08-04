package storage

import (
	"context"
	"testing"

	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	ocv1 "github.com/operator-framework/operator-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestNewSubscription(t *testing.T) {
	sub := NewCrunchySubscription(&globalhubv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "globalhub",
		},
		Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
	}, &subv1alpha1.SubscriptionConfig{}, true)

	if sub.Spec.Package != communityPackageName {
		t.Errorf("Expected package name %s, got %s", communityPackageName, sub.Spec.Package)
	}

	sub = NewCrunchySubscription(&globalhubv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "globalhub",
		},
		Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
	}, &subv1alpha1.SubscriptionConfig{}, false)

	if sub.Spec.Package != packageName {
		t.Errorf("Expected package name %s, got %s", packageName, sub.Spec.Package)
	}

	sub = NewCrunchySubscription(&globalhubv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "globalhub",
		},
		Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
	}, &subv1alpha1.SubscriptionConfig{
		NodeSelector: map[string]string{
			"foo": "bar",
		},
	}, true)

	if sub.Spec.Config.NodeSelector["foo"] != "bar" {
		t.Errorf("Expected nodeSelector name foo value is bar, got %s", sub.Spec.Config.NodeSelector["foo"])
	}
}

func TestRenderSubscription(t *testing.T) {
	sub := ExpectedSubscription(&subv1alpha1.Subscription{
		Spec: &subv1alpha1.SubscriptionSpec{
			Package: packageName,
			Channel: "foo",
		},
	}, &subv1alpha1.SubscriptionConfig{}, true)
	if sub.Spec.Package != communityPackageName {
		t.Errorf("Expected package name %s, got %s", communityPackageName, sub.Spec.Package)
	}

	sub = ExpectedSubscription(&subv1alpha1.Subscription{
		Spec: &subv1alpha1.SubscriptionSpec{
			Package: packageName,
			Channel: "foo",
		},
	}, &subv1alpha1.SubscriptionConfig{}, true)
	if sub.Spec.Package != communityPackageName {
		t.Errorf("Expected package name %s, got %s", communityPackageName, sub.Spec.Package)
	}
}

func TestNewPostgres(t *testing.T) {
	kafka := NewPostgresCluster(config.PostgresName, "default")
	if kafka.Name != config.PostgresName {
		t.Errorf("Expected name %s, got %s", config.PostgresName, kafka.Name)
	}
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := rbacv1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add rbacv1 to scheme: %v", err)
	}
	if err := ocv1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add ocv1 to scheme: %v", err)
	}
	return s
}

func newTestMGH(namespace string) *globalhubv1alpha4.MulticlusterGlobalHub {
	return &globalhubv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mgh",
			Namespace: namespace,
		},
		Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
	}
}

func TestEnsureCrunchyPostgresClusterExtension_CreateFromScratch(t *testing.T) {
	ctx := context.Background()
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build()
	mgh := newTestMGH("test-ns")

	if err := EnsureCrunchyPostgresClusterExtension(ctx, c, mgh); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify SA was created
	sa := &corev1.ServiceAccount{}
	if err := c.Get(ctx, types.NamespacedName{Name: CrunchyInstallerSAName, Namespace: "test-ns"}, sa); err != nil {
		t.Fatalf("expected SA to be created: %v", err)
	}

	// Verify CRB was created
	crb := &rbacv1.ClusterRoleBinding{}
	if err := c.Get(ctx, types.NamespacedName{Name: CrunchyInstallerCRBName}, crb); err != nil {
		t.Fatalf("expected CRB to be created: %v", err)
	}
	if crb.RoleRef.Name != "cluster-admin" {
		t.Errorf("expected cluster-admin roleRef, got %q", crb.RoleRef.Name)
	}

	// Verify CE was created
	ce := &ocv1.ClusterExtension{}
	if err := c.Get(ctx, types.NamespacedName{Name: CrunchyClusterExtensionName}, ce); err != nil {
		t.Fatalf("expected ClusterExtension to be created: %v", err)
	}
	if ce.Spec.Namespace != "test-ns" {
		t.Errorf("expected namespace 'test-ns', got %q", ce.Spec.Namespace)
	}
	if ce.Spec.ServiceAccount.Name != CrunchyInstallerSAName {
		t.Errorf("expected SA %q, got %q", CrunchyInstallerSAName, ce.Spec.ServiceAccount.Name)
	}
}

func TestEnsureCrunchyPostgresClusterExtension_AlreadyExists(t *testing.T) {
	ctx := context.Background()
	mgh := newTestMGH("test-ns")

	existingCE := &ocv1.ClusterExtension{
		ObjectMeta: metav1.ObjectMeta{
			Name: CrunchyClusterExtensionName,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
		},
		Spec: ocv1.ClusterExtensionSpec{
			Namespace: "test-ns",
			ServiceAccount: ocv1.ServiceAccountReference{
				Name: CrunchyInstallerSAName,
			},
			Source: ocv1.SourceConfig{
				SourceType: ocv1.SourceTypeCatalog,
				Catalog: &ocv1.CatalogFilter{
					PackageName: packageName,
					Channels:    []string{channel},
				},
			},
		},
	}

	existingSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CrunchyInstallerSAName,
			Namespace: "test-ns",
		},
	}
	existingCRB := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: CrunchyInstallerCRBName},
		Subjects: []rbacv1.Subject{
			{Kind: rbacv1.ServiceAccountKind, Name: CrunchyInstallerSAName, Namespace: "test-ns"},
		},
		RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "cluster-admin"},
	}

	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).
		WithObjects(existingSA, existingCRB, existingCE).Build()

	if err := EnsureCrunchyPostgresClusterExtension(ctx, c, mgh); err != nil {
		t.Fatalf("unexpected error on idempotent call: %v", err)
	}
}

func TestEnsureCrunchyPostgresClusterExtension_ChannelUpdate(t *testing.T) {
	ctx := context.Background()
	mgh := newTestMGH("test-ns")

	existingCE := &ocv1.ClusterExtension{
		ObjectMeta: metav1.ObjectMeta{
			Name: CrunchyClusterExtensionName,
		},
		Spec: ocv1.ClusterExtensionSpec{
			Namespace: "test-ns",
			ServiceAccount: ocv1.ServiceAccountReference{
				Name: CrunchyInstallerSAName,
			},
			Source: ocv1.SourceConfig{
				SourceType: ocv1.SourceTypeCatalog,
				Catalog: &ocv1.CatalogFilter{
					PackageName: packageName,
					Channels:    []string{"old-channel"},
				},
			},
		},
	}

	existingSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: CrunchyInstallerSAName, Namespace: "test-ns"},
	}
	existingCRB := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: CrunchyInstallerCRBName},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: CrunchyInstallerSAName, Namespace: "test-ns"}},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "cluster-admin"},
	}

	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).
		WithObjects(existingSA, existingCRB, existingCE).Build()

	if err := EnsureCrunchyPostgresClusterExtension(ctx, c, mgh); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify channel was patched
	updated := &ocv1.ClusterExtension{}
	if err := c.Get(ctx, types.NamespacedName{Name: CrunchyClusterExtensionName}, updated); err != nil {
		t.Fatalf("failed to get updated CE: %v", err)
	}
	if len(updated.Spec.Source.Catalog.Channels) != 1 || updated.Spec.Source.Catalog.Channels[0] != channel {
		t.Errorf("expected channel %q, got %v", channel, updated.Spec.Source.Catalog.Channels)
	}
}

func TestEnsureCrunchyPostgresClusterExtension_ImmutableFieldDrift(t *testing.T) {
	ctx := context.Background()
	mgh := newTestMGH("test-ns")

	existingCE := &ocv1.ClusterExtension{
		ObjectMeta: metav1.ObjectMeta{
			Name: CrunchyClusterExtensionName,
		},
		Spec: ocv1.ClusterExtensionSpec{
			Namespace: "different-ns",
			ServiceAccount: ocv1.ServiceAccountReference{
				Name: CrunchyInstallerSAName,
			},
			Source: ocv1.SourceConfig{
				SourceType: ocv1.SourceTypeCatalog,
				Catalog: &ocv1.CatalogFilter{
					PackageName: packageName,
					Channels:    []string{channel},
				},
			},
		},
	}

	existingSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: CrunchyInstallerSAName, Namespace: "test-ns"},
	}
	existingCRB := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: CrunchyInstallerCRBName},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: CrunchyInstallerSAName, Namespace: "test-ns"}},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "cluster-admin"},
	}

	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).
		WithObjects(existingSA, existingCRB, existingCE).Build()

	err := EnsureCrunchyPostgresClusterExtension(ctx, c, mgh)
	if err == nil {
		t.Fatal("expected error for immutable field drift")
	}

	// Verify CE was deleted (will be recreated on next reconcile)
	ce := &ocv1.ClusterExtension{}
	if err := c.Get(ctx, types.NamespacedName{Name: CrunchyClusterExtensionName}, ce); !errors.IsNotFound(err) {
		t.Errorf("expected NotFound for CE after immutable field drift, got: %v", err)
	}
}

func TestEnsureCrunchyPostgresClusterExtension_CRBDrift(t *testing.T) {
	ctx := context.Background()
	mgh := newTestMGH("test-ns")

	existingSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: CrunchyInstallerSAName, Namespace: "test-ns"},
	}
	existingCRB := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: CrunchyInstallerCRBName},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: "wrong-sa", Namespace: "test-ns"}},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "cluster-admin"},
	}

	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).
		WithObjects(existingSA, existingCRB).Build()

	err := EnsureCrunchyPostgresClusterExtension(ctx, c, mgh)
	if err == nil {
		t.Fatal("expected error for CRB drift (requeue to recreate)")
	}

	// Verify CRB was deleted
	crb := &rbacv1.ClusterRoleBinding{}
	if err := c.Get(ctx, types.NamespacedName{Name: CrunchyInstallerCRBName}, crb); !errors.IsNotFound(err) {
		t.Errorf("expected NotFound for CRB after drift, got: %v", err)
	}
}

func TestPruneCrunchyClusterExtensionResources_CEExists(t *testing.T) {
	ctx := context.Background()

	ce := &ocv1.ClusterExtension{
		ObjectMeta: metav1.ObjectMeta{Name: CrunchyClusterExtensionName},
	}
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: CrunchyInstallerSAName, Namespace: "test-ns"},
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: CrunchyInstallerCRBName},
	}

	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).
		WithObjects(ce, sa, crb).Build()

	removed, err := PruneCrunchyClusterExtensionResources(ctx, c, "test-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed {
		t.Error("expected removed=false when CE still exists (requeue)")
	}

	// Verify CE was deleted
	if err := c.Get(ctx, types.NamespacedName{Name: CrunchyClusterExtensionName},
		&ocv1.ClusterExtension{}); !errors.IsNotFound(err) {
		t.Errorf("expected NotFound for CE, got: %v", err)
	}
	// Verify SA and CRB still exist (needed for OLM finalizer work)
	if err := c.Get(ctx, types.NamespacedName{Name: CrunchyInstallerSAName, Namespace: "test-ns"},
		&corev1.ServiceAccount{}); err != nil {
		t.Errorf("expected SA to still exist, got: %v", err)
	}
	if err := c.Get(ctx, types.NamespacedName{Name: CrunchyInstallerCRBName},
		&rbacv1.ClusterRoleBinding{}); err != nil {
		t.Errorf("expected CRB to still exist, got: %v", err)
	}
}

func TestPruneCrunchyClusterExtensionResources_CEGone(t *testing.T) {
	ctx := context.Background()

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: CrunchyInstallerSAName, Namespace: "test-ns"},
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: CrunchyInstallerCRBName},
	}

	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).
		WithObjects(sa, crb).Build()

	removed, err := PruneCrunchyClusterExtensionResources(ctx, c, "test-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !removed {
		t.Error("expected removed=true when CE is gone and CRB+SA are cleaned up")
	}

	// Verify SA and CRB are deleted
	if err := c.Get(ctx, types.NamespacedName{Name: CrunchyInstallerSAName, Namespace: "test-ns"},
		&corev1.ServiceAccount{}); !errors.IsNotFound(err) {
		t.Errorf("expected NotFound for SA, got: %v", err)
	}
	if err := c.Get(ctx, types.NamespacedName{Name: CrunchyInstallerCRBName},
		&rbacv1.ClusterRoleBinding{}); !errors.IsNotFound(err) {
		t.Errorf("expected NotFound for CRB, got: %v", err)
	}
}

func TestPruneCrunchyClusterExtensionResources_AllGone(t *testing.T) {
	ctx := context.Background()
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build()

	removed, err := PruneCrunchyClusterExtensionResources(ctx, c, "test-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !removed {
		t.Error("expected removed=true when all resources already gone")
	}
}
