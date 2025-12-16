package inventory

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"reflect"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	certctrl "github.com/stolostron/multicluster-global-hub/operator/pkg/certificates"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/inventory/manifests"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs,verbs=get;list;watch;
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;delete

var (
	log                 = logger.DefaultZapLogger()
	inventoryReconciler *InventoryReconciler
)

const InventoryDatabaseName = "inventory"

func StartInventoryController(initOption config.ControllerOption) (config.ControllerInterface, error) {
	if inventoryReconciler != nil {
		return inventoryReconciler, nil
	}
	if !config.WithInventory(initOption.MulticlusterGlobalHub) {
		return nil, nil
	}
	if config.GetStorageConnection() == nil {
		return nil, nil
	}
	log.Info("start inventory controller")

	inventoryReconciler = NewInventoryReconciler(initOption.Manager,
		initOption.KubeClient)
	err := inventoryReconciler.SetupWithManager(initOption.Manager)
	if err != nil {
		inventoryReconciler = nil
		return nil, err
	}
	log.Infof("inited inventory controller")
	return inventoryReconciler, nil
}

type InventoryReconciler struct {
	kubeClient kubernetes.Interface
	ctrl.Manager
}

func (r *InventoryReconciler) IsResourceRemoved() bool {
	return true
}

// SetupWithManager sets up the controller with the Manager.
func (r *InventoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("inventory").
		For(&globalhubv1alpha4.MulticlusterGlobalHub{},
			builder.WithPredicates(config.MGHPred)).
		Watches(&appsv1.Deployment{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(deploymentPred)).
		Watches(&corev1.Secret{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Watches(&corev1.Service{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Watches(&corev1.ServiceAccount{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Complete(r)
}

var deploymentPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.Object.GetName() == config.COMPONENTS_INVENTORY_API_NAME
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.ObjectNew.GetName() == config.COMPONENTS_INVENTORY_API_NAME
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.Object.GetName() == config.COMPONENTS_INVENTORY_API_NAME
	},
}

func NewInventoryReconciler(mgr ctrl.Manager, kubeClient kubernetes.Interface) *InventoryReconciler {
	return &InventoryReconciler{
		Manager:    mgr,
		kubeClient: kubeClient,
	}
}

func (r *InventoryReconciler) Reconcile(ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	log.Debugf("reconcile inventory controller")
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.GetClient())
	if err != nil {
		return ctrl.Result{}, nil
	}
	if mgh == nil || config.IsPaused(mgh) || mgh.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	var reconcileErr error

	defer func() {
		err = config.UpdateMGHComponent(ctx, r.GetClient(),
			config.GetComponentStatusWithReconcileError(ctx, r.GetClient(),
				mgh.Namespace, config.COMPONENTS_INVENTORY_API_NAME, reconcileErr),
			false,
		)
		if err != nil {
			log.Errorf("failed to update mgh status, err:%v", err)
		}
	}()
	// start certificate controller
	certctrl.Start(ctx, r.GetClient(), r.kubeClient)

	// Need to create route so that the cert can use it
	if reconcileErr = createUpdateInventoryRoute(ctx, r.GetClient(), r.GetScheme(), mgh); reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}

	// create inventory certs
	if reconcileErr = certctrl.CreateInventoryCerts(ctx, r.GetClient(), r.GetScheme(), mgh); reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}

	// set the client ca to signing the inventory client cert
	if reconcileErr = config.SetInventoryClientCA(ctx, mgh.Namespace, certctrl.InventoryClientCASecretName,
		r.GetClient()); reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}

	// create new HoHRenderer and HoHDeployer
	hohRenderer, hohDeployer := renderer.NewHoHRenderer(manifests.InventoryManifestFiles),
		deployer.NewHoHDeployer(r.GetClient())

	// create discovery client
	dc, err := discovery.NewDiscoveryClientForConfig(r.GetConfig())
	if err != nil {
		reconcileErr = err
		return ctrl.Result{}, reconcileErr
	}

	// create restmapper for deployer to find GVR
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	imagePullPolicy := corev1.PullAlways
	if mgh.Spec.ImagePullPolicy != "" {
		imagePullPolicy = mgh.Spec.ImagePullPolicy
	}

	replicas := int32(1)
	if mgh.Spec.AvailabilityConfig == globalhubv1alpha4.HAHigh {
		replicas = 2
	}

	storageConn := config.GetStorageConnection()
	if storageConn == nil || !config.GetDatabaseReady() {
		reconcileErr = fmt.Errorf("the database isn't ready")
		return ctrl.Result{}, reconcileErr
	}

	postgresURI, err := url.Parse(string(storageConn.SuperuserDatabaseURI))
	if err != nil {
		reconcileErr = err
		return ctrl.Result{}, reconcileErr
	}
	postgresPassword, ok := postgresURI.User.Password()
	if !ok {
		reconcileErr = fmt.Errorf("failed to get password from database_uri: %s", postgresURI)
		return ctrl.Result{}, reconcileErr
	}

	if !config.IsTransportConfigReady(ctx, mgh.Namespace, r.GetClient()) {
		log.Info("Waiting for transport-config secret to be created by transporter controller")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	transportConfigSecret := &corev1.Secret{}
	if err = r.GetClient().Get(ctx, types.NamespacedName{
		Name:      constants.GHTransportConfigSecret,
		Namespace: mgh.Namespace,
	}, transportConfigSecret); err != nil {
		log.Infof("Failed to get transport-config secret, will retry: %v", err)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	kafkaConfig, err := commonutils.GetKafkaCredentialBySecret(transportConfigSecret, r.GetClient())
	if err != nil {
		log.Infof("Failed to parse kafka config from transport-config secret, will retry: %v", err)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	inventoryRoute := &routev1.Route{}
	if reconcileErr = r.GetClient().Get(ctx, types.NamespacedName{
		Name:      constants.InventoryRouteName,
		Namespace: mgh.Namespace,
	}, inventoryRoute); reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}

	inventoryObjects, err := hohRenderer.Render("inventory-api", "", func(profile string) (interface{}, error) {
		return struct {
			Image                 string
			Replicas              int32
			ImagePullSecret       string
			ImagePullPolicy       string
			PostgresHost          string
			PostgresPort          string
			PostgresUser          string
			PostgresPassword      string
			PostgresCACert        string
			PostgresDBName        string
			Namespace             string
			NodeSelector          map[string]string
			Tolerations           []corev1.Toleration
			KafkaBootstrapServer  string
			KafkaSSLCAPEM         string
			KafkaSSLCertPEM       string
			KafkaSSLKeyPEM        string
			InventoryAPIRouteHost string
		}{
			Image:                 config.GetImage(config.InventoryImageKey),
			Replicas:              replicas,
			ImagePullSecret:       mgh.Spec.ImagePullSecret,
			ImagePullPolicy:       string(imagePullPolicy),
			PostgresHost:          postgresURI.Hostname(),
			PostgresPort:          postgresURI.Port(),
			PostgresUser:          postgresURI.User.Username(),
			PostgresPassword:      postgresPassword,
			PostgresDBName:        InventoryDatabaseName,
			PostgresCACert:        base64.StdEncoding.EncodeToString(storageConn.CACert),
			Namespace:             mgh.Namespace,
			NodeSelector:          mgh.Spec.NodeSelector,
			Tolerations:           mgh.Spec.Tolerations,
			KafkaBootstrapServer:  kafkaConfig.BootstrapServer,
			KafkaSSLCAPEM:         base64.StdEncoding.EncodeToString([]byte(kafkaConfig.CACert)),
			KafkaSSLCertPEM:       base64.StdEncoding.EncodeToString([]byte(kafkaConfig.ClientCert)),
			KafkaSSLKeyPEM:        base64.StdEncoding.EncodeToString([]byte(kafkaConfig.ClientKey)),
			InventoryAPIRouteHost: fmt.Sprintf("https://%s", inventoryRoute.Spec.Host),
		}, nil
	})
	if err != nil {
		reconcileErr = fmt.Errorf("failed to render inventory objects: %v", err)
		return ctrl.Result{}, reconcileErr
	}
	if err = utils.ManipulateGlobalHubObjects(inventoryObjects, mgh, hohDeployer, mapper, r.GetScheme()); err != nil {
		reconcileErr = fmt.Errorf("failed to create/update inventory objects: %v", err)
		return ctrl.Result{}, reconcileErr
	}
	return ctrl.Result{}, nil
}

func newInventoryRoute(mgh *globalhubv1alpha4.MulticlusterGlobalHub, gvk schema.GroupVersionKind) *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.InventoryRouteName,
			Namespace: mgh.Namespace,
			Labels: map[string]string{
				"name": constants.InventoryRouteName,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         gvk.GroupVersion().String(),
					Kind:               gvk.Kind,
					Name:               mgh.GetName(),
					UID:                mgh.GetUID(),
					BlockOwnerDeletion: ptr.To(true),
					Controller:         ptr.To(true),
				},
			},
		},
		Spec: routev1.RouteSpec{
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromString("http-server"),
			},
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: constants.InventoryRouteName,
			},
			TLS: &routev1.TLSConfig{
				Termination:                   routev1.TLSTerminationPassthrough,
				InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyNone,
			},
		},
	}
}

func createUpdateInventoryRoute(ctx context.Context, c client.Client,
	scheme *runtime.Scheme, mgh *globalhubv1alpha4.MulticlusterGlobalHub,
) error {
	// finds the GroupVersionKind associated with mgh
	gvk, err := apiutil.GVKForObject(mgh, scheme)
	if err != nil {
		return err
	}
	desiredRoute := newInventoryRoute(mgh, gvk)

	existingRoute := &routev1.Route{}
	err = c.Get(ctx, types.NamespacedName{
		Name:      constants.InventoryRouteName,
		Namespace: mgh.Namespace,
	}, existingRoute)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.Create(ctx, desiredRoute)
		}
		return err
	}

	updatedRoute := &routev1.Route{}
	err = utils.MergeObjects(existingRoute, desiredRoute, updatedRoute)
	if err != nil {
		return err
	}

	if !reflect.DeepEqual(updatedRoute.Spec, existingRoute.Spec) {
		return c.Update(ctx, updatedRoute)
	}

	return nil
}
