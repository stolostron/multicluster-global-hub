package addon

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

type HoHAddonInstallReconciler struct {
	client.Client
}

func (r *HoHAddonInstallReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	log.Info("Reconciling", "namespacedname", req.NamespacedName)

	if config.GetHoHMGHNamespacedName().Namespace == "" ||
		config.GetHoHMGHNamespacedName().Name == "" {
		log.Info("waiting multiclusterglobalhub instance", "namespacedname", req.NamespacedName)
		return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
	}

	clusterManagementAddOn := &v1alpha1.ClusterManagementAddOn{}
	err := r.Get(ctx, types.NamespacedName{
		Name: operatorconstants.GHClusterManagementAddonName,
	}, clusterManagementAddOn)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("waiting util clustermanagementaddon is created", "namespacedname", req.NamespacedName)
			return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
		} else {
			return ctrl.Result{}, err
		}
	}

	cluster := &clusterv1.ManagedCluster{}
	clusterName := req.NamespacedName.Name
	err = r.Get(ctx, req.NamespacedName, cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get managedcluster")
		return ctrl.Result{}, err
	}

	if !cluster.DeletionTimestamp.IsZero() {
		log.V(4).Info("Cluster is deleting, skip addon deploy", clusterName)
		return ctrl.Result{}, nil
	}

	addon := &v1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconstants.GHManagedClusterAddonName,
			Namespace: clusterName,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
		},
		Spec: v1alpha1.ManagedClusterAddOnSpec{
			InstallNamespace: operatorconstants.GHAgentInstallNamespace,
		},
	}

	// install addon in open-cluster-management-global-hub-system ns if the cluster has local-cluster.
	switch getHub(cluster) {
	case constants.HubInstalledWithSelfManagement:
		addon.Spec.InstallNamespace = constants.GHSystemNamespace
	}

	labels := cluster.GetLabels()
	switch labels[operatorconstants.GHAgentDeployModeLabelKey] {
	case operatorconstants.GHAgentDeployModeHosted:
		annotations := cluster.GetAnnotations()
		if hostingCluster := annotations[operatorconstants.AnnotationClusterHostingClusterName]; hostingCluster != "" {
			addon.SetAnnotations(map[string]string{
				operatorconstants.AnnotationAddonHostingClusterName: hostingCluster,
			})
			addon.Spec.InstallNamespace = fmt.Sprintf(
				"open-cluster-management-%s-hoh-addon", clusterName)
		} else {
			return ctrl.Result{}, fmt.Errorf("failed to get hosting cluster name "+
				"when addon in %s is installed in hosted mode", clusterName)
		}
	case operatorconstants.GHAgentDeployModeNone:
		return ctrl.Result{}, nil
	}

	existingAddon := &v1alpha1.ManagedClusterAddOn{}
	err = r.Get(ctx, types.NamespacedName{
		Namespace: clusterName,
		Name:      operatorconstants.GHManagedClusterAddonName,
	}, existingAddon)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, r.Create(ctx, addon)
		}
		return ctrl.Result{}, err
	}

	if existingAddon.GetAnnotations()[operatorconstants.AnnotationAddonHostingClusterName] !=
		addon.GetAnnotations()[operatorconstants.AnnotationAddonHostingClusterName] ||
		existingAddon.Spec.InstallNamespace != addon.Spec.InstallNamespace {
		existingAddon.SetAnnotations(addon.Annotations)
		existingAddon.Spec.InstallNamespace = addon.Spec.InstallNamespace
		return ctrl.Result{}, r.Update(ctx, existingAddon)
	}

	return ctrl.Result{}, nil
}

func getHub(cluster *clusterv1.ManagedCluster) string {
	for _, claim := range cluster.Status.ClusterClaims {
		if claim.Name != constants.HubClusterClaimName {
			continue
		}

		return claim.Value
	}
	return ""
}

// SetupWithManager sets up the controller with the Manager.
func (r *HoHAddonInstallReconciler) SetupWithManager(mgr ctrl.Manager) error {
	clusterPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetLabels()["vendor"] != "OpenShift" ||
				e.Object.GetName() == operatorconstants.LocalClusterName ||
				!meta.IsStatusConditionTrue(e.Object.(*clusterv1.ManagedCluster).Status.Conditions,
					"ManagedClusterConditionAvailable") {
				return false
			}

			if e.Object.GetLabels()[operatorconstants.GHAgentDeployModeLabelKey] ==
				operatorconstants.GHAgentDeployModeNone {
				return false
			}
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetLabels()["vendor"] != "OpenShift" ||
				e.ObjectNew.GetName() == operatorconstants.LocalClusterName ||
				!meta.IsStatusConditionTrue(e.ObjectNew.(*clusterv1.ManagedCluster).Status.Conditions,
					"ManagedClusterConditionAvailable") {
				return false
			}

			if e.ObjectNew.GetLabels()[operatorconstants.GHAgentDeployModeLabelKey] ==
				operatorconstants.GHAgentDeployModeNone {
				return false
			}
			if e.ObjectNew.GetResourceVersion() == e.ObjectOld.GetResourceVersion() {
				return false
			}

			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetLabels()["vendor"] != "OpenShift" ||
				e.Object.GetName() == operatorconstants.LocalClusterName ||
				!meta.IsStatusConditionTrue(e.Object.(*clusterv1.ManagedCluster).Status.Conditions,
					"ManagedClusterConditionAvailable") {
				return false
			}

			if e.Object.GetLabels()[operatorconstants.GHAgentDeployModeLabelKey] ==
				operatorconstants.GHAgentDeployModeNone {
				return false
			}
			return true
		},
	}

	addonPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() != operatorconstants.GHManagedClusterAddonName {
				return false
			}
			if e.ObjectNew.GetGeneration() == e.ObjectOld.GetGeneration() {
				return false
			}
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return e.Object.GetName() == operatorconstants.GHManagedClusterAddonName
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		// primary watch for managedcluster
		For(&clusterv1.ManagedCluster{}, builder.WithPredicates(clusterPred)).
		// secondary watch for managedclusteraddon
		Watches(&source.Kind{Type: &v1alpha1.ManagedClusterAddOn{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					// only trigger the addon reconcile when addon is updated/deleted
					{NamespacedName: types.NamespacedName{
						Name: obj.GetNamespace(),
					}},
				}
			}), builder.WithPredicates(addonPred)).
		Complete(r)
}
