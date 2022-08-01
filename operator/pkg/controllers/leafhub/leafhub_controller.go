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
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"
	hubofhubsv1alpha1 "github.com/stolostron/hub-of-hubs/operator/apis/hubofhubs/v1alpha1"
	"github.com/stolostron/hub-of-hubs/operator/pkg/config"
	"github.com/stolostron/hub-of-hubs/operator/pkg/constants"

	hypershiftdeploymentv1alpha1 "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
)

var logger = ctrllog.Log.WithName("controller_leafhub")

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
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclusters,verbs=get;list;watch
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=hypershiftdeployments,verbs=get
//+kubebuilder:rbac:groups=work.open-cluster-management.io,resources=manifestworks,verbs=get;list;watch
//+kubebuilder:rbac:groups=packages.operators.coreos.com,resources=packagemanifests,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Config object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *LeafHubReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	log.Info("Reconciling", "namespacedname", req.NamespacedName)

	if config.GetHoHConfigNamespacedName().Namespace == "" || config.GetHoHConfigNamespacedName().Name == "" {
		log.Info("HoH Config resource is not available yet")
		return ctrl.Result{}, nil
	}

	// shouldPruneAll is the flag that indicates whether all the resources deployed to the leaf hubs should be removed
	shouldPruneAll := false

	// fetch the hoH config instance
	hohConfig := &hubofhubsv1alpha1.Config{}
	err := r.Client.Get(context.TODO(), config.GetHoHConfigNamespacedName(), hohConfig)
	if err != nil {
		if errors.IsNotFound(err) {
			shouldPruneAll = true
		} else {
			// Error reading the object - requeue the request.
			return ctrl.Result{}, err
		}
	}

	// if namespace of the reconcile request is empty, then the reconcile request is
	// from either managed cluster changes or manifestwork changes for that managedcluster
	// in either case, the controller doesn't need to go through all managed clusters
	if req.NamespacedName.Namespace == "" && req.NamespacedName.Name != "" {
		return r.reconcileLeafHub(ctx, req, hohConfig, shouldPruneAll, log)
	}

	return r.reconcileHoHConfig(ctx, req, hohConfig, shouldPruneAll, log)
}

// reconcileLeafHub reconciles a single leafhub
func (r *LeafHubReconciler) reconcileLeafHub(ctx context.Context, req ctrl.Request, hohConfig *hubofhubsv1alpha1.Config, toDelete bool, log logr.Logger) (ctrl.Result, error) {
	if toDelete {
		// do nothing when in prune mode, the hoh config reconcile request will clean up resources for all leafhubs
		return ctrl.Result{}, nil
	}

	hohConfigName := hohConfig.GetName()
	// Fetch the managedcluster instance
	managedCluster := &clusterv1.ManagedCluster{}
	managedClusterName := req.NamespacedName.Name

	// double check that current managedcluster is not local-cluster
	// in case the reconcile request is launched from manifework change
	if managedClusterName == "local-cluster" {
		return ctrl.Result{}, nil
	}

	err := r.Get(ctx, req.NamespacedName, managedCluster)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("managedcluster resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get managedcluster")
		return ctrl.Result{}, err
	}

	hostingClusterName, hostedClusterName, hypershiftDeploymentNamespace := "", "", ""
	annotations := managedCluster.GetAnnotations()
	if val, ok := annotations["import.open-cluster-management.io/klusterlet-deploy-mode"]; ok && val == "Hosted" {
		hostingClusterName, ok = annotations["import.open-cluster-management.io/hosting-cluster-name"]
		if !ok || hostingClusterName == "" {
			return ctrl.Result{}, fmt.Errorf("missing hosting-cluster-name in managed cluster.")
		}
		hypershiftdeploymentName, ok := annotations["cluster.open-cluster-management.io/hypershiftdeployment"]
		if !ok || hypershiftdeploymentName == "" {
			return ctrl.Result{}, fmt.Errorf("missing hypershiftdeployment name in managed cluster.")
		}
		splits := strings.Split(hypershiftdeploymentName, "/")
		if len(splits) != 2 || splits[1] == "" {
			return ctrl.Result{}, fmt.Errorf("bad hypershiftdeployment name in managed cluster.")
		}
		hypershiftDeploymentNamespace = splits[0]
		hostedClusterName = splits[1]

		// for hypershift hosted managedcluster, add leafhub annotation
		// TODO: remove this after UI supports this
		if val, ok := annotations[constants.LeafHubClusterAnnotationKey]; !ok || val != "true" {
			annotations[constants.LeafHubClusterAnnotationKey] = "true"
			managedCluster.SetAnnotations(annotations)
			if err := r.Client.Update(ctx, managedCluster); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// managedcluster is being deleted
	if !managedCluster.DeletionTimestamp.IsZero() {
		// the managed cluster is deleting, we should not re-apply the manifestwork
		// wait for managedcluster-import-controller to clean up the manifestwork
		if hostingClusterName == "" { // for non-hypershift hosted leaf hub
			err := removePostponeDeleteAnnotationFromHubSubWork(ctx, r.Client, managedClusterName)
			return ctrl.Result{}, err
		} else { // for hypershift hosted leaf hub, remove the corresponding manifestwork from hypershift hosting cluster
			err := removeLeafHubHostingWork(ctx, r.Client, managedClusterName, hostingClusterName)
			return ctrl.Result{}, err
		}
	}

	pm, err := getPackageManifestConfig(ctx, r.Client, log)
	if err != nil {
		return ctrl.Result{}, err
	}
	if pm == nil || pm.ACMDefaultChannel == "" || pm.ACMCurrentCSV == "" {
		log.Info("PackageManifest for ACM is not ready yet")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if hostingClusterName == "" { // for non-hypershift hosted leaf hub
		hubSubWork, err := applyHubSubWork(ctx, r.Client, log, hohConfigName, managedClusterName, pm)
		if err != nil {
			return ctrl.Result{}, err
		}

		// if the csv PHASE is Succeeded, then create mch manifestwork to install Hub
		log.Info("checking status feedback value from hub subscription manifestwork before applying hub mch manifestwork")
		for _, manifestCondition := range hubSubWork.Status.ResourceStatus.Manifests {
			if manifestCondition.ResourceMeta.Kind == "Subscription" {
				for _, value := range manifestCondition.StatusFeedbacks.Values {
					if value.Name == "state" && *value.Value.String == "AtLatestKnown" {
						// TODO: fetch user defined mch from annotation
						hubMCHWork, err := applyHubMCHWork(ctx, r.Client, log, hohConfigName, managedClusterName)
						if err != nil {
							return ctrl.Result{}, err
						}
						log.Info("checking status feedback value from hub mch manifestwork before applying hub-of-hubs-agent manifestwork")
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
							log.Info("hub MCH is not ready, won't apply the hub-of-hubs-agent manifestwork")
							return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
						}

						// apply the hub-of-hubs-agent manifestwork
						return ctrl.Result{}, applyHoHAgentWork(ctx, r.Client, log, hohConfig, managedClusterName)
					}
				}
			}
		}
	} else { // for hypershift hosted leaf hub
		if pm == nil || pm.ACMDefaultChannel == "" || pm.ACMCurrentCSV == "" || pm.MCEDefaultChannel == "" || pm.MCECurrentCSV == "" {
			log.Info("PackageManifests for ACM and MCE are not ready yet")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		hypershiftDeploymentInstance := &hypershiftdeploymentv1alpha1.HypershiftDeployment{}
		if err := r.Client.Get(ctx,
			types.NamespacedName{
				Namespace: hypershiftDeploymentNamespace,
				Name:      hostedClusterName,
			}, hypershiftDeploymentInstance); err != nil {
			return ctrl.Result{}, err
		}
		hostingNamespace := hypershiftDeploymentInstance.Spec.HostingNamespace

		// check the manifestwork for hosted hub before apply it, be careful about the order
		// don't call applyHubHypershiftWorks wil different channelClusterIP in one reconcile loop
		hubMgtWork := &workv1.ManifestWork{}
		if err = r.Client.Get(ctx, types.NamespacedName{
			Namespace: hostingClusterName,
			Name:      fmt.Sprintf("%s-%s", managedClusterName, constants.HoHHostingHubWorkSuffix),
		}, hubMgtWork); err != nil {
			if errors.IsNotFound(err) {
				_, err := applyHubHypershiftWorks(ctx, r.Client, log, pm, hohConfigName, managedClusterName, hostingClusterName, hostingNamespace, hostedClusterName, "")
				return ctrl.Result{}, err
			} else {
				return ctrl.Result{}, err
			}
		}

		log.Info("checking status feedback value from hypershift hub manifestwork before applying channel service to manifestwork")
		for _, manifestCondition := range hubMgtWork.Status.ResourceStatus.Manifests {
			if manifestCondition.ResourceMeta.Kind == "Service" {
				for _, value := range manifestCondition.StatusFeedbacks.Values {
					if value.Name == "clusterIP" && value.Value.String != nil {
						log.Info("Got clusterIP for channel service", "ClusterIP", *value.Value.String)
						channelClusterIP := *value.Value.String
						if _, err := applyHubHypershiftWorks(ctx, r.Client, log, pm, hohConfigName, managedClusterName, hostingClusterName, hostingNamespace, hostedClusterName, channelClusterIP); err != nil {
							return ctrl.Result{}, err
						}

						// apply the hub-of-hubs-agent manifestwork
						return ctrl.Result{}, applyHoHAgentHypershiftWork(ctx, r.Client, log, hohConfig, hohConfigName, managedClusterName, hostingClusterName, hostingNamespace, hostedClusterName)
					}
				}
			}
		}
	}

	return ctrl.Result{}, nil
}

// reconcileHoHConfig reconsiles the hoh config change
func (r *LeafHubReconciler) reconcileHoHConfig(ctx context.Context, req ctrl.Request, hohConfig *hubofhubsv1alpha1.Config, toPruneAll bool, log logr.Logger) (ctrl.Result, error) {
	// handle hoh config deleting
	if hohConfig.GetDeletionTimestamp() != nil {
		log.Info("hoh config is terminating, delete manifests for leafhubs...")
		// remove the leafhub components
		for leafhub := range leafhubs.clusters {
			listOpts := []client.ListOption{
				client.InNamespace(leafhub),
				client.MatchingLabels(map[string]string{constants.HoHOperatorOwnerLabelKey: hohConfig.GetName()}),
			}
			workList := &workv1.ManifestWorkList{}
			err := r.Client.List(ctx, workList, listOpts...)
			if err != nil {
				return ctrl.Result{}, err
			}
			for _, work := range workList.Items {
				err := r.Client.Delete(ctx, &work, &client.DeleteOptions{})
				if err != nil {
					return ctrl.Result{}, err
				}
			}
		}

		// also handle case of local-cluster as hypershift hosting cluster
		listOpts := []client.ListOption{
			client.InNamespace("local-cluster"),
			client.MatchingLabels(map[string]string{constants.HoHOperatorOwnerLabelKey: hohConfig.GetName()}),
		}
		workList := &workv1.ManifestWorkList{}
		err := r.Client.List(ctx, workList, listOpts...)
		if err != nil {
			return ctrl.Result{}, err
		}
		for _, work := range workList.Items {
			err := r.Client.Delete(ctx, &work, &client.DeleteOptions{})
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// TODO: handle hoh config change here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LeafHubReconciler) SetupWithManager(mgr ctrl.Manager) error {
	clusterPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetLabels()["vendor"] == "OpenShift" &&
				e.Object.GetName() != "local-cluster" &&
				e.Object.GetLabels()[constants.LeafHubClusterDisabledLabelKey] != constants.LeafHubClusterDisabledLabelval &&
				meta.IsStatusConditionTrue(e.Object.(*clusterv1.ManagedCluster).Status.Conditions, "ManagedClusterConditionAvailable") {
				leafhubs.append(e.Object.GetName())
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetLabels()["vendor"] == "OpenShift" &&
				e.ObjectNew.GetName() != "local-cluster" &&
				e.ObjectNew.GetLabels()[constants.LeafHubClusterDisabledLabelKey] != constants.LeafHubClusterDisabledLabelval &&
				meta.IsStatusConditionTrue(e.ObjectNew.(*clusterv1.ManagedCluster).Status.Conditions, "ManagedClusterConditionAvailable") {
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
				e.Object.GetName() != "local-cluster" &&
				e.Object.GetLabels()[constants.LeafHubClusterDisabledLabelKey] != constants.LeafHubClusterDisabledLabelval &&
				meta.IsStatusConditionTrue(e.Object.(*clusterv1.ManagedCluster).Status.Conditions, "ManagedClusterConditionAvailable") {
				leafhubs.delete(e.Object.GetName())
				return true
			}
			return false
		},
	}

	hohConfigPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			config.SetHoHConfigNamespacedName(types.NamespacedName{Namespace: e.Object.GetNamespace(), Name: e.Object.GetName()})
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// only requeue the event when leafhub configuration is changed
			if e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return !e.DeleteStateUnknown
		},
	}

	workPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetLabels()[constants.HoHOperatorOwnerLabelKey] != "" &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				// && !reflect.DeepEqual(e.ObjectNew.(*workv1.ManifestWork).Spec.Workload.Manifests,
				// 	e.ObjectOld.(*workv1.ManifestWork).Spec.Workload.Manifests) {
				if e.ObjectNew.GetName() == e.ObjectNew.GetNamespace()+constants.HOHHubSubscriptionWorkSuffix ||
					e.ObjectNew.GetName() == e.ObjectNew.GetNamespace()+constants.HoHHubMCHWorkSuffix ||
					strings.Contains(e.ObjectNew.GetName(), constants.HoHHostingHubWorkSuffix) {
					return true
				}
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		// primary watch for managedcluster
		For(&clusterv1.ManagedCluster{}, builder.WithPredicates(clusterPred)).
		// secondary watch for manifestwork
		Watches(&source.Kind{Type: &workv1.ManifestWork{}}, handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
			managedClusterName := obj.GetNamespace()
			if strings.Contains(obj.GetName(), constants.HoHHostingHubWorkSuffix) {
				managedClusterName = strings.TrimSuffix(obj.GetName(), fmt.Sprintf("-%s", constants.HoHHostingHubWorkSuffix))
			}
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{
					Name: managedClusterName,
				}},
			}
		}), builder.WithPredicates(workPred)).
		// watch for hoh config change
		Watches(&source.Kind{Type: &hubofhubsv1alpha1.Config{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(hohConfigPred)).
		Complete(r)
}
