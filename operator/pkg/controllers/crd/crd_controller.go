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

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/addon"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/backup"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs"
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
	kubeClient               *kubernetes.Clientset
	operatorConfig           *config.OperatorConfig
	resources                map[string]bool
	addonInstallerReady      bool
	addonController          *addon.AddonController
	globalHubControllerReady bool
	backupControllerReady    bool
	mu                       sync.Mutex
}

func (r *CrdController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// the reconcile will update the resources map in multiple goroutines simultaneously
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resources[req.Name] = true
	if !r.readyToWatchACMResources() {
		return ctrl.Result{}, nil
	}

	// mark the states of kafka crd
	if r.readyToWatchKafkaResources() {
		config.SetKafkaResourceReady(true)
	}

	// start addon installer
	if !r.addonInstallerReady {
		if err := (&addon.AddonInstaller{
			Client: r.GetClient(),
			Log:    ctrl.Log.WithName("addon-reconciler"),
		}).SetupWithManager(ctx, r.Manager); err != nil {
			return ctrl.Result{}, err
		}
		r.addonInstallerReady = true
	}

	// start addon controller
	if r.addonController == nil {
		addonController, err := addon.NewAddonController(r.Manager.GetConfig(), r.Manager.GetClient(), r.operatorConfig)
		if err != nil {
			return ctrl.Result{}, err
		}
		err = r.Manager.Add(addonController)
		if err != nil {
			return ctrl.Result{}, err
		}
		r.addonController = addonController
	}

	// global hub controller
	if !r.globalHubControllerReady {
		err := hubofhubs.NewMulticlusterGlobalHubController(
			r.Manager,
			r.addonController.AddonManager(),
			r.kubeClient,
			r.operatorConfig,
		).SetupWithManager(r.Manager)
		if err != nil {
			return ctrl.Result{}, err
		}
		r.globalHubControllerReady = true
	}

	// backup controller
	if !r.backupControllerReady {
		err := backup.NewBackupReconciler(r.Manager, ctrl.Log.WithName("backup-reconciler")).SetupWithManager(r.Manager)
		if err != nil {
			return ctrl.Result{}, err
		}
		r.backupControllerReady = true
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
	kubeClient *kubernetes.Clientset,
) (*CrdController, error) {
	allCrds := map[string]bool{}
	for _, val := range ACMCrds {
		allCrds[val] = false
	}
	for _, val := range KafkaCrds {
		allCrds[val] = false
	}
	controller := &CrdController{
		Manager:        mgr,
		kubeClient:     kubeClient,
		operatorConfig: operatorConfig,
		resources:      allCrds,
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
