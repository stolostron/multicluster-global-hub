/*
Copyright 2024.

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

package crd

import (
	"context"
	"sync"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	runtimeController "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/addons"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/agent"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/backup"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var ACMCrds = []string{
	"multiclusterhubs.operator.open-cluster-management.io",
	"managedclusters.cluster.open-cluster-management.io",
	"clustermanagementaddons.addon.open-cluster-management.io",
	"managedclusteraddons.addon.open-cluster-management.io",
	"manifestworks.work.open-cluster-management.io",
}

var KafkaCrds = []string{
	"kafkas.kafka.strimzi.io",
	"kafkatopics.kafka.strimzi.io",
	"kafkausers.kafka.strimzi.io",
}

// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;update

// CrdController reconciles a ACM Resources. It might be removed once the target controller is started.
// https://github.com/kubernetes-sigs/controller-runtime/pull/2159
type CrdController struct {
	manager.Manager
	kubeClient            *kubernetes.Clientset
	operatorConfig        *config.OperatorConfig
	resources             map[string]bool
	addonInstallerReady   bool
	agentController       *agent.AddonController
	globalHubController   runtimeController.Controller
	backupControllerReady bool
	addonsControllerReady bool
	mu                    sync.Mutex
}

func (r *CrdController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// the reconcile will update the resources map in multiple goroutines simultaneously
	r.mu.Lock()
	defer r.mu.Unlock()
	// Check if mgh exist or deleting
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.GetClient())
	if err != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	if mgh.DeletionTimestamp != nil {
		klog.V(2).Info("mgh instance is deleting")
		return ctrl.Result{}, nil
	}

	// set resource as ready
	r.resources[req.Name] = true

	// mark the states of kafka crd
	if r.readyToWatchKafkaResources() {
		config.SetKafkaResourceReady(true)
	}

	if r.readyToWatchACMResources() {
		config.SetACMResourceReady(true)
		if err := r.watchACMRelatedResources(); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		return ctrl.Result{}, nil
	}

	// start addon installer
	if !r.addonInstallerReady {
		if err := (&agent.AddonInstaller{
			Client: r.GetClient(),
			Log:    ctrl.Log.WithName("addon-reconciler"),
		}).SetupWithManager(ctx, r.Manager); err != nil {
			return ctrl.Result{}, err
		}
		r.addonInstallerReady = true
	}

	// start addon controller
	if r.agentController == nil {
		agentController, err := agent.NewAddonController(r.Manager.GetConfig(), r.Manager.GetClient(), r.operatorConfig)
		if err != nil {
			return ctrl.Result{}, err
		}
		err = r.Manager.Add(agentController)
		if err != nil {
			return ctrl.Result{}, err
		}
		r.agentController = agentController
	}

	// backup controller
	if !r.backupControllerReady {
		err := backup.NewBackupReconciler(r.Manager, ctrl.Log.WithName("backup-reconciler")).SetupWithManager(r.Manager)
		if err != nil {
			return ctrl.Result{}, err
		}
		r.backupControllerReady = true
	}

	if !r.addonsControllerReady {
		err := addons.NewAddonsReconciler(r.Manager).SetupWithManager(r.Manager)
		if err != nil {
			return ctrl.Result{}, err
		}
		r.addonsControllerReady = true
	}
	return ctrl.Result{}, nil
}

func (r *CrdController) readyToWatchACMResources() bool {
	for _, val := range ACMCrds {
		if ready := r.resources[val]; !ready {
			return false
		}
	}
	return true
}

func (r *CrdController) readyToWatchKafkaResources() bool {
	for _, val := range KafkaCrds {
		if ready := r.resources[val]; !ready {
			return false
		}
	}
	return true
}

func AddCRDController(mgr ctrl.Manager, operatorConfig *config.OperatorConfig,
	kubeClient *kubernetes.Clientset, globalHubController runtimeController.Controller,
) (*CrdController, error) {
	allCrds := map[string]bool{}
	for _, val := range ACMCrds {
		allCrds[val] = false
	}
	for _, val := range KafkaCrds {
		allCrds[val] = false
	}
	controller := &CrdController{
		Manager:             mgr,
		kubeClient:          kubeClient,
		operatorConfig:      operatorConfig,
		resources:           allCrds,
		globalHubController: globalHubController,
	}

	crdPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// the predicate function might be invoked in different controller goroutines.
			controller.mu.Lock()
			_, ok := controller.resources[e.Object.GetName()]
			controller.mu.Unlock()
			return ok
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}

	return controller, ctrl.NewControllerManagedBy(mgr).
		Named("CRDController").
		WatchesMetadata(
			&apiextensionsv1.CustomResourceDefinition{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(crdPred),
		).
		Complete(controller)
}

func (r *CrdController) watchACMRelatedResources() error {
	// add watcher dynamically
	if err := r.globalHubController.Watch(
		source.Kind(
			r.Manager.GetCache(), &v1alpha1.ClusterManagementAddOn{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context,
				c *v1alpha1.ClusterManagementAddOn,
			) []reconcile.Request {
				return []reconcile.Request{
					// trigger MGH instance reconcile
					{NamespacedName: config.GetMGHNamespacedName()},
				}
			}), watchClusterManagementAddOnPredict(),
		)); err != nil {
		return err
	}
	if err := r.globalHubController.Watch(
		source.Kind(
			r.Manager.GetCache(), &clusterv1.ManagedCluster{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context,
				c *clusterv1.ManagedCluster,
			) []reconcile.Request {
				return []reconcile.Request{
					// trigger MGH instance reconcile
					{NamespacedName: config.GetMGHNamespacedName()},
				}
			}), watchManagedClusterPredict(),
		)); err != nil {
		return err
	}
	return nil
}

func watchClusterManagementAddOnPredict() predicate.TypedPredicate[*v1alpha1.ClusterManagementAddOn] {
	return predicate.TypedFuncs[*v1alpha1.ClusterManagementAddOn]{
		CreateFunc: func(e event.TypedCreateEvent[*v1alpha1.ClusterManagementAddOn]) bool {
			return false
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha1.ClusterManagementAddOn]) bool {
			if e.ObjectNew.GetLabels()[constants.GlobalHubOwnerLabelKey] !=
				constants.GHOperatorOwnerLabelVal {
				return false
			}
			// only requeue when spec change, if the resource do not have spec field, the generation is always 0
			if e.ObjectNew.GetGeneration() == 0 {
				return true
			}
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
		},
		DeleteFunc: func(e event.TypedDeleteEvent[*v1alpha1.ClusterManagementAddOn]) bool {
			return e.Object.GetLabels()[constants.GlobalHubOwnerLabelKey] ==
				constants.GHOperatorOwnerLabelVal
		},
	}
}

func watchManagedClusterPredict() predicate.TypedPredicate[*clusterv1.ManagedCluster] {
	return predicate.TypedFuncs[*clusterv1.ManagedCluster]{
		CreateFunc: func(e event.TypedCreateEvent[*clusterv1.ManagedCluster]) bool {
			return false
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*clusterv1.ManagedCluster]) bool {
			if e.ObjectNew.GetLabels()[constants.GlobalHubOwnerLabelKey] !=
				constants.GHOperatorOwnerLabelVal {
				return false
			}
			// only requeue when spec change, if the resource do not have spec field, the generation is always 0
			if e.ObjectNew.GetGeneration() == 0 {
				return true
			}
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
		},
		DeleteFunc: func(e event.TypedDeleteEvent[*clusterv1.ManagedCluster]) bool {
			return e.Object.GetLabels()[constants.GlobalHubOwnerLabelKey] ==
				constants.GHOperatorOwnerLabelVal
		},
	}
}
