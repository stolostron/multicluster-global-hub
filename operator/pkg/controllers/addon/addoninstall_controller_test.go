package addon

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"open-cluster-management.io/api/addon/v1alpha1"
	v1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	globalconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func fakeCluster(name, hostingCluster, addonDeployMode string) *v1.ManagedCluster {
	cluster := &v1.ManagedCluster{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.ManagedClusterSpec{},
	}
	labels := map[string]string{
		globalconstants.AgentDeployModeLabelKey: addonDeployMode,
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

func fakeHoHAddon(cluster, installNamespace, addonDeployMode string) *v1alpha1.ManagedClusterAddOn {
	addon := &v1alpha1.ManagedClusterAddOn{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.HoHManagedClusterAddonName,
			Namespace: cluster,
		},
		Spec: v1alpha1.ManagedClusterAddOnSpec{
			InstallNamespace: installNamespace,
		},
	}

	if addonDeployMode == constants.ClusterDeployModeHosted {
		addon.SetAnnotations(map[string]string{constants.AnnotationAddonHostingClusterName: "hostingcluster"})
	}

	return addon
}

func TestHoHAddonReconciler(t *testing.T) {
	addonTestScheme := scheme.Scheme
	utilruntime.Must(v1.AddToScheme(addonTestScheme))
	utilruntime.Must(v1alpha1.AddToScheme(addonTestScheme))
	config.SetHoHMGHNamespacedName(types.NamespacedName{Namespace: "open-cluster-management", Name: "multiclusterglobalhub"})

	tests := []struct {
		name         string
		cluster      *v1.ManagedCluster
		addon        *v1alpha1.ManagedClusterAddOn
		req          reconcile.Request
		validateFunc func(t *testing.T, addon *v1alpha1.ManagedClusterAddOn, err error)
	}{
		{
			name:    "req not found",
			cluster: fakeCluster("cluster1", "", globalconstants.AgentDeployModeDefault),
			req:     reconcile.Request{NamespacedName: types.NamespacedName{Name: "cluster2"}},
			validateFunc: func(t *testing.T, addon *v1alpha1.ManagedClusterAddOn, err error) {
				if !errors.IsNotFound(err) {
					t.Errorf("expected not found addon ,but got err %v", err)
				}
				if addon != nil {
					t.Errorf("expected nil addon, but got %v", addon)
				}
			},
		},
		{
			name:    "do not create addon",
			cluster: fakeCluster("cluster1", "", globalconstants.AgentDeployModeNone),
			req:     reconcile.Request{NamespacedName: types.NamespacedName{Name: "cluster1"}},
			validateFunc: func(t *testing.T, addon *v1alpha1.ManagedClusterAddOn, err error) {
				if !errors.IsNotFound(err) {
					t.Errorf("expected not found addon ,but got err %v", err)
				}
				if addon != nil {
					t.Errorf("expected nil addon, but got %v", addon)
				}
			},
		},
		{
			name:    "create addon in default mode",
			cluster: fakeCluster("cluster1", "", globalconstants.AgentDeployModeDefault),
			req:     reconcile.Request{NamespacedName: types.NamespacedName{Name: "cluster1"}},
			validateFunc: func(t *testing.T, addon *v1alpha1.ManagedClusterAddOn, err error) {
				if err != nil {
					t.Errorf("failed to reconcile .%v", err)
				}
				if addon.Spec.InstallNamespace != constants.HoHAgentInstallNamespace {
					t.Errorf("expected installname %s, but got %s",
						constants.HoHAgentInstallNamespace, addon.Spec.InstallNamespace)
				}
			},
		},
		{
			name:    "create addon in hosted mode",
			cluster: fakeCluster("cluster1", "cluster2", globalconstants.AgentDeployModeHosted),
			req:     reconcile.Request{NamespacedName: types.NamespacedName{Name: "cluster1"}},
			validateFunc: func(t *testing.T, addon *v1alpha1.ManagedClusterAddOn, err error) {
				if err != nil {
					t.Errorf("failed to reconcile .%v", err)
				}
				if addon.Spec.InstallNamespace != "open-cluster-management-cluster1-hoh-addon" {
					t.Errorf("expected installname open-cluster-management-cluster1-hoh-addon, but got %s", addon.Spec.InstallNamespace)
				}
				if addon.Annotations[constants.AnnotationAddonHostingClusterName] != "cluster2" {
					t.Errorf("expected hosting cluster cluster2, but got %s",
						addon.Annotations[constants.AnnotationAddonHostingClusterName])
				}
			},
		},
		{
			name:    "update addon in hosted mode",
			cluster: fakeCluster("cluster1", "cluster2", globalconstants.AgentDeployModeHosted),
			addon:   fakeHoHAddon("cluster1", "test", ""),
			req:     reconcile.Request{NamespacedName: types.NamespacedName{Name: "cluster1"}},
			validateFunc: func(t *testing.T, addon *v1alpha1.ManagedClusterAddOn, err error) {
				if err != nil {
					t.Errorf("failed to reconcile .%v", err)
				}
				if addon.Spec.InstallNamespace != "open-cluster-management-cluster1-hoh-addon" {
					t.Errorf("expected installname open-cluster-management-cluster1-hoh-addon, but got %s", addon.Spec.InstallNamespace)
				}
				if addon.Annotations[constants.AnnotationAddonHostingClusterName] != "cluster2" {
					t.Errorf("expected hosting cluster cluster2, but got %s",
						addon.Annotations[constants.AnnotationAddonHostingClusterName])
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			objects := []client.Object{test.cluster}
			if test.addon != nil {
				objects = append(objects, test.addon)
			}
			r := NewHoHAddonInstallReconciler(fake.NewClientBuilder().WithScheme(
				addonTestScheme).WithObjects(objects...).Build())
			mgr, _ := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{})
			_ = r.SetupWithManager(mgr)

			_, err := r.Reconcile(context.TODO(), test.req)
			if err != nil {
				test.validateFunc(t, nil, err)
			} else {
				addon := &v1alpha1.ManagedClusterAddOn{}
				err = r.Get(context.TODO(), types.NamespacedName{
					Namespace: test.cluster.Name, Name: constants.HoHManagedClusterAddonName,
				}, addon)
				if err != nil {
					if errors.IsNotFound(err) {
						test.validateFunc(t, nil, err)
					} else {
						t.Errorf("failed to get addon %s", test.cluster.Name)
					}
				} else {
					test.validateFunc(t, addon, nil)
				}
			}
		})
	}
}
