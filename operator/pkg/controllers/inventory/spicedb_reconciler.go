package inventory

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/inventory/manifests"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// +kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=jobs,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=patch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=create;get;list;watch;delete;update
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=patch
// +kubebuilder:rbac:groups=authzed.com,resources=spicedbclusters,verbs=create;delete;get;list;patch;update;watch;deletecollection
// +kubebuilder:rbac:groups=authzed.com,resources=spicedbclusters/status,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=patch

// The manifests of the spicedb operator is from
// https://github.com/authzed/spicedb-operator/releases/download/v1.18.0/bundle.yaml
// It has defined some cluster scoped resources, such as: ClusterRole, ClusterRoleBinding, etc.

var spiceDBReconciler *SpiceDBReconciler

// TODO:
// 1. Currently, we use MGH to initialize the SpiceDB operator and cluster, applying customized configurations
// such as image pull secrets, node selectors, tolerations, etc. However, we have not applied a ResourceQuota to
// these components, as it may not be suitable for SpiceDB.
// 2. The preshared token used to connect to SpiceDB is hardcoded for now. It will be removed once we introduce
// TLS-based connections.
func StartSpiceDBReconciler(initOption config.ControllerOption) (config.ControllerInterface, error) {
	if !config.WithInventory(initOption.MulticlusterGlobalHub) {
		return nil, nil
	}
	if spiceDBReconciler != nil {
		return spiceDBReconciler, nil
	}
	if config.GetStorageConnection() == nil {
		return nil, nil
	}

	spiceDBCtrl := &SpiceDBReconciler{
		Manager: initOption.Manager,
	}
	if err := spiceDBCtrl.SetupWithManager(initOption.Manager); err != nil {
		spiceDBReconciler = nil
		return nil, err
	}
	spiceDBReconciler = spiceDBCtrl
	log.Info("start spiceDB controller")
	return spiceDBReconciler, nil
}

type SpiceDBReconciler struct {
	ctrl.Manager
}

func (r *SpiceDBReconciler) IsResourceRemoved() bool {
	return true
}

// SetupWithManager sets up the controller with the Manager.
func (r *SpiceDBReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("spicedb-reconciler").
		For(&v1alpha4.MulticlusterGlobalHub{}, builder.WithPredicates(config.MGHPred)).
		Watches(&appsv1.Deployment{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(spiceDBdeploymentPred)).
		Complete(r)
}

var spiceDBdeploymentPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.Object.GetName() == config.COMPONENTS_SPICEDB_OPERATOR_NAME
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.ObjectNew.GetName() == config.COMPONENTS_SPICEDB_OPERATOR_NAME
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.Object.GetName() == config.COMPONENTS_SPICEDB_OPERATOR_NAME
	},
}

func (r *SpiceDBReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Debugf("reconcile spicedb controller")
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.GetClient())
	if err != nil {
		log.Errorf("failed to get mgh, err:%v", err)
		return ctrl.Result{}, nil
	}

	// the secret, spicedb-operator and spicedbcluster all added ownerReference with mgh
	if mgh == nil || config.IsPaused(mgh) || mgh.DeletionTimestamp != nil || !config.WithInventory(mgh) {
		return ctrl.Result{}, nil
	}

	// create new HoHRenderer and HoHDeployer
	hohRenderer, hohDeployer := renderer.NewHoHRenderer(manifests.SpiceDBOperatorManifestFiles),
		deployer.NewHoHDeployer(r.GetClient())

	// create discovery client
	dc, err := discovery.NewDiscoveryClientForConfig(r.GetConfig())
	if err != nil {
		log.Errorf("failed to create discovery client: %v", err)
		return ctrl.Result{}, err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	operandConfig := config.GetOperandConfig(mgh)
	inventoryObjects, err := hohRenderer.Render("spicedb-operator", "", func(profile string) (interface{}, error) {
		return struct {
			Namespace       string
			Replicas        int32
			Image           string
			ImagePullPolicy string
			ImagePullSecret string
			NodeSelector    map[string]string
			Tolerations     []corev1.Toleration
		}{
			Namespace:       mgh.Namespace,
			Replicas:        operandConfig.Replicas,
			Image:           config.GetImage(config.SpiceDBOperatorImageKey),
			ImagePullPolicy: string(operandConfig.ImagePullPolicy),
			ImagePullSecret: operandConfig.ImagePullSecret,
			NodeSelector:    operandConfig.NodeSelector,
			Tolerations:     operandConfig.Tolerations,
		}, nil
	})
	if err != nil {
		log.Errorf("failed to render spicedb inventory objects: %v", err)
		return ctrl.Result{}, err
	}
	if err = utils.ManipulateGlobalHubObjects(inventoryObjects, mgh, hohDeployer, mapper, r.GetScheme()); err != nil {
		log.Errorf("failed to manipulate spicedb inventory objects: %v", err)
		return ctrl.Result{}, err
	}

	// start watch the resource spiceDBCluster
	if spiceDBInstanceReconciler == nil {
		reconciler, err := startSpiceDBController(r.Manager)
		if err != nil {
			return ctrl.Result{}, err
		}
		spiceDBInstanceReconciler = reconciler
	}
	return ctrl.Result{}, nil
}
