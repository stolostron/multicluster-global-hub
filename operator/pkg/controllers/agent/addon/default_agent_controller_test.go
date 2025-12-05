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
				if addon.Spec.InstallNamespace != constants.GHAgentNamespace {
					t.Errorf("expected install name %s, but got %s",
						operatorconstants.GHAgentInstallNamespace, addon.Spec.InstallNamespace)
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

			transporter := operatortrans.EnsureBYOTransport(ctx, types.NamespacedName{
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
