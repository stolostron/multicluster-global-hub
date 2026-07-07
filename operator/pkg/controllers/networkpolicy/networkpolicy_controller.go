package networkpolicy

import (
	"context"
	"embed"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	nputils "github.com/stolostron/multicluster-global-hub/operator/pkg/networkpolicy"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs,verbs=get;list;watch;
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch
// +kubebuilder:rbac:groups=config.openshift.io,resources=networks,verbs=get;list;watch

//go:embed manifests
var fs embed.FS

var log = logger.DefaultZapLogger()

const (
	NetworkPolicyDefaultDenyAll = "default-deny-all"
	NetworkPolicyAllowDNSAndAPI = "allow-dns-and-api"
	NetworkPolicyOperator       = "multicluster-global-hub-operator"
)

var watchedByoSecrets = sets.NewString(
	constants.GHStorageSecretName,
	constants.GHTransportSecretName,
)

type NetworkPolicyReconciler struct {
	ctrl.Manager
	kubeClient kubernetes.Interface
}

var (
	networkPolicyController     *NetworkPolicyReconciler
	networkPolicyWatchNamespace string
)

func isWatchedNetworkPolicy(obj client.Object) bool {
	ns := networkPolicyWatchNamespace
	if ns == "" {
		ns = commonutils.GetDefaultNamespace()
	}
	return obj.GetNamespace() == ns &&
		(obj.GetName() == NetworkPolicyDefaultDenyAll ||
			obj.GetName() == NetworkPolicyAllowDNSAndAPI ||
			obj.GetName() == NetworkPolicyOperator)
}

func StartController(initOption config.ControllerOption) (config.ControllerInterface, error) {
	if networkPolicyController != nil {
		return networkPolicyController, nil
	}
	log.Info("start networkpolicy controller")

	if initOption.MulticlusterGlobalHub != nil {
		networkPolicyWatchNamespace = initOption.MulticlusterGlobalHub.Namespace
	} else {
		networkPolicyWatchNamespace = commonutils.GetDefaultNamespace()
	}

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
		Watches(&corev1.Secret{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(byoSecretPred)).
		Watches(&networkingv1.NetworkPolicy{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(networkPolicyPred)).
		Complete(r)
}

func isWatchedByoSecret(obj client.Object) bool {
	ns := networkPolicyWatchNamespace
	if ns == "" {
		ns = commonutils.GetDefaultNamespace()
	}
	return obj.GetNamespace() == ns && watchedByoSecrets.Has(obj.GetName())
}

var byoSecretPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return isWatchedByoSecret(e.Object)
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return isWatchedByoSecret(e.ObjectNew)
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return isWatchedByoSecret(e.Object)
	},
}

var networkPolicyPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return isWatchedNetworkPolicy(e.Object)
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return isWatchedNetworkPolicy(e.ObjectNew)
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return isWatchedNetworkPolicy(e.Object)
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

	npValues := nputils.BuildBaselineValues(ctx, r.GetClient(), mgh.Namespace, config.COMPONENTS_POSTGRES_NAME)
	networkPolicyObjects, err := npRenderer.Render("manifests", "", func(profile string) (interface{}, error) {
		return npValues, nil
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
