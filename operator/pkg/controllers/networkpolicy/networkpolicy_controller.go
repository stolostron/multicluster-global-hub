package networkpolicy

import (
	"context"
	"embed"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
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
	NetworkPolicyVolsyncMover   = "volsync-mover"

	volsyncGroupVersion         = "volsync.backube/v1alpha1"
	volsyncReplicationSourceCRD = "replicationsources.volsync.backube"
	networkPolicyManagedByLabel = "global-hub.open-cluster-management.io/managed-by"
	networkPolicyManagedByValue = "global-hub-operator"
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
			obj.GetName() == NetworkPolicyOperator ||
			obj.GetName() == NetworkPolicyVolsyncMover)
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
		WatchesMetadata(&apiextensionsv1.CustomResourceDefinition{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueForVolsyncCRD),
			builder.WithPredicates(volsyncCRDPred)).
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

	// Mover pods exist only after VolSync is installed. Skip the allow rule otherwise,
	// and remove it if VolSync was uninstalled after a previous reconcile.
	volsyncInstalled, err := volsyncAPIInstalled(ctx, r.kubeClient.Discovery())
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to check whether volsync is installed: %w", err)
	}
	if volsyncInstalled {
		log.Info("volsync is installed, ensuring volsync-mover networkpolicy")
	} else {
		networkPolicyObjects = withoutVolsyncMover(networkPolicyObjects)
		if err := r.deleteVolsyncMoverPolicy(ctx, mgh.Namespace); err != nil {
			return ctrl.Result{}, err
		}
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

func (r *NetworkPolicyReconciler) enqueueForVolsyncCRD(_ context.Context, obj client.Object) []ctrl.Request {
	if !isVolsyncReplicationSourceCRD(obj) {
		return nil
	}
	// Reconcile loads the hub and returns lookup errors, which are retried.
	// Do not drop the CRD event when that lookup fails.
	nn := config.GetMGHNamespacedName()
	if nn.Name == "" {
		nn = types.NamespacedName{
			Name:      "multiclusterglobalhub",
			Namespace: commonutils.GetDefaultNamespace(),
		}
	}
	return []ctrl.Request{{NamespacedName: nn}}
}

type groupVersionLister interface {
	RESTClient() rest.Interface
}

func volsyncAPIInstalled(ctx context.Context, d groupVersionLister) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	// DiscoveryClient.ServerResourcesForGroupVersion ignores the caller context, so
	// this helper issues the GET with the reconcile context.
	_, err := d.RESTClient().Get().AbsPath("/apis/" + volsyncGroupVersion).Do(ctx).Raw()
	return classifyVolsyncDiscovery(err)
}

func classifyVolsyncDiscovery(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	groupErr, ok := err.(*discovery.ErrGroupDiscoveryFailed)
	if !ok {
		return false, err
	}
	for gv := range groupErr.Groups {
		if gv.String() == volsyncGroupVersion {
			return false, nil
		}
	}
	return false, err
}

func withoutVolsyncMover(objects []*unstructured.Unstructured) []*unstructured.Unstructured {
	filtered := make([]*unstructured.Unstructured, 0, len(objects))
	for _, obj := range objects {
		if obj.GetKind() == "NetworkPolicy" && obj.GetName() == NetworkPolicyVolsyncMover {
			continue
		}
		filtered = append(filtered, obj)
	}
	return filtered
}

func (r *NetworkPolicyReconciler) deleteVolsyncMoverPolicy(ctx context.Context, namespace string) error {
	np := &networkingv1.NetworkPolicy{}
	key := types.NamespacedName{Name: NetworkPolicyVolsyncMover, Namespace: namespace}
	if err := r.GetClient().Get(ctx, key, np); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get volsync-mover networkpolicy: %w", err)
	}
	if np.Labels[networkPolicyManagedByLabel] != networkPolicyManagedByValue {
		return nil
	}
	preconditions := client.Preconditions{
		UID:             &np.UID,
		ResourceVersion: &np.ResourceVersion,
	}
	if err := r.GetClient().Delete(ctx, np, preconditions); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete volsync-mover networkpolicy: %w", err)
	}
	return nil
}

var volsyncCRDPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return isVolsyncReplicationSourceCRD(e.Object)
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return isVolsyncReplicationSourceCRD(e.ObjectNew)
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return isVolsyncReplicationSourceCRD(e.Object)
	},
}

func isVolsyncReplicationSourceCRD(obj client.Object) bool {
	return obj != nil && obj.GetName() == volsyncReplicationSourceCRD
}
