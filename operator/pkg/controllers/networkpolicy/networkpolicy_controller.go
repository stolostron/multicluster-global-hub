package networkpolicy

import (
	"context"
	"embed"
	"fmt"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs,verbs=get;list;watch;
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;delete

//go:embed manifests
var fs embed.FS

var log = logger.DefaultZapLogger()

const (
	NetworkPolicyDefaultDenyAll = "default-deny-all"
	NetworkPolicyAllowDNSAndAPI = "allow-dns-and-api"
	NetworkPolicyOperator       = "multicluster-global-hub-operator"
)

type NetworkPolicyReconciler struct {
	ctrl.Manager
	kubeClient kubernetes.Interface
}

var networkPolicyController *NetworkPolicyReconciler

func StartController(initOption config.ControllerOption) (config.ControllerInterface, error) {
	if networkPolicyController != nil {
		return networkPolicyController, nil
	}
	log.Info("start networkpolicy controller")

	networkPolicyController = &NetworkPolicyReconciler{
		Manager:    initOption.Manager,
		kubeClient: initOption.KubeClient,
	}
	err := networkPolicyController.SetupWithManager(initOption.Manager)
	if err != nil {
		networkPolicyController = nil
		return networkPolicyController, err
	}
	log.Info("initialized networkpolicy controller")
	return networkPolicyController, nil
}

func (r *NetworkPolicyReconciler) IsResourceRemoved() bool {
	return true
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("networkpolicyController").
		For(&v1alpha4.MulticlusterGlobalHub{},
			builder.WithPredicates(config.MGHPred)).
		Watches(&networkingv1.NetworkPolicy{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(networkPolicyPred)).
		Complete(r)
}

var networkPolicyPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			(e.Object.GetName() == NetworkPolicyDefaultDenyAll ||
				e.Object.GetName() == NetworkPolicyAllowDNSAndAPI ||
				e.Object.GetName() == NetworkPolicyOperator)
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetNamespace() == commonutils.GetDefaultNamespace() &&
			(e.ObjectNew.GetName() == NetworkPolicyDefaultDenyAll ||
				e.ObjectNew.GetName() == NetworkPolicyAllowDNSAndAPI ||
				e.ObjectNew.GetName() == NetworkPolicyOperator)
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			(e.Object.GetName() == NetworkPolicyDefaultDenyAll ||
				e.Object.GetName() == NetworkPolicyAllowDNSAndAPI ||
				e.Object.GetName() == NetworkPolicyOperator)
	},
}

func (r *NetworkPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Debug("reconcile networkpolicy controller")

	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.GetClient())
	if err != nil {
		return ctrl.Result{}, err
	}
	if mgh == nil || config.IsPaused(mgh) || mgh.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	// Render NetworkPolicy manifests
	npRenderer := renderer.NewHoHRenderer(fs)
	npDeployer := deployer.NewHoHDeployer(r.GetClient())

	networkPolicyObjects, err := npRenderer.Render("manifests", "", func(profile string) (interface{}, error) {
		return struct {
			Namespace    string
			PostgresName string
		}{
			Namespace:    mgh.Namespace,
			PostgresName: config.COMPONENTS_POSTGRES_NAME,
		}, nil
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to render networkpolicy manifests: %w", err)
	}

	// Create restmapper for deployer to find GVR
	dc, err := discovery.NewDiscoveryClientForConfig(r.GetConfig())
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create discovery client: %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	// Deploy NetworkPolicies
	if err = operatorutils.ManipulateGlobalHubObjects(networkPolicyObjects, mgh, npDeployer,
		mapper, r.GetScheme()); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to deploy networkpolicy: %w", err)
	}

	log.Info("networkpolicy reconciliation completed successfully")
	return ctrl.Result{}, nil
}
