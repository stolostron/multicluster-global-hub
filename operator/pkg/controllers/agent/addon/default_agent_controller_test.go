package addon

import (
	"context"
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"open-cluster-management.io/api/addon/v1alpha1"
	v1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	operatorv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	operatortrans "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter/protocol"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func fakeCluster(name, hostingCluster, addonDeployMode string) *v1.ManagedCluster {
	cluster := &v1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.ManagedClusterSpec{},
	}
	labels := map[string]string{
		constants.GHDeployModeLabelKey: addonDeployMode,
	}
	cluster.SetLabels(labels)

	if hostingCluster != "" {
		annotations := map[string]string{
			constants.AnnotationClusterDeployMode:         constants.ClusterDeployModeHosted,
			constants.AnnotationClusterHostingClusterName: hostingCluster,
		}
		cluster.SetAnnotations(annotations)
	}

	return cluster
}

func fakeClusterManagementAddon() *v1alpha1.ClusterManagementAddOn {
	return &v1alpha1.ClusterManagementAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.GHClusterManagementAddonName,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
		},
	}
}

func fakeMGH(namespace, name string) *operatorv1alpha4.MulticlusterGlobalHub {
	mgh := &operatorv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: operatorv1alpha4.MulticlusterGlobalHubStatus{
			Conditions: []metav1.Condition{
				{
					Type:   config.CONDITION_TYPE_GLOBALHUB_READY,
					Status: metav1.ConditionTrue,
				},
			},
		},
	}
	return mgh
}

// go test -run ^TestAddonInstaller$ github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/agent/addon
func TestAddonInstaller(t *testing.T) {
	namespace := "multicluster-global-hub"
	name := "test"
	config.SetMGHNamespacedName(types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	})
	now := metav1.Now()
	cases := []struct {
		name            string
		cluster         *v1.ManagedCluster
		managementAddon *v1alpha1.ClusterManagementAddOn
		mgh             *operatorv1alpha4.MulticlusterGlobalHub
		addon           *v1alpha1.ManagedClusterAddOn
		req             reconcile.Request
		validateFunc    func(t *testing.T, addon *v1alpha1.ManagedClusterAddOn, err error)
	}{
		{
			name:            "clustermanagementaddon not ready",
			mgh:             fakeMGH(namespace, name),
			cluster:         fakeCluster("cluster1", "", constants.GHDeployModeDefault),
			managementAddon: nil,
			req:             reconcile.Request{NamespacedName: types.NamespacedName{Name: "cluster1"}},
			validateFunc: func(t *testing.T, addon *v1alpha1.ManagedClusterAddOn, err error) {
				if !errors.IsNotFound(err) {
					t.Errorf("expected not found addon, but got err %v", err)
				}
				if addon != nil {
					t.Errorf("expected nil addon, but got %v", addon)
				}
			},
		},
		{
			name:            "req not found",
			mgh:             fakeMGH(namespace, name),
			cluster:         fakeCluster("cluster1", "", constants.GHDeployModeDefault),
			managementAddon: fakeClusterManagementAddon(),
			req:             reconcile.Request{NamespacedName: types.NamespacedName{Name: "cluster2"}},
			validateFunc: func(t *testing.T, addon *v1alpha1.ManagedClusterAddOn, err error) {
				if !errors.IsNotFound(err) {
					t.Errorf("expected not found addon, but got err %v", err)
				}
				if addon != nil {
					t.Errorf("expected nil addon, but got %v", addon)
				}
			},
		},
		{
			name:            "create addon in default mode",
			mgh:             fakeMGH(namespace, name),
			cluster:         fakeCluster("cluster1", "", constants.GHDeployModeDefault),
			managementAddon: fakeClusterManagementAddon(),
			req:             reconcile.Request{NamespacedName: types.NamespacedName{Name: "cluster1"}},
			validateFunc: func(t *testing.T, addon *v1alpha1.ManagedClusterAddOn, err error) {
				if err != nil {
					t.Errorf("failed to reconcile .%v", err)
				}
				if addon.Spec.InstallNamespace != constants.GHAgentNamespace { //nolint:staticcheck
					t.Errorf("expected install name %s, but got %s",
						operatorconstants.GHAgentInstallNamespace, addon.Spec.InstallNamespace) //nolint:staticcheck
				}
			},
		},
		{
			name: "mgh is deleting",
			mgh: &operatorv1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:              name,
					Namespace:         namespace,
					DeletionTimestamp: &now,
					Finalizers: []string{
						"test-finalizer",
					},
				},
				Status: operatorv1alpha4.MulticlusterGlobalHubStatus{
					Conditions: []metav1.Condition{
						{
							Type:   config.CONDITION_TYPE_GLOBALHUB_READY,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			cluster:         fakeCluster("cluster1", "", constants.GHDeployModeDefault),
			managementAddon: nil,
			req:             reconcile.Request{NamespacedName: types.NamespacedName{Name: "cluster1"}},
			validateFunc: func(t *testing.T, addon *v1alpha1.ManagedClusterAddOn, err error) {
				if !errors.IsNotFound(err) {
					t.Errorf("expected not found addon, but got err %v", err)
				}
				if addon != nil {
					t.Errorf("expected nil addon, but got %v", addon)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			objects := []client.Object{tc.cluster}
			if tc.managementAddon != nil {
				objects = append(objects, tc.managementAddon)
			}
			if tc.addon != nil {
				objects = append(objects, tc.addon)
			}
			if tc.mgh != nil {
				objects = append(objects, tc.mgh)
				config.SetMGHNamespacedName(types.NamespacedName{
					Namespace: tc.mgh.Namespace, Name: tc.mgh.Name,
				})
			} else {
				config.SetMGHNamespacedName(types.NamespacedName{Namespace: "", Name: ""})
			}

			objects = append(objects, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-kafka-user", tc.cluster.Name),
					Namespace: tc.mgh.Namespace,
					Labels: map[string]string{
						constants.GlobalHubOwnerLabelKey: constants.GlobalHubAddonOwnerLabelVal,
					},
				},
			})

			fakeClient := fake.NewClientBuilder().WithScheme(config.GetRuntimeScheme()).WithObjects(objects...).Build()

			transporter := operatortrans.NewBYOTransporter(ctx, types.NamespacedName{
				Namespace: tc.mgh.Namespace,
				Name:      constants.GHTransportSecretName,
			}, fakeClient)
			config.SetTransporter(transporter)

			r := &DefaultAgentController{
				Client: fakeClient,
			}

			_, err := r.Reconcile(ctx, tc.req)
			for err != nil && strings.Contains(err.Error(), "object was modified") {
				fmt.Println("error message:", err.Error())
				_, err = r.Reconcile(ctx, tc.req)
			}

			if err != nil {
				tc.validateFunc(t, nil, err)
			} else {
				addon := &v1alpha1.ManagedClusterAddOn{}
				err = r.Get(context.TODO(), types.NamespacedName{
					Namespace: tc.cluster.Name, Name: constants.GHManagedClusterAddonName,
				}, addon)
				if err != nil {
					if errors.IsNotFound(err) {
						tc.validateFunc(t, nil, err)
					} else {
						t.Errorf("failed to get addon %s", tc.cluster.Name)
					}
				} else {
					tc.validateFunc(t, addon, nil)
				}
			}
		})
	}
}

// TestConfirmDeployLabelAbsent verifies the ACM-40204 guard: the destructive prune path must only
// treat the deploy-mode label as absent when an uncached read of the API server confirms it, so a
// stale controller cache during an operator restart/upgrade cannot delete a live managed hub's
// KafkaUser.
//
//	go test -run ^TestConfirmDeployLabelAbsent$ \
//	  github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/agent/addon
func TestConfirmDeployLabelAbsent(t *testing.T) {
	clusterName := "regionalhub1"

	withLabel := &v1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:   clusterName,
			Labels: map[string]string{constants.GHDeployModeLabelKey: constants.GHDeployModeDefault},
		},
	}
	withoutLabel := &v1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: clusterName},
	}

	tests := []struct {
		name       string
		apiObjects []client.Object
		nilReader  bool
		wantAbsent bool
	}{
		{
			name:       "label present per API server -> not absent (must NOT prune)",
			apiObjects: []client.Object{withLabel},
			wantAbsent: false,
		},
		{
			name:       "label absent per API server -> absent (prune allowed)",
			apiObjects: []client.Object{withoutLabel},
			wantAbsent: true,
		},
		{
			name:       "cluster not found -> absent (prune allowed)",
			apiObjects: []client.Object{},
			wantAbsent: true,
		},
		{
			name:       "no api reader -> fall back to absent",
			nilReader:  true,
			wantAbsent: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &DefaultAgentController{}
			if !tc.nilReader {
				r.apiReader = fake.NewClientBuilder().
					WithScheme(config.GetRuntimeScheme()).
					WithObjects(tc.apiObjects...).
					Build()
			}
			absent, err := r.confirmDeployLabelAbsent(context.Background(), clusterName)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if absent != tc.wantAbsent {
				t.Fatalf("confirmDeployLabelAbsent = %v, want %v", absent, tc.wantAbsent)
			}
		})
	}
}

// TestExpectedManagedClusterAddon_RegionalHubSkipsHubRoleAnnotation verifies ACM-42804 fix: when a
// regional hub (identified by addon.open-cluster-management.io/on-multicluster-hub=true) has the
// hub-role label, expectedManagedClusterAddon must NOT copy it to the addon annotations to prevent
// triggering addon recreation.
//
//	go test -run ^TestExpectedManagedClusterAddon_RegionalHubSkipsHubRoleAnnotation$ \
//	  github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/agent/addon
func TestExpectedManagedClusterAddon_RegionalHubSkipsHubRoleAnnotation(t *testing.T) {
	tests := []struct {
		name                      string
		cluster                   *v1.ManagedCluster
		expectHubRoleInAnnotation bool
	}{
		{
			name: "regular managed cluster with hub-role label -> annotation added",
			cluster: &v1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "regular-cluster",
					Labels: map[string]string{
						constants.GHHubRoleLabelKey: constants.GHHubRoleActive,
					},
				},
			},
			expectHubRoleInAnnotation: true,
		},
		{
			name: "regional hub with hub-role label -> annotation skipped (ACM-42804)",
			cluster: &v1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "regional-hub",
					Labels: map[string]string{
						constants.GHHubRoleLabelKey: constants.GHHubRoleActive,
					},
					Annotations: map[string]string{
						constants.AnnotationONMulticlusterHub: "true",
					},
				},
			},
			expectHubRoleInAnnotation: false,
		},
		{
			name: "regional hub without hub-role label -> no annotation",
			cluster: &v1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "regional-hub-no-role",
					Annotations: map[string]string{
						constants.AnnotationONMulticlusterHub: "true",
					},
				},
			},
			expectHubRoleInAnnotation: false,
		},
		{
			name: "cluster without hub-role label -> no annotation",
			cluster: &v1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "regular-cluster-no-role",
				},
			},
			expectHubRoleInAnnotation: false,
		},
	}

	cma := fakeClusterManagementAddon()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			addon, err := expectedManagedClusterAddon(tc.cluster, cma)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			annotations := addon.GetAnnotations()
			_, hasHubRole := annotations[constants.GHHubRoleLabelKey]

			if hasHubRole != tc.expectHubRoleInAnnotation {
				t.Errorf("hub-role in addon annotations = %v, want %v (annotations: %v)",
					hasHubRole, tc.expectHubRoleInAnnotation, annotations)
			}

			// If hub-role annotation is expected, verify it matches the label value
			if tc.expectHubRoleInAnnotation {
				expectedRole := tc.cluster.Labels[constants.GHHubRoleLabelKey]
				actualRole := annotations[constants.GHHubRoleLabelKey]
				if actualRole != expectedRole {
					t.Errorf("hub-role annotation value = %q, want %q", actualRole, expectedRole)
				}
			}
		})
	}
}

// TestReconcileStaleCacheKeepsManagedHub is the ACM-40204 Reconcile regression: when the controller
// cache transiently lacks BOTH the deploy-mode label and the ManagedClusterAddOn during an operator
// restart/upgrade, but the API server still reports the labeled cluster, Reconcile must NOT take the
// destructive prune path. It must confirm the label with an uncached read and reconcile the addon
// instead of pruning the managed hub's KafkaUser.
//
//	go test -run ^TestReconcileStaleCacheKeepsManagedHub$ \
//	  github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/agent/addon
func TestReconcileStaleCacheKeepsManagedHub(t *testing.T) {
	namespace := "multicluster-global-hub"
	name := "test"
	clusterName := "regionalhub1"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config.SetMGHNamespacedName(types.NamespacedName{Namespace: namespace, Name: name})

	// Cached view during an operator upgrade: the cluster is missing the deploy-mode label and the
	// ManagedClusterAddOn is absent from the cache.
	cachedCluster := &v1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: clusterName},
	}
	cachedClient := fake.NewClientBuilder().
		WithScheme(config.GetRuntimeScheme()).
		WithObjects(fakeMGH(namespace, name), fakeClusterManagementAddon(), cachedCluster).
		Build()

	// API server truth: the cluster still carries the deploy-mode label.
	apiReader := fake.NewClientBuilder().
		WithScheme(config.GetRuntimeScheme()).
		WithObjects(fakeCluster(clusterName, "", constants.GHDeployModeDefault)).
		Build()

	config.SetTransporter(operatortrans.NewBYOTransporter(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      constants.GHTransportSecretName,
	}, cachedClient))

	r := &DefaultAgentController{
		Client:    cachedClient,
		apiReader: apiReader,
	}

	if _, err := r.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: clusterName},
	}); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	// The addon must be reconciled (created), proving the prune path was skipped despite the stale
	// cache lacking both the label and the addon.
	addon := &v1alpha1.ManagedClusterAddOn{}
	if err := cachedClient.Get(ctx, types.NamespacedName{
		Namespace: clusterName, Name: constants.GHManagedClusterAddonName,
	}, addon); err != nil {
		t.Fatalf("expected addon to be reconciled (prune path skipped), but got err: %v", err)
	}
}
