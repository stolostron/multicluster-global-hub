package inventory

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	spicedbv1alpha1 "github.com/authzed/spicedb-operator/pkg/apis/authzed/v1alpha1"
	"github.com/jackc/pgx/v5"
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

// spiceDBInstanceReconciler is to reconcile the cluster
var spiceDBInstanceReconciler *spiceDBClusterReconciler

const (
	SpiceDBConfigSecretName         = "spicedb-secret-config"
	SpiceDBConfigSecretURIKey       = "datastore_uri" // #nosec G101
	SpiceDBConfigSecretPreSharedKey = "preshared_key"
	SpiceDBConfigSecretPreSharedVal = "averysecretpresharedkey"
	SpiceDBConfigClusterName        = "spicedb"
)

type spiceDBClusterReconciler struct {
	ctrl.Manager
}

// SetupWithManager sets up the controller with the Manager.
func startSpiceDBController(mgr ctrl.Manager) (*spiceDBClusterReconciler, error) {
	reconciler := &spiceDBClusterReconciler{
		Manager: mgr,
	}
	err := ctrl.NewControllerManagedBy(mgr).Named("spicedb-cluster").
		For(&v1alpha4.MulticlusterGlobalHub{}, builder.WithPredicates(config.MGHPred)).
		Owns(&corev1.Secret{}, builder.WithPredicates(spiceDBSecretPred)).
		Owns(&spicedbv1alpha1.SpiceDBCluster{}, builder.WithPredicates(spiceDBClusterPred)).
		Complete(reconciler)
	if err != nil {
		return nil, fmt.Errorf("failed to start the spicedb cluster %w", err)
	}
	return reconciler, nil
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

func (r *spiceDBClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (
	ctrl.Result, error,
) {
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.GetClient())
	if err != nil {
		log.Errorf("failed to get mgh, err:%v", err)
		return ctrl.Result{}, nil
	}
	if mgh == nil || config.IsPaused(mgh) || mgh.DeletionTimestamp != nil || !config.WithInventory(mgh) {
		return ctrl.Result{}, nil
	}

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
	// TODO: Currently using the 'disable' method to establish the connection.
	// Other methods have not been validated. GH itself uses `required-ca`,
	// so we might need to update both to support `verify-full`.
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

	// relations api
	operandConfig := config.GetOperandConfig(mgh)
	hohRenderer, hohDeployer := renderer.NewHoHRenderer(manifests.RelationsAPIFiles),
		deployer.NewHoHDeployer(r.GetClient())
	dc, err := discovery.NewDiscoveryClientForConfig(r.Manager.GetConfig())
	if err != nil {
		log.Errorf("failed to create discovery client: %v", err)
		return ctrl.Result{}, err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	relationsAPIObjects, err := hohRenderer.Render("relations-api", "", func(profile string) (interface{}, error) {
		return struct {
			Namespace                       string
			Replicas                        int32
			Image                           string
			ImagePullPolicy                 string
			ImagePullSecret                 string
			NodeSelector                    map[string]string
			Tolerations                     []corev1.Toleration
			SpiceDBConfigClusterName        string
			SpiceDBConfigSecretName         string
			SpiceDBConfigSecretPreSharedKey string
		}{
			Namespace:                       mgh.Namespace,
			Replicas:                        operandConfig.Replicas,
			Image:                           config.GetImage(config.SpiceDBRelationsAPIImageKey),
			ImagePullPolicy:                 string(operandConfig.ImagePullPolicy),
			ImagePullSecret:                 operandConfig.ImagePullSecret,
			NodeSelector:                    operandConfig.NodeSelector,
			Tolerations:                     operandConfig.Tolerations,
			SpiceDBConfigClusterName:        SpiceDBConfigClusterName,
			SpiceDBConfigSecretName:         SpiceDBConfigSecretName,
			SpiceDBConfigSecretPreSharedKey: SpiceDBConfigSecretPreSharedKey,
		}, nil
	})
	if err != nil {
		log.Errorf("failed to render spicedb relations api objects: %v", err)
		return ctrl.Result{}, err
	}
	if err = utils.ManipulateGlobalHubObjects(relationsAPIObjects, mgh, hohDeployer, mapper, r.GetScheme()); err != nil {
		log.Errorf("failed to manipulate spicedb realtions api objects: %v", err)
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func getSpiceDBCluster(mgh *v1alpha4.MulticlusterGlobalHub) (*spicedbv1alpha1.SpiceDBCluster, error) {
	operandConfig := config.GetOperandConfig(mgh)

	// create spicedb cluster
	configData := map[string]interface{}{
		"replicas":        operandConfig.Replicas,
		"datastoreEngine": "postgres",
		"image":           config.GetImage(config.SpiceDBInstanceImageKey),
	}

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
	if operandConfig.ImagePullSecret != "" {
		pullSecretPatch := spicedbv1alpha1.Patch{
			Kind: "Deployment",
			Patch: json.RawMessage(`{
					"op": "replace",
					"path": "/spec/template/spec/imagePullSecrets",
					"value": [
							{
									"name": "` + operandConfig.ImagePullSecret + `"
							}
					]
				}`),
		}
		spicedbCluster.Spec.Patches = append(spicedbCluster.Spec.Patches, pullSecretPatch)
	}
	if operandConfig.ImagePullPolicy != "" {
		// JSON6902 Patch
		pullPolicyPatch := spicedbv1alpha1.Patch{
			Kind: "Deployment",
			Patch: json.RawMessage(`{
					"op": "replace",
					"path": "/spec/template/spec/containers/0/imagePullPolicy",
					"value": "` + operandConfig.ImagePullPolicy + `"
				}`),
		}
		spicedbCluster.Spec.Patches = append(spicedbCluster.Spec.Patches, pullPolicyPatch)
	}

	if len(operandConfig.Tolerations) > 0 {
		bytes, err := json.Marshal(operandConfig.Tolerations)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tolerations: %w", err)
		}
		tolerationsPatch := spicedbv1alpha1.Patch{
			Kind: "Deployment",
			Patch: json.RawMessage(fmt.Sprintf(`{
					"op": "replace",
					"path": "/spec/template/spec/tolerations",
					"value": %s
				}
			`, string(bytes))),
		}
		spicedbCluster.Spec.Patches = append(spicedbCluster.Spec.Patches, tolerationsPatch)
	}

	if len(operandConfig.NodeSelector) > 0 {
		bytes, err := json.Marshal(operandConfig.NodeSelector)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal nodeSelector: %w", err)
		}
		nodeSelectorPatch := spicedbv1alpha1.Patch{
			Kind: "Deployment",
			Patch: json.RawMessage(fmt.Sprintf(`{
					"op": "replace",
					"path": "/spec/template/spec/nodeSelector",
					"value": %s
				}`, string(bytes))),
		}
		spicedbCluster.Spec.Patches = append(spicedbCluster.Spec.Patches, nodeSelectorPatch)
	}
	return &spicedbCluster, nil
}
