package addon

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	imageregistryv1alpha1 "github.com/stolostron/cluster-lifecycle-api/imageregistry/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	operatortrans "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/transporter/protocol"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

type AddonInstaller struct {
	client.Client
	Log logr.Logger
}

func (r *AddonInstaller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.Client)
	if err != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	if mgh.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	if config.IsPaused(mgh) {
		r.Log.Info("multiclusterglobalhub addon installer is paused, nothing more to do")
		return ctrl.Result{}, nil
	}

	err = utils.WaitTransporterReady(ctx, 10*time.Minute)
	if err != nil {
		return ctrl.Result{}, err
	}

	clusterManagementAddOn := &v1alpha1.ClusterManagementAddOn{}
	err = r.Get(ctx, types.NamespacedName{
		Name: operatorconstants.GHClusterManagementAddonName,
	}, clusterManagementAddOn)
	if err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("waiting until clustermanagementaddon is created", "namespacedname", req.NamespacedName)
			return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
		} else {
			return ctrl.Result{}, err
		}
	}
	if !clusterManagementAddOn.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.NamespacedName.Name,
		},
	}
	err = r.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	deployMode := cluster.GetLabels()[operatorconstants.GHAgentDeployModeLabelKey]
	// delete the resources
	if !cluster.DeletionTimestamp.IsZero() ||
		deployMode == operatorconstants.GHAgentDeployModeNone {
		r.Log.Info("deleting resourcs and addon", "cluster", cluster.Name, "deployMode", deployMode)
		if err := r.removeResourcesAndAddon(ctx, cluster); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove resources and addon %s: %v", cluster.Name, err)
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, r.reconclieAddonAndResources(ctx, cluster)
}

func (r *AddonInstaller) reconclieAddonAndResources(ctx context.Context, cluster *clusterv1.ManagedCluster) error {
	expectedAddon, err := expectedManagedClusterAddon(cluster)
	if err != nil {
		return err
	}

	existingAddon := &v1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconstants.GHManagedClusterAddonName,
			Namespace: cluster.Name,
		},
	}
	err = r.Get(ctx, client.ObjectKeyFromObject(existingAddon), existingAddon)
	// create is not found, update if err == nil
	if err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("creating resourcs and addon", "cluster", cluster.Name, "addon", existingAddon.Name)
			if e := r.Create(ctx, expectedAddon); e != nil {
				return e
			}
		} else {
			return fmt.Errorf("failed to get the addon: %v", err)
		}
	} else {
		// delete
		if !existingAddon.DeletionTimestamp.IsZero() {
			r.Log.Info("deleting resourcs and addon", "cluster", cluster.Name, "addon", existingAddon.Name)
			return r.removeResourcesAndAddon(ctx, cluster)
		}

		// update
		if !reflect.DeepEqual(expectedAddon.Annotations, existingAddon.Annotations) ||
			existingAddon.Spec.InstallNamespace != expectedAddon.Spec.InstallNamespace {
			existingAddon.SetAnnotations(expectedAddon.Annotations)
			existingAddon.Spec.InstallNamespace = expectedAddon.Spec.InstallNamespace
			r.Log.Info("updating addon", "cluster", cluster.Name, "addon", expectedAddon.Name)
			if e := r.Update(ctx, existingAddon); e != nil {
				return e
			}
		}
	}

	// reconcile transport resources
	return ensureTransportResource(cluster.Name)
}

func ensureTransportResource(clusterName string) error {
	// create kafka resource: user and topic
	trans := config.GetTransporter()
	if trans == nil {
		return fmt.Errorf("failed to get the transporter")
	}
	if _, err := trans.EnsureUser(clusterName); err != nil {
		return err
	}
	if _, err := trans.EnsureTopic(clusterName); err != nil {
		return err
	}
	return nil
}

func (r *AddonInstaller) removeResourcesAndAddon(ctx context.Context, cluster *clusterv1.ManagedCluster) error {
	// should remove the addon first, otherwise it mightn't update the mainfiest work for the addon
	existingAddon := &v1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconstants.GHManagedClusterAddonName,
			Namespace: cluster.Name,
		},
	}
	err := r.Get(ctx, client.ObjectKeyFromObject(existingAddon), existingAddon)
	if err == nil {
		if e := r.Delete(ctx, existingAddon); e != nil {
			return fmt.Errorf("failed to delete the managedclusteraddon %v", e)
		}
	}
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed go get the managedclusteraddon %v", err)
	}

	// clean kafka resource: user and topic
	trans := config.GetTransporter()
	if trans == nil {
		return fmt.Errorf("failed to get the transporter")
	}

	return trans.Prune(cluster.Name)
}

func expectedManagedClusterAddon(cluster *clusterv1.ManagedCluster) (*v1alpha1.ManagedClusterAddOn, error) {
	expectedAddon := &v1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconstants.GHManagedClusterAddonName,
			Namespace: cluster.Name,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
		},
		Spec: v1alpha1.ManagedClusterAddOnSpec{
			InstallNamespace: constants.GHAgentNamespace,
		},
	}
	expectedAddonAnnotations := map[string]string{}

	deployMode := cluster.GetLabels()[operatorconstants.GHAgentDeployModeLabelKey]
	if deployMode == operatorconstants.GHAgentDeployModeHosted {
		annotations := cluster.GetAnnotations()
		if hostingCluster := annotations[operatorconstants.AnnotationClusterHostingClusterName]; hostingCluster != "" {
			expectedAddonAnnotations[operatorconstants.AnnotationAddonHostingClusterName] = hostingCluster
			expectedAddon.Spec.InstallNamespace = fmt.Sprintf("klusterlet-%s", cluster.Name)
		} else {
			return nil, fmt.Errorf("failed to get %s when addon in %s is installed in hosted mode",
				operatorconstants.AnnotationClusterHostingClusterName, cluster.Name)
		}
	}

	if val, ok := cluster.Annotations[imageregistryv1alpha1.ClusterImageRegistriesAnnotation]; ok {
		expectedAddonAnnotations[imageregistryv1alpha1.ClusterImageRegistriesAnnotation] = val
	}
	if len(expectedAddonAnnotations) > 0 {
		expectedAddon.SetAnnotations(expectedAddonAnnotations)
	}
	return expectedAddon, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AddonInstaller) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	clusterPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return !filterManagedCluster(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if filterManagedCluster(e.ObjectNew) {
				return false
			}
			if e.ObjectNew.GetResourceVersion() == e.ObjectOld.GetResourceVersion() {
				return false
			}
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return !filterManagedCluster(e.Object)
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

	clusterManagementAddonPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return e.Object.GetName() == operatorconstants.GHManagedClusterAddonName
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

	secretCond := func(obj client.Object) bool {
		if obj.GetName() == config.GetImagePullSecretName() ||
			obj.GetName() == constants.GHTransportSecretName ||
			obj.GetLabels() != nil && obj.GetLabels()["strimzi.io/cluster"] == operatortrans.KafkaClusterName &&
				obj.GetLabels()["strimzi.io/kind"] == "KafkaUser" {
			return true
		}
		return false
	}
	secretPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return secretCond(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return secretCond(e.ObjectNew)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}

	// TODO: investgate why the dynamic cache cannot work
	// acmCache, err := config.ACMCache(mgr)
	// if err != nil {
	// 	return err
	// }

	return ctrl.NewControllerManagedBy(mgr).
		Named("addonInstaller").
		// primary watch for managedcluster
		Watches(&clusterv1.ManagedCluster{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(clusterPred)).
		// WatchesRawSource(source.Kind(acmCache, &clusterv1.ManagedCluster{}),
		// 	&handler.EnqueueRequestForObject{}, builder.WithPredicates(clusterPred)).
		// secondary watch for managedclusteraddon
		Watches(&v1alpha1.ManagedClusterAddOn{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					// only trigger the addon reconcile when addon is updated/deleted
					{NamespacedName: types.NamespacedName{
						Name: obj.GetNamespace(),
					}},
				}
			}), builder.WithPredicates(addonPred)).
		// secondary watch for managedclusteraddon
		Watches(&v1alpha1.ClusterManagementAddOn{},
			handler.EnqueueRequestsFromMapFunc(r.renderAllManifestsHandler),
			builder.WithPredicates(clusterManagementAddonPred)).
		// secondary watch for transport credentials or image pull secret
		Watches(&corev1.Secret{}, // the cache is set in manager
			handler.EnqueueRequestsFromMapFunc(r.renderAllManifestsHandler),
			builder.WithPredicates(secretPred)).
		Complete(r)
}

func (r *AddonInstaller) renderAllManifestsHandler(
	ctx context.Context, obj client.Object,
) []reconcile.Request {
	requests := []reconcile.Request{}

	hubNames, err := GetAllManagedHubNames(ctx, r.Client)
	if err != nil {
		r.Log.Error(err, "failed to list managed clusters to trigger addoninstall reconciler")
		return requests
	}
	for _, name := range hubNames {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: name,
			},
		})
	}
	r.Log.Info("triggers addoninstall reconciler for all managed clusters", "requests", len(requests))
	return requests
}

func GetAllManagedHubNames(ctx context.Context, c client.Client) ([]string, error) {
	names := []string{}
	managedClusterList := &clusterv1.ManagedClusterList{}
	err := c.List(ctx, managedClusterList)
	if err != nil {
		return nil, err
	}

	for i := range managedClusterList.Items {
		managedCluster := managedClusterList.Items[i]
		if filterManagedCluster(&managedCluster) {
			continue
		}
		names = append(names, managedCluster.GetName())
	}
	return names, nil
}

func filterManagedCluster(obj client.Object) bool {
	return obj.GetLabels()["vendor"] != "OpenShift" ||
		obj.GetLabels()["openshiftVersion"] == "3" ||
		obj.GetName() == operatorconstants.LocalClusterName
}
