package inventory

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	spicedbv1alpha1 "github.com/authzed/spicedb-operator/pkg/apis/authzed/v1alpha1"
	"github.com/jackc/pgx/v5"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/inventory/manifests"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
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

var (
	spiceDBReconciler  *SpiceDBReconciler
	v1alpha1ClusterGVR = spicedbv1alpha1.SchemeGroupVersion.WithResource(spicedbv1alpha1.SpiceDBClusterResourceName)
)

const (
	SpiceDBConfigSecretName         = "spicedb-secret-config"
	SpiceDBConfigSecretURIKey       = "datastore_uri" // #nosec G101
	SpiceDBConfigSecretPreSharedKey = "preshared_key"
	SpiceDBConfigSecretPreSharedVal = "averysecretpresharedkey"
	SpiceDBConfigClusterName        = "spicedb"
)

func StartSpiceDBController(initOption config.ControllerOption) (config.ControllerInterface, error) {
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
	return ctrl.NewControllerManagedBy(mgr).Named("spicedb").
		For(&v1alpha4.MulticlusterGlobalHub{}, builder.WithPredicates(config.MGHPred)).
		Watches(&appsv1.Deployment{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(spiceDBdeploymentPred)).
		Watches(&corev1.Secret{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(spiceDBSecretPred)).
		Watches(&spicedbv1alpha1.SpiceDBCluster{}, &handler.EnqueueRequestForObject{},
			builder.WithPredicates(spiceDBClusterPred)).
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

var spiceDBSecretPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.Object.GetName() == SpiceDBConfigSecretName
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.ObjectNew.GetName() == SpiceDBConfigSecretName
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.Object.GetName() == SpiceDBConfigSecretName
	},
}

var spiceDBClusterPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.Object.GetName() == SpiceDBConfigClusterName
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.ObjectNew.GetName() == SpiceDBConfigClusterName
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.Object.GetName() == SpiceDBConfigClusterName
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
	if mgh == nil || config.IsPaused(mgh) || mgh.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	// create new HoHRenderer and HoHDeployer
	hohRenderer, hohDeployer := renderer.NewHoHRenderer(manifests.SpiceDBOperatorManifestFiles),
		deployer.NewHoHDeployer(r.GetClient())

	// create discovery client
	dc, err := discovery.NewDiscoveryClientForConfig(r.Manager.GetConfig())
	if err != nil {
		log.Errorf("failed to create discovery client: %v", err)
		return ctrl.Result{}, err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	imagePullPolicy := corev1.PullAlways
	if mgh.Spec.ImagePullPolicy != "" {
		imagePullPolicy = mgh.Spec.ImagePullPolicy
	}
	replicas := int32(1)
	if mgh.Spec.AvailabilityConfig == v1alpha4.HAHigh {
		replicas = 2
	}

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
			Replicas:        replicas,
			Image:           config.GetImage(config.SpiceDBOperatorImageKey),
			ImagePullPolicy: string(imagePullPolicy),
			ImagePullSecret: mgh.Spec.ImagePullSecret,
			NodeSelector:    mgh.Spec.NodeSelector,
			Tolerations:     mgh.Spec.Tolerations,
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

	return r.ReconcileCluster(ctx, mgh)
}

func (r *SpiceDBReconciler) ReconcileCluster(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub) (
	ctrl.Result, error,
) {
	storageConn := config.GetStorageConnection()
	if storageConn == nil {
		log.Info("the storage connection is not ready")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	pgConfig, err := pgx.ParseConfig(storageConn.SuperuserDatabaseURI)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to parse database uri: %w", err)
	}

	// Refer https://github.com/authzed/spicedb-operator/blob/main/examples/cockroachdb-tls-ingress/spicedb/spicedb.yaml
	pgURI := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		url.QueryEscape(pgConfig.User),
		url.QueryEscape(pgConfig.Password),
		pgConfig.Host,
		pgConfig.Port,
		InventoryDatabaseName,
		"disable",
	)
	spicedbConfigSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SpiceDBConfigSecretName,
			Namespace: mgh.Namespace,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
		},
		StringData: map[string]string{},
	}
	err = controllerutil.SetControllerReference(mgh, spicedbConfigSecret, r.GetScheme())
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set ownerReference: %w", err)
	}

	// reconcile secret
	err = r.GetClient().Get(ctx, client.ObjectKeyFromObject(spicedbConfigSecret), spicedbConfigSecret)
	if err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to get secret %s: %w", spicedbConfigSecret.Name, err)
	} else if errors.IsNotFound(err) {
		spicedbConfigSecret.StringData[SpiceDBConfigSecretURIKey] = pgURI
		// TODO: remove the hardcode
		spicedbConfigSecret.StringData[SpiceDBConfigSecretPreSharedKey] = SpiceDBConfigSecretPreSharedVal
		if err = r.GetClient().Create(ctx, spicedbConfigSecret); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create secret %s: %w", spicedbConfigSecret.Name, err)
		}
	} else {
		if spicedbConfigSecret.StringData[SpiceDBConfigSecretURIKey] != pgURI ||
			spicedbConfigSecret.StringData[SpiceDBConfigSecretPreSharedKey] != SpiceDBConfigSecretPreSharedVal {
			if err = r.GetClient().Update(ctx, spicedbConfigSecret); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update secret %s: %w", spicedbConfigSecret.Name, err)
			}
		}
	}
	expectedCluster, err := getSpiceDBCluster(mgh)
	if err != nil {
		return ctrl.Result{}, err
	}
	err = controllerutil.SetControllerReference(mgh, expectedCluster, r.GetScheme())
	if err != nil {
		return ctrl.Result{}, err
	}
	currentCluster := &spicedbv1alpha1.SpiceDBCluster{}
	err = r.GetClient().Get(ctx, client.ObjectKeyFromObject(expectedCluster), currentCluster)
	if err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed get spicedb cluster instance: %w", err)
	} else if errors.IsNotFound(err) {
		// create
		err = r.GetClient().Create(ctx, expectedCluster)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create spicedb cluster: %w", err)
		}
		log.Infof("spicedb cluster is created %s", expectedCluster.Name)
		return ctrl.Result{}, nil
	}

	// update
	if !equality.Semantic.DeepEqual(currentCluster.Labels, expectedCluster.Labels) ||
		!equality.Semantic.DeepEqual(currentCluster.Spec, expectedCluster.Spec) {
		currentCluster.Labels = expectedCluster.Labels
		currentCluster.Spec = expectedCluster.Spec
		err = r.GetClient().Update(ctx, currentCluster)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create spicedb cluster: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

func getSpiceDBCluster(mgh *v1alpha4.MulticlusterGlobalHub) (*spicedbv1alpha1.SpiceDBCluster, error) {
	replicas := int32(1)
	if mgh.Spec.AvailabilityConfig == v1alpha4.HAHigh {
		replicas = 2
	}

	// create spicedb cluster
	configData := map[string]interface{}{
		"replicas":        replicas,
		"datastoreEngine": "postgres",
		"image":           config.GetImage(config.SpiceDBInstanceImageKey),
	}

	imagePullPolicy := corev1.PullAlways
	if mgh.Spec.ImagePullPolicy != "" {
		imagePullPolicy = mgh.Spec.ImagePullPolicy
	}
	imagePullSecret := mgh.Spec.ImagePullSecret

	configJSON, err := json.Marshal(configData)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling spicedb config: %w", err)
	}
	spicedbCluster := spicedbv1alpha1.SpiceDBCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: mgh.Namespace,
			Name:      SpiceDBConfigClusterName,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
		},
		Spec: spicedbv1alpha1.ClusterSpec{
			SecretRef: SpiceDBConfigSecretName,
			Config:    configJSON,
			Patches:   make([]spicedbv1alpha1.Patch, 0),
		},
	}
	if imagePullSecret != "" {
		pullSecretPatch := spicedbv1alpha1.Patch{
			Kind: "Deployment",
			Patch: json.RawMessage(`{
                "op": "replace",
                "path": "/spec/template/spec/imagePullSecrets",
                "value": [
                    {
                        "name": "` + imagePullSecret + `"
                    }
                ]
            }`),
		}
		spicedbCluster.Spec.Patches = append(spicedbCluster.Spec.Patches, pullSecretPatch)
	}
	if imagePullPolicy != "" {
		// JSON6902 Patch
		pullPolicyPatch := spicedbv1alpha1.Patch{
			Kind: "Deployment",
			Patch: json.RawMessage(`{
                "op": "replace",
                "path": "/spec/template/spec/containers/0/imagePullPolicy",
                "value": "` + imagePullPolicy + `"
            }`),
		}
		spicedbCluster.Spec.Patches = append(spicedbCluster.Spec.Patches, pullPolicyPatch)
	}

	if len(mgh.Spec.Tolerations) > 0 {
		bytes, err := json.Marshal(mgh.Spec.Tolerations)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tolerations: %w", err)
		}
		tolerationsPatch := spicedbv1alpha1.Patch{
			Kind: "Deployment",
			Patch: json.RawMessage(`{
							"op": "replace",
							"path": "/spec/template/spec/tolerations",
							"value": ` + string(bytes) + `
					}
			`),
		}
		spicedbCluster.Spec.Patches = append(spicedbCluster.Spec.Patches, tolerationsPatch)
	}

	if len(mgh.Spec.NodeSelector) > 0 {
		bytes, err := json.Marshal(mgh.Spec.NodeSelector)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal nodeSelector: %w", err)
		}
		nodeSelectorPatch := spicedbv1alpha1.Patch{
			Kind: "Deployment",
			Patch: json.RawMessage(`[
            {
                "op": "replace",
                "path": "/spec/template/spec/nodeSelector",
                "value": ` + string(bytes) + `
            }
        ]`),
		}
		spicedbCluster.Spec.Patches = append(spicedbCluster.Spec.Patches, nodeSelectorPatch)
	}
	return &spicedbCluster, nil
}
