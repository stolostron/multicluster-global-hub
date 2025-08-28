package addon

import (
	"context"
	"fmt"
	"reflect"
	"time"

	imageregistryv1alpha1 "github.com/stolostron/cluster-lifecycle-api/imageregistry/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatortrans "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter/protocol"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs,verbs=get;list;watch;
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclusters,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=managedclusteraddons,verbs=create;update;get;list;watch;delete;deletecollection;patch
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=managedclusteraddons/finalizers,verbs=update
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=managedclusteraddons/status,verbs=update;patch

var (
	log                    = logger.DefaultZapLogger()
	defaultAgentController *DefaultAgentController
)

type DefaultAgentController struct {
	client.Client
}

// Assert whether a secret is associated with or affects the addon
func targetAddonSecret(obj client.Object) bool {
	if obj.GetName() == config.GetImagePullSecretName() ||
		obj.GetName() == constants.GHTransportSecretName ||
		obj.GetLabels() != nil && obj.GetLabels()["strimzi.io/cluster"] == operatortrans.KafkaClusterName &&
			obj.GetLabels()["strimzi.io/kind"] == "KafkaUser" {
		return true
	}
	return false
}

func NewDefaultAgentController(c client.Client) *DefaultAgentController {
	return &DefaultAgentController{
		Client: c,
	}
}

// StartDefaultAgentController manages addon-related resources for each cluster.
// It watches ManagedCluster resources to create or modify addons accordingly.
// It also watches the MulticlusterGlobalHub (MGH) resource to trigger cleanup
// of these addon resources when the MGH is deleted.
func StartDefaultAgentController(initOption config.ControllerOption) (config.ControllerInterface, error) {
	if defaultAgentController != nil {
		return defaultAgentController, nil
	}
	log.Info("start default agent controller")

	if !ReadyToEnableAddonManager(initOption.MulticlusterGlobalHub) {
		return nil, nil
	}
	defaultAgentController = NewDefaultAgentController(initOption.Manager.GetClient())

	err := ctrl.NewControllerManagedBy(initOption.Manager).
		Named("default-agent-ctrl").
		// TODO: Should move the mgh into secondary watch for MGH, also only trigger it when mgh is deleting
		// Why watch MGH? to trigger cleanup of managed cluster addons when MGH is deleted,
		// so there's no need to requeue all clusters.
		For(&v1alpha4.MulticlusterGlobalHub{},
			builder.WithPredicates(config.MGHPred)).
		Watches(&clusterv1.ManagedCluster{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(predicate.Funcs{
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
			})).
		// secondary watch for managedclusteraddon
		// only trigger the addon reconcile when addon is updated/deleted
		Watches(&addonv1alpha1.ManagedClusterAddOn{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					{NamespacedName: types.NamespacedName{
						Name: obj.GetNamespace(),
					}},
				}
			}),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return false
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					if e.ObjectNew.GetName() != constants.GHManagedClusterAddonName {
						return false
					}
					if e.ObjectNew.GetGeneration() == e.ObjectOld.GetGeneration() {
						return false
					}
					return true
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return e.Object.GetName() == constants.GHManagedClusterAddonName
				},
			})).
		// secondary watch for managedclusteraddon
		Watches(&addonv1alpha1.ClusterManagementAddOn{},
			handler.EnqueueRequestsFromMapFunc(defaultAgentController.allClusterRequests),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return e.Object.GetName() == constants.GHManagedClusterAddonName
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					if e.ObjectNew.GetName() != constants.GHManagedClusterAddonName {
						return false
					}
					if e.ObjectNew.GetGeneration() == e.ObjectOld.GetGeneration() {
						return false
					}
					return true
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return e.Object.GetName() == constants.GHManagedClusterAddonName
				},
			})).
		// secondary watch for transport credentials or image pull secret
		Watches(&corev1.Secret{}, // the cache is set in manager
			handler.EnqueueRequestsFromMapFunc(defaultAgentController.allClusterRequests),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return targetAddonSecret(e.Object)
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					return targetAddonSecret(e.ObjectNew)
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return false
				},
			})).
		Complete(defaultAgentController)
	if err != nil {
		defaultAgentController = nil
		return nil, err
	}
	log.Info("the default agent controller is started")
	return defaultAgentController, nil
}

func (c *DefaultAgentController) IsResourceRemoved() bool {
	log.Infof("default agent resource removed: %v", config.GetGlobalhubAgentRemoved())
	return config.GetGlobalhubAgentRemoved()
}

func (r *DefaultAgentController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Debugf("reconcile managedClusterAddon and related resources: %v", req)

	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.Client)
	if err != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	if mgh == nil || config.IsPaused(mgh) {
		return ctrl.Result{}, nil
	}

	if mgh.DeletionTimestamp != nil {
		// delete ClusterManagementAddon firstly to trigger clean up addons.
		if err := r.deleteClusterManagementAddon(ctx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete ClusterManagementAddon: %w", err)
		}
		log.Infow("deleted ClusterManagementAddon and related resources", "name", constants.GHClusterManagementAddonName)

		addonList := &addonv1alpha1.ManagedClusterAddOnList{}
		listOptions := []client.ListOption{
			client.MatchingLabels(map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			}),
		}

		if err := r.List(ctx, addonList, listOptions...); err != nil {
			return ctrl.Result{}, err
		}

		if len(addonList.Items) != 0 {
			log.Infow("waiting for ManagedClusterAddon and resources to be deleted", "addon size", len(addonList.Items))
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		log.Info("all ManagedClusterAddons are deleted")
		config.SetGlobalhubAgentRemoved(true)
		return ctrl.Result{}, nil
	}

	config.SetGlobalhubAgentRemoved(false)
	if config.GetTransporter() == nil {
		log.Debug("wait transporter ready")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	clusterManagementAddOn := &addonv1alpha1.ClusterManagementAddOn{}
	err = r.Get(ctx, types.NamespacedName{
		Name: constants.GHClusterManagementAddonName,
	}, clusterManagementAddOn)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Infow("waiting until ClusterManagementAddon is created", "namespacedname", req.NamespacedName)
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
			Name: req.Name,
		},
	}
	err = r.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Infof("not found the cluster %s, the controller might triggered by multiclusterglboalhub", mgh.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// skip the local cluster
	if cluster.Labels[constants.LocalClusterName] == "true" {
		return ctrl.Result{}, nil
	}

	// Avoid installing the global hub agent in the following scenarios:
	//   1. The cluster is being detached.
	//   2. The deploy mode label is not added
	_, hasDeployLabel := cluster.GetLabels()[constants.GHDeployModeLabelKey]
	isDetaching := !cluster.DeletionTimestamp.IsZero()

	log.Infof("cluster(%s): isDetaching - %v, hasDeployLabel - %v", cluster.Name, isDetaching, hasDeployLabel)
	if isDetaching || !hasDeployLabel {
		log.Infow("deleting resources and addon", "cluster", cluster.Name)
		if err := r.removeResourcesAndAddon(ctx, cluster); err != nil {
			log.Error(err, "failed to delete resources and addon", "cluster", cluster.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, r.reconcileAddonAndResources(ctx, cluster, clusterManagementAddOn)
}

func (r *DefaultAgentController) deleteClusterManagementAddon(ctx context.Context) error {
	clusterManagementAddOn := &addonv1alpha1.ClusterManagementAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.GHClusterManagementAddonName,
		},
	}
	if err := r.Delete(ctx, clusterManagementAddOn); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

func (r *DefaultAgentController) reconcileAddonAndResources(ctx context.Context,
	cluster *clusterv1.ManagedCluster, cma *addonv1alpha1.ClusterManagementAddOn,
) error {
	expectedAddon, err := expectedManagedClusterAddon(cluster, cma)
	if err != nil {
		return err
	}

	existingAddon := &addonv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.GHManagedClusterAddonName,
			Namespace: cluster.Name,
		},
	}
	err = r.Get(ctx, client.ObjectKeyFromObject(existingAddon), existingAddon)
	// create is not found, update if err == nil
	if err != nil {
		if errors.IsNotFound(err) {
			log.Infow("creating resources and addon", "cluster", cluster.Name, "addon", existingAddon.Name)
			if e := r.Create(ctx, expectedAddon); e != nil {
				return e
			}
		} else {
			return fmt.Errorf("failed to get the addon: %v", err)
		}
	} else {
		// delete
		if !existingAddon.DeletionTimestamp.IsZero() {
			log.Infow("deleting resources and addon", "cluster", cluster.Name, "addon", existingAddon.Name)
			return r.removeResourcesAndAddon(ctx, cluster)
		}

		// update
		if !reflect.DeepEqual(expectedAddon.Annotations, existingAddon.Annotations) ||
			existingAddon.Spec.InstallNamespace != expectedAddon.Spec.InstallNamespace {
			existingAddon.SetAnnotations(expectedAddon.Annotations)
			existingAddon.Spec.InstallNamespace = expectedAddon.Spec.InstallNamespace
			log.Infow("updating addon", "cluster", cluster.Name, "addon", expectedAddon.Name)
			if e := r.Update(ctx, existingAddon); e != nil {
				return e
			}
		}
	}

	// reconcile transport resources
	return EnsureTransportResource(cluster.Name)
}

func EnsureTransportResource(clusterName string) error {
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

func (r *DefaultAgentController) removeResourcesAndAddon(ctx context.Context, cluster *clusterv1.ManagedCluster) error {
	// should remove the addon first, otherwise it mightn't update the mainfiest work for the addon
	existingAddon := &addonv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.GHManagedClusterAddonName,
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

func expectedManagedClusterAddon(cluster *clusterv1.ManagedCluster, cma *addonv1alpha1.ClusterManagementAddOn) (
	*addonv1alpha1.ManagedClusterAddOn, error,
) {
	expectedAddon := &addonv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.GHManagedClusterAddonName,
			Namespace: cluster.Name,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
			// The OwnerReferences will be added automatically by the addon-manager(OCM) in the production evnvironment.
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "addon.open-cluster-management.io/v1alpha1",
					Kind:       "ClusterManagementAddOn",
					Name:       cma.Name,
					UID:        cma.GetUID(),
				},
			},
		},
		Spec: addonv1alpha1.ManagedClusterAddOnSpec{
			InstallNamespace: constants.GHAgentNamespace,
		},
	}
	expectedAddonAnnotations := map[string]string{}

	if val, ok := cluster.Annotations[imageregistryv1alpha1.ClusterImageRegistriesAnnotation]; ok {
		expectedAddonAnnotations[imageregistryv1alpha1.ClusterImageRegistriesAnnotation] = val
	}
	if len(expectedAddonAnnotations) > 0 {
		expectedAddon.SetAnnotations(expectedAddonAnnotations)
	}
	return expectedAddon, nil
}

func (r *DefaultAgentController) allClusterRequests(
	ctx context.Context, obj client.Object,
) []reconcile.Request {
	requests := []reconcile.Request{}

	managedClusterList := &clusterv1.ManagedClusterList{}
	err := r.List(ctx, managedClusterList)
	if err != nil {
		log.Errorf("failed to list managedClusters to trigger mca controller")
		return requests
	}

	for i := range managedClusterList.Items {
		managedCluster := managedClusterList.Items[i]
		if filterManagedCluster(&managedCluster) {
			continue
		}
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: managedCluster.GetName(),
			},
		})
	}

	log.Info("triggering all the addons/clusters: %d", len(requests))
	return requests
}

func filterManagedCluster(obj client.Object) bool {
	return obj.GetLabels()["vendor"] != "OpenShift" ||
		obj.GetLabels()["openshiftVersion"] == "3"
}
