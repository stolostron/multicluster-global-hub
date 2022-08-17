/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package leafhub

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	hypershiftdeploymentv1alpha1 "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	operatorv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/apis/operator/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	commonconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const failedConditionMsg = "failed to set condition(%s): %w"

// hubClusters defines internal map that stores hub clusters
type hubClusters struct {
	sync.RWMutex
	clusters map[string]bool
}

func (hc *hubClusters) append(clusterName string) {
	hc.Lock()
	defer hc.Unlock()

	hc.clusters[clusterName] = true
}

func (hc *hubClusters) delete(clusterName string) {
	hc.Lock()
	defer hc.Unlock()

	delete(hc.clusters, clusterName)
}

var leafhubs = hubClusters{clusters: make(map[string]bool)}

// LeafHubReconciler reconciles a LeafHub Cluster
type LeafHubReconciler struct {
	DynamicClient dynamic.Interface
	KubeClient    kubernetes.Interface
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclusters,verbs=get;list;watch
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=hypershiftdeployments,verbs=get;list;watch
//+kubebuilder:rbac:groups=work.open-cluster-management.io,resources=manifestworks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=clustermanagementaddons,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=managedclusteraddons,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=packages.operators.coreos.com,resources=packagemanifests,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MultiClusterGlobalHub object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *LeafHubReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	log.Info("Reconciling", "namespacedname", req.NamespacedName)

	if config.GetHoHMGHNamespacedName().Namespace == "" ||
		config.GetHoHMGHNamespacedName().Name == "" {
		log.Info("multiclusterglobalhub resource is not available yet")
		return ctrl.Result{}, nil
	}

	// shouldPruneAll is the flag that indicates whether all the resources deployed to the leaf hubs should be removed
	shouldPruneAll := false

	// fetch the multiclusterglobalhub instance
	mgh := &operatorv1alpha1.MultiClusterGlobalHub{}
	err := r.Client.Get(context.TODO(), config.GetHoHMGHNamespacedName(), mgh)
	if err != nil {
		if errors.IsNotFound(err) {
			shouldPruneAll = true
		} else {
			// Error reading the object - requeue the request.
			return ctrl.Result{}, err
		}
	}

	// if multiclusterglobalhub is terminating, also set prune all flag to true
	if mgh.GetDeletionTimestamp() != nil {
		shouldPruneAll = true
	}

	if config.IsPaused(mgh) {
		log.Info("multiclusterglobalhub reconciliation is paused, nothing more to do")
		return ctrl.Result{}, nil
	}

	// if namespace of the reconcile request is empty, then the reconcile request is
	// from either managed cluster changes or manifestwork changes for that managedcluster
	// in either case, the controller doesn't need to go through all managed clusters
	if req.NamespacedName.Namespace == "" && req.NamespacedName.Name != "" {
		if err := r.reconcileLeafHub(ctx, req, mgh, shouldPruneAll, log); err != nil {
			if !shouldPruneAll {
				if conditionError := condition.SetConditionLeafHubDeployed(ctx, r.Client, mgh,
					req.NamespacedName.Name, condition.CONDITION_STATUS_FALSE); conditionError != nil {
					return ctrl.Result{}, fmt.Errorf(failedConditionMsg,
						condition.CONDITION_STATUS_FALSE, conditionError)
				}
			}
			return ctrl.Result{}, err
		}
		if !shouldPruneAll && !condition.ContainsCondition(mgh,
			condition.CONDITION_TYPE_LEAFHUB_DEPLOY) {
			if conditionError := condition.SetConditionLeafHubDeployed(ctx, r.Client, mgh, req.NamespacedName.Name,
				condition.CONDITION_STATUS_TRUE); conditionError != nil {
				return ctrl.Result{}, fmt.Errorf(failedConditionMsg,
					condition.CONDITION_STATUS_TRUE, conditionError)
			}
		}
		return ctrl.Result{}, nil
	}

	if err := r.reconcileMultiClusterGlobalHub(ctx, req, mgh, shouldPruneAll, log); err != nil {
		if conditionError := condition.SetConditionLeafHubDeployed(ctx, r.Client, mgh, "",
			condition.CONDITION_STATUS_FALSE); conditionError != nil {
			return ctrl.Result{}, fmt.Errorf(failedConditionMsg,
				condition.CONDITION_STATUS_FALSE, conditionError)
		}
		return ctrl.Result{}, err
	}

	if !shouldPruneAll && !condition.ContainsCondition(mgh,
		condition.CONDITION_TYPE_LEAFHUB_DEPLOY) {
		if conditionError := condition.SetConditionLeafHubDeployed(ctx, r.Client, mgh, req.NamespacedName.Name,
			condition.CONDITION_STATUS_TRUE); conditionError != nil {
			return ctrl.Result{}, fmt.Errorf(failedConditionMsg,
				condition.CONDITION_STATUS_TRUE, conditionError)
		}
	}
	return ctrl.Result{}, nil
}

// reconcileLeafHub reconciles a single leafhub
func (r *LeafHubReconciler) reconcileLeafHub(ctx context.Context, req ctrl.Request,
	mgh *operatorv1alpha1.MultiClusterGlobalHub, toDelete bool, log logr.Logger,
) error {
	if toDelete {
		// do nothing when in prune mode, the multiclusterglobalhub reconcile request will clean up resources for all leafhubs
		return nil
	}

	// Fetch the managedcluster instance
	managedCluster := &clusterv1.ManagedCluster{}
	managedClusterName := req.NamespacedName.Name

	// double check that current managedcluster is not local-cluster
	// in case the reconcile request is launched from manifework change
	if managedClusterName == constants.LocalClusterName {
		return nil
	}

	err := r.Get(ctx, req.NamespacedName, managedCluster)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("managedcluster resource not found. Ignoring since object must be deleted")
			return nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get managedcluster")
		return err
	}

	hostingClusterName, hostedClusterName, hostingNamespace := "", "", ""
	annotations := managedCluster.GetAnnotations()
	if val, ok := annotations["import.open-cluster-management.io/klusterlet-deploy-mode"]; ok && val == "Hosted" {
		hostingClusterName, ok = annotations["import.open-cluster-management.io/hosting-cluster-name"]
		if !ok || hostingClusterName == "" {
			return fmt.Errorf("missing hosting-cluster-name in managed cluster")
		}
		hypershiftdeploymentName, ok := annotations["cluster.open-cluster-management.io/hypershiftdeployment"]
		if !ok || hypershiftdeploymentName == "" {
			return fmt.Errorf("missing hypershiftdeployment name in managed cluster")
		}
		splits := strings.Split(hypershiftdeploymentName, "/")
		if len(splits) != 2 || splits[1] == "" {
			return fmt.Errorf("bad hypershiftdeployment name in managed cluster")
		}
		hypershiftDeploymentNamespace := splits[0]
		hostedClusterName = splits[1]

		hypershiftDeploymentInstance := &hypershiftdeploymentv1alpha1.HypershiftDeployment{}
		if err := r.Client.Get(ctx,
			types.NamespacedName{
				Namespace: hypershiftDeploymentNamespace,
				Name:      hostedClusterName,
			}, hypershiftDeploymentInstance); err != nil {
			return err
		}

		hostingNamespace = hypershiftDeploymentInstance.Spec.HostingNamespace

		// for hypershift hosted managedcluster, add leafhub annotation
		// TODO: remove this after UI supports this
		val, ok := annotations[commonconstants.ManagedClusterManagedByAnnotation]
		if !ok || val != commonconstants.GlobalHubOwnerLabelVal {
			annotations[commonconstants.ManagedClusterManagedByAnnotation] = commonconstants.GlobalHubOwnerLabelVal
			managedCluster.SetAnnotations(annotations)
			if err := r.Client.Update(ctx, managedCluster); err != nil {
				return err
			}
		}
	}

	// init the hostedcluster config
	hcConfig := &config.HostedClusterConfig{
		ManagedClusterName: managedClusterName,
		HostingClusterName: hostingClusterName,
		HostingNamespace:   hostingNamespace,
		HostedClusterName:  hostedClusterName,
	}

	// managedcluster is being deleted
	if !managedCluster.DeletionTimestamp.IsZero() {
		// the managed cluster is deleting, we should not re-apply the manifestwork
		// wait for managedcluster-import-controller to clean up the manifestwork
		if hostingClusterName == "" { // for non-hypershift hosted leaf hub
			if err := removePostponeDeleteAnnotationFromHubSubWork(ctx, r.Client, managedClusterName); err != nil {
				return err
			}
		} else { // for hypershift hosted leaf hub, remove the corresponding manifestwork from hypershift hosting cluster
			if err := removeLeafHubHostingWork(ctx, r.Client, managedClusterName, hostingClusterName); err != nil {
				return err
			}
		}
		// delete managedclusteraddon for the managedcluster
		return deleteManagedClusterAddon(ctx, r.Client, log, managedClusterName)
	}

	if managedCluster.GetLabels()[commonconstants.RegionalHubTypeLabelKey] ==
		commonconstants.RegionalHubTypeNoHubInstall {
		// this is for e2e testing only. In e2e tests, we install ocm in leaf hub.
		err := applyHoHAgentWork(ctx, r.Client, r.KubeClient, log, mgh, managedClusterName)
		if err != nil {
			return err
		}
	} else {
		pm, err := getPackageManifestConfig(ctx, r.DynamicClient, log)
		if err != nil {
			return err
		}
		if pm == nil || pm.ACMDefaultChannel == "" || pm.ACMCurrentCSV == "" {
			return fmt.Errorf("PackageManifest for ACM is not ready")
		}

		if hostingClusterName == "" { // for non-hypershift hosted leaf hub
			if err := r.reconcileNonHostedLeafHub(ctx, log, managedClusterName, mgh, pm); err != nil {
				return err
			}
		} else { // for hypershift hosted leaf hub
			if pm.MCEDefaultChannel == "" || pm.MCECurrentCSV == "" {
				return fmt.Errorf("PackageManifest for MCE is not ready")
			}

			if err := r.reconcileHostedLeafHub(ctx, log, mgh, pm, hcConfig); err != nil {
				return err
			}
		}
	}

	// apply ManagedClusterAddons
	return applyManagedClusterAddon(ctx, r.Client, log, managedClusterName)
}

// reconcileNonHostedLeafHub reconciles the normal leafhub, which is not running hosted mode
func (r *LeafHubReconciler) reconcileNonHostedLeafHub(ctx context.Context, log logr.Logger, managedClusterName string,
	mgh *operatorv1alpha1.MultiClusterGlobalHub, pm *packageManifestConfig,
) error {
	hubSubWork, err := applyHubSubWork(ctx, r.Client, r.KubeClient, log, managedClusterName, pm)
	if err != nil {
		return err
	}

	// if the csv PHASE is Succeeded, then create mch manifestwork to install Hub
	isSubReady, _ := findStatusFeedbackValueFromWork(hubSubWork, "Subscription", "state", "AtLatestKnown", log)
	if !isSubReady {
		return nil
	}

	log.Info("hub subscription is ready, applying hub MCH manifestwork")
	// TODO: fetch user defined mch from annotation
	hubMCHWork, err := applyHubMCHWork(ctx, r.Client, r.KubeClient, log, managedClusterName)
	if err != nil {
		return err
	}

	log.Info("checking status feedback value from hub mch manifestwork" +
		" before applying multicluster-global-hub-agent manifestwork")
	mchIsReadyNum := 0
	// if the MCH is Running, then create hoh agent manifestwork to install HoH agent
	// ideally, the mch status should be in Running state.
	// but due to this bug - https://github.com/stolostron/backlog/issues/20555
	// the mch status can be in Installing for a long time.
	// so here just check the dependencies status is True, then install HoH agent
	for _, mchManifestCondition := range hubMCHWork.Status.ResourceStatus.Manifests {
		if mchManifestCondition.ResourceMeta.Kind == "MultiClusterHub" {
			for _, value := range mchManifestCondition.StatusFeedbacks.Values {
				// no application-chart in 2.5
				// if value.Name == "application-chart-sub-status" && *value.Value.String == "True" {
				// 	mchIsReadyNum++
				// 	continue
				// }
				if value.Name == "cluster-manager-cr-status" && *value.Value.String == "True" {
					mchIsReadyNum++
					continue
				}
				// for ACM 2.5.
				if value.Name == "multicluster-engine-status" && *value.Value.String == "True" {
					mchIsReadyNum++
					continue
				}
				if value.Name == "grc-sub-status" && *value.Value.String == "True" {
					mchIsReadyNum++
					continue
				}
			}
		}
	}
	if mchIsReadyNum != 2 {
		log.Info("hub MCH is not ready, won't apply the multicluster-global-hub-agent manifestwork")
		return nil
	}

	// apply the multicluster-global-hub-agent manifestwork
	return applyHoHAgentWork(ctx, r.Client, r.KubeClient, log, mgh, managedClusterName)
}

// reconcileHostedLeafHub reconciles the multiclusterglobalhub change
func (r *LeafHubReconciler) reconcileHostedLeafHub(ctx context.Context, log logr.Logger,
	mgh *operatorv1alpha1.MultiClusterGlobalHub, pm *packageManifestConfig, hcConfig *config.HostedClusterConfig,
) error {
	// check the manifestwork for hosted hub before apply it, be careful about the order
	// don't call applyHubHypershiftWorks wil different channelClusterIP in one reconcile loop
	hubMgtWork := &workv1.ManifestWork{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: hcConfig.HostingClusterName,
		Name:      fmt.Sprintf("%s-%s", hcConfig.ManagedClusterName, constants.HoHHostingHubWorkSuffix),
	}, hubMgtWork); err != nil {
		if errors.IsNotFound(err) {
			_, err := applyHubHypershiftWorks(ctx, r.Client, r.KubeClient, log, mgh, "", pm, hcConfig)
			return err
		} else {
			return err
		}
	}

	isChannelServiceReady, channelServiceIP :=
		findStatusFeedbackValueFromWork(hubMgtWork, "Service", "clusterIP", "", log)
	if !isChannelServiceReady {
		log.Info("channel service is not ready, won't apply the multicluster-global-hub-agent manifestwork")
		return nil
	}

	log.Info("got clusterIP for channel service", "ClusterIP", channelServiceIP)
	if _, err := applyHubHypershiftWorks(ctx, r.Client, r.KubeClient, log, mgh,
		channelServiceIP, pm, hcConfig); err != nil {
		return err
	}

	// apply the multicluster-global-hub-agent manifestwork
	return applyHoHAgentHypershiftWork(ctx, r.Client, r.KubeClient, log, mgh, hcConfig)
}

// reconcileMultiClusterGlobalHub reconciles the multiclusterglobalhub change
func (r *LeafHubReconciler) reconcileMultiClusterGlobalHub(ctx context.Context, req ctrl.Request,
	mgh *operatorv1alpha1.MultiClusterGlobalHub, toPruneAll bool, log logr.Logger,
) error {
	// handle multiclusterglobalhub deleting
	if toPruneAll {
		log.Info("multiclusterglobalhub is terminating, delete manifests for leafhubs...")
		// remove the leafhub components
		for leafhub := range leafhubs.clusters {
			if err := r.Client.DeleteAllOf(ctx, &workv1.ManifestWork{}, client.InNamespace(leafhub),
				client.MatchingLabels(map[string]string{
					commonconstants.GlobalHubOwnerLabelKey: commonconstants.HoHOperatorOwnerLabelVal,
				})); err != nil {
				return err
			}
			// delete managedclusteraddon
			if err := deleteManagedClusterAddon(ctx, r.Client, log, leafhub); err != nil {
				return err
			}
		}

		// also handle case of local-cluster as hypershift hosting cluster
		if err := r.Client.DeleteAllOf(ctx, &workv1.ManifestWork{},
			client.InNamespace(constants.LocalClusterName),
			client.MatchingLabels(map[string]string{
				commonconstants.GlobalHubOwnerLabelKey: commonconstants.HoHOperatorOwnerLabelVal,
			})); err != nil {
			return err
		}

		// delete ClusterManagementAddon
		if err := deleteClusterManagementAddon(ctx, r.Client, log); err != nil {
			return err
		}

		return nil
	}

	if err := applyClusterManagementAddon(ctx, r.Client, log); err != nil {
		return err
	}

	errors := []error{}
	// handle multiclusterglobalhub change here
	for leafhub := range leafhubs.clusters {
		newReq := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name: leafhub,
			},
		}

		// trigger reconcile for each leafhub
		if err := r.reconcileLeafHub(ctx, newReq, mgh, false, log); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return utilerrors.NewAggregate(errors)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LeafHubReconciler) SetupWithManager(mgr ctrl.Manager) error {
	clusterPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetLabels()["vendor"] == "OpenShift" &&
				e.Object.GetName() != constants.LocalClusterName &&
				e.Object.GetLabels()[commonconstants.RegionalHubTypeLabelKey] !=
					commonconstants.RegionalHubTypeNoHubAgentInstall &&
				meta.IsStatusConditionTrue(e.Object.(*clusterv1.ManagedCluster).Status.Conditions,
					"ManagedClusterConditionAvailable") {
				leafhubs.append(e.Object.GetName())
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetLabels()["vendor"] == "OpenShift" &&
				e.ObjectNew.GetName() != constants.LocalClusterName &&
				e.ObjectNew.GetLabels()[commonconstants.RegionalHubTypeLabelKey] !=
					commonconstants.RegionalHubTypeNoHubAgentInstall &&
				meta.IsStatusConditionTrue(e.ObjectNew.(*clusterv1.ManagedCluster).Status.Conditions,
					"ManagedClusterConditionAvailable") {
				if e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
					if e.ObjectNew.GetDeletionTimestamp() == nil {
						leafhubs.append(e.ObjectNew.GetName())
					} else {
						leafhubs.delete(e.ObjectNew.GetName())
					}
					return true
				}
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetLabels()["vendor"] == "OpenShift" &&
				e.Object.GetName() != constants.LocalClusterName &&
				e.Object.GetLabels()[commonconstants.RegionalHubTypeLabelKey] !=
					commonconstants.RegionalHubTypeNoHubAgentInstall &&
				meta.IsStatusConditionTrue(e.Object.(*clusterv1.ManagedCluster).Status.Conditions,
					"ManagedClusterConditionAvailable") {
				leafhubs.delete(e.Object.GetName())
				return true
			}
			return false
		},
	}

	mghPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			config.SetHoHMGHNamespacedName(types.NamespacedName{
				Namespace: e.Object.GetNamespace(),
				Name:      e.Object.GetName(),
			})
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// only requeue the event when leafhub configuration is changed
			return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return !e.DeleteStateUnknown
		},
	}

	clusterManagementAddonPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetLabels()[commonconstants.GlobalHubOwnerLabelKey] ==
				commonconstants.HoHOperatorOwnerLabelVal &&
				e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration() {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return e.Object.GetLabels()[commonconstants.GlobalHubOwnerLabelKey] ==
				commonconstants.HoHOperatorOwnerLabelVal
		},
	}

	managedClusterAddonPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetLabels()[commonconstants.GlobalHubOwnerLabelKey] ==
				commonconstants.HoHOperatorOwnerLabelVal &&
				e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration() {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return e.Object.GetLabels()[commonconstants.GlobalHubOwnerLabelKey] ==
				commonconstants.HoHOperatorOwnerLabelVal
		},
	}

	workPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetLabels()[commonconstants.GlobalHubOwnerLabelKey] ==
				commonconstants.HoHOperatorOwnerLabelVal &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				if e.ObjectNew.GetName() == fmt.Sprintf("%s-%s", e.ObjectNew.GetNamespace(),
					constants.HOHHubSubscriptionWorkSuffix) ||
					e.ObjectNew.GetName() == fmt.Sprintf("%s-%s", e.ObjectNew.GetNamespace(), constants.HoHHubMCHWorkSuffix) ||
					strings.Contains(e.ObjectNew.GetName(), constants.HoHHostingHubWorkSuffix) {
					return true
				} else if !reflect.DeepEqual(e.ObjectNew.(*workv1.ManifestWork).Spec.Workload.Manifests,
					e.ObjectOld.(*workv1.ManifestWork).Spec.Workload.Manifests) {
					return true
				}
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return e.Object.GetLabels()[commonconstants.GlobalHubOwnerLabelKey] ==
				commonconstants.HoHOperatorOwnerLabelVal
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		// primary watch for managedcluster
		For(&clusterv1.ManagedCluster{}, builder.WithPredicates(clusterPred)).
		// watch for multiclusterglobalhub change
		Watches(&source.Kind{Type: &operatorv1alpha1.MultiClusterGlobalHub{}},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(mghPred)).
		// watch for clustermanagementaddon change
		Watches(&source.Kind{Type: &addonv1alpha1.ClusterManagementAddOn{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					{NamespacedName: types.NamespacedName{
						// add fake namespace to trigger MGH reconcile when clustermanagementaddon updated/deleted
						Namespace: constants.HOHDefaultNamespace,
						Name:      obj.GetName(),
					}},
				}
			}), builder.WithPredicates(clusterManagementAddonPred)).
		// secondary watch for managedclusteraddon
		Watches(&source.Kind{Type: &addonv1alpha1.ManagedClusterAddOn{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					// only trigger the leafhub reconcile when managedclusteraddon is updated/deleted
					{NamespacedName: types.NamespacedName{
						Name: obj.GetNamespace(),
					}},
				}
			}), builder.WithPredicates(managedClusterAddonPred)).
		// secondary watch for manifestwork
		Watches(&source.Kind{Type: &workv1.ManifestWork{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				managedClusterName := obj.GetNamespace()
				if strings.Contains(obj.GetName(), constants.HoHHostingHubWorkSuffix) {
					managedClusterName = strings.TrimSuffix(obj.GetName(), fmt.Sprintf("-%s",
						constants.HoHHostingHubWorkSuffix))
				}
				return []reconcile.Request{
					{NamespacedName: types.NamespacedName{
						Name: managedClusterName,
					}},
				}
			}), builder.WithPredicates(workPred)).
		Complete(r)
}

// findStatusFeedbackValueFromWork finds the expected feedback value from given resource
// and field in manifestwork status
// return true if the expected value is found
func findStatusFeedbackValueFromWork(work *workv1.ManifestWork, kind, feedbackField, feedbackValue string,
	log logr.Logger,
) (bool, string) {
	log.Info("checking status feedback value from manifestwork", "manifestwork namespace", work.GetNamespace(),
		"manifestwork name", work.GetName(), "resource kind", kind, "feedback field", feedbackField)
	for _, manifestCondition := range work.Status.ResourceStatus.Manifests {
		if manifestCondition.ResourceMeta.Kind == kind {
			for _, value := range manifestCondition.StatusFeedbacks.Values {
				if value.Name == feedbackField && value.Value.String != nil &&
					(*value.Value.String == feedbackValue || feedbackValue == "") {
					return true, *value.Value.String
				}
			}
		}
	}

	return false, ""
}
