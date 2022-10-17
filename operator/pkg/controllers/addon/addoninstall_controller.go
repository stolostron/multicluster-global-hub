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
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	commonconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
)

type HoHAddonInstallReconciler struct {
	client.Client
}

func (r *HoHAddonInstallReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	log.Info("Reconciling", "namespacedname", req.NamespacedName)

	if config.GetHoHMGHNamespacedName().Namespace == "" ||
		config.GetHoHMGHNamespacedName().Name == "" {
		log.Info("waiting multiclusterglobalhub instance", req.NamespacedName)
		return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
	}

	cluster := &clusterv1.ManagedCluster{}
	clusterName := req.NamespacedName.Name
	err := r.Get(ctx, req.NamespacedName, cluster)
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
			Name:      constants.HoHManagedClusterAddonName,
			Namespace: clusterName,
		},
		Spec: v1alpha1.ManagedClusterAddOnSpec{
			InstallNamespace: constants.HoHAgentInstallNamespace,
		},
	}

	labels := cluster.GetLabels()
	switch labels[commonconstants.AgentDeployModeLabelKey] {
	case commonconstants.AgentDeployModeHosted:
		annotations := cluster.GetAnnotations()
		if hostingCluster, _ := annotations[constants.AnnotationClusterHostingClusterName]; hostingCluster != "" {
			addon.SetAnnotations(map[string]string{
				constants.AnnotationAddonHostingClusterName: hostingCluster,
			})
			addon.Spec.InstallNamespace = fmt.Sprintf(
				"open-cluster-management-%s-hoh-addon", clusterName)
		} else {
			return ctrl.Result{}, fmt.Errorf("failed to get hosting cluster name "+
				"when addon in %s is installed in hosted mode", clusterName)
		}
	case commonconstants.AgentDeployModeNone:
		return ctrl.Result{}, nil
	}

	existingAddon := &v1alpha1.ManagedClusterAddOn{}
	err = r.Get(ctx, types.NamespacedName{
		Namespace: clusterName,
		Name:      constants.HoHManagedClusterAddonName,
	}, existingAddon)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, r.Create(ctx, addon)
		}
		return ctrl.Result{}, err
	}

	if existingAddon.GetAnnotations()[constants.AnnotationAddonHostingClusterName] !=
		addon.GetAnnotations()[constants.AnnotationAddonHostingClusterName] ||
		existingAddon.Spec.InstallNamespace != addon.Spec.InstallNamespace {
		existingAddon.SetAnnotations(addon.Annotations)
		existingAddon.Spec.InstallNamespace = addon.Spec.InstallNamespace
		return ctrl.Result{}, r.Update(ctx, existingAddon)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HoHAddonInstallReconciler) SetupWithManager(mgr ctrl.Manager) error {
	clusterPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetLabels()["vendor"] != "OpenShift" ||
				e.Object.GetName() == constants.LocalClusterName ||
				!meta.IsStatusConditionTrue(e.Object.(*clusterv1.ManagedCluster).Status.Conditions,
					"ManagedClusterConditionAvailable") {
				return false
			}

			if e.Object.GetLabels()[commonconstants.AgentDeployModeLabelKey] == commonconstants.AgentDeployModeNone {
				return false
			}
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetLabels()["vendor"] != "OpenShift" ||
				e.ObjectNew.GetName() == constants.LocalClusterName ||
				!meta.IsStatusConditionTrue(e.ObjectNew.(*clusterv1.ManagedCluster).Status.Conditions,
					"ManagedClusterConditionAvailable") {
				return false
			}

			if e.ObjectNew.GetLabels()[commonconstants.AgentDeployModeLabelKey] == commonconstants.AgentDeployModeNone {
				return false
			}
			if e.ObjectNew.GetResourceVersion() == e.ObjectOld.GetResourceVersion() {
				return false
			}

			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetLabels()["vendor"] != "OpenShift" ||
				e.Object.GetName() == constants.LocalClusterName ||
				!meta.IsStatusConditionTrue(e.Object.(*clusterv1.ManagedCluster).Status.Conditions,
					"ManagedClusterConditionAvailable") {
				return false
			}

			if e.Object.GetLabels()[commonconstants.AgentDeployModeLabelKey] == commonconstants.AgentDeployModeNone {
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
			if e.ObjectNew.GetName() != constants.HoHManagedClusterAddonName {
				return false
			}
			if e.ObjectNew.GetGeneration() == e.ObjectOld.GetGeneration() {
				return false
			}
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetName() != constants.HoHManagedClusterAddonName {
				return false
			}
			return true
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

func NewHoHAddonInstallReconciler(client client.Client) *HoHAddonInstallReconciler {
	return &HoHAddonInstallReconciler{Client: client}
}
