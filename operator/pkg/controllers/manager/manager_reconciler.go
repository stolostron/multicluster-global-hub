package manager

import (
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"reflect"
	"strconv"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"open-cluster-management.io/api/addon/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/certificates"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs,verbs=get;list;watch;
// +kubebuilder:rbac:groups="route.openshift.io",resources=routes,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterrolebindings,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;update;get;list;watch;delete;deletecollection;patch
// +kubebuilder:rbac:groups="admissionregistration.k8s.io",resources=mutatingwebhookconfigurations,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors;prometheusrules;podmonitors,verbs=get;create;delete;update;list;watch
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=clustermanagementaddons,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=app.k8s.io,resources=applications,verbs=get;list;patch;update
// +kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=subscriptions,verbs=get;list;update;patch
// +kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=placementrules,verbs=get;list;update;patch
// +kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=channels,verbs=get;list;update;patch
// +kubebuilder:rbac:groups="config.open-cluster-management.io",resources=klusterletconfigs,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersetbindings,verbs=create;get;list;patch;update;delete
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersets,verbs=get;list;patch;update
// +kubebuilder:rbac:groups="authentication.open-cluster-management.io",resources=managedserviceaccounts,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="global-hub.open-cluster-management.io",resources=managedclustermigrations,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups="global-hub.open-cluster-management.io",resources=managedclustermigrations/status,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=policy.open-cluster-management.io,resources=placementbindings,verbs=get;list;patch;update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=kafka.strimzi.io,resources=kafkausers,verbs=get;watch;update

//go:embed manifests
var fs embed.FS

var log = logger.DefaultZapLogger()

var storageConnectionCache *config.PostgresConnection

type ManagerReconciler struct {
	ctrl.Manager
	kubeClient     kubernetes.Interface
	operatorConfig *config.OperatorConfig
}

var (
	managerController *ManagerReconciler
	isResourceRemoved = true
)

func StartController(initOption config.ControllerOption) (config.ControllerInterface, error) {
	if managerController != nil {
		return managerController, nil
	}
	log.Info("start manager controller")

	if config.GetTransporterConn() == nil {
		return nil, nil
	}
	if config.GetStorageConnection() == nil {
		return nil, nil
	}
	if config.WithInventory(initOption.MulticlusterGlobalHub) {
		inventoryComponents, ok := initOption.MulticlusterGlobalHub.Status.Components[config.COMPONENTS_INVENTORY_API_NAME]
		if !ok {
			log.Infof("wait inventory ready")
			return nil, nil
		}
		if inventoryComponents.Type != config.COMPONENTS_AVAILABLE ||
			inventoryComponents.Status != config.CONDITION_STATUS_TRUE {
			log.Infof("wait inventory ready")
			return nil, nil
		}
	}
	managerController = NewManagerReconciler(initOption.Manager,
		initOption.KubeClient, initOption.OperatorConfig)

	err := managerController.SetupWithManager(initOption.Manager)
	if err != nil {
		managerController = nil
		return managerController, err
	}
	log.Infof("inited manager controller")
	return managerController, nil
}

func (r *ManagerReconciler) IsResourceRemoved() bool {
	log.Infof("managerController resource removed: %v", isResourceRemoved)
	return isResourceRemoved
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mgrBuilder := ctrl.NewControllerManagedBy(mgr).Named("manager").
		For(&v1alpha4.MulticlusterGlobalHub{},
			builder.WithPredicates(config.MGHPred)).
		Watches(&appsv1.Deployment{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(deploymentPred)).
		Watches(&corev1.Service{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Watches(&corev1.ServiceAccount{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Watches(&rbacv1.ClusterRole{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Watches(&rbacv1.ClusterRoleBinding{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Watches(&rbacv1.Role{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Watches(&rbacv1.RoleBinding{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Watches(&routev1.Route{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate))

	if config.IsACMResourceReady() {
		mgrBuilder = mgrBuilder.
			Watches(&v1alpha1.ClusterManagementAddOn{},
				&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate))
	}
	return mgrBuilder.Complete(r)
}

var deploymentPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.Object.GetName() == config.COMPONENTS_MANAGER_NAME
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.ObjectNew.GetName() == config.COMPONENTS_MANAGER_NAME
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.Object.GetName() == config.COMPONENTS_MANAGER_NAME
	},
}

func NewManagerReconciler(mgr ctrl.Manager, kubeClient kubernetes.Interface,
	conf *config.OperatorConfig,
) *ManagerReconciler {
	return &ManagerReconciler{
		Manager:        mgr,
		kubeClient:     kubeClient,
		operatorConfig: conf,
	}
}

func (r *ManagerReconciler) Reconcile(ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	log.Debug("reconcile manager controller")
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.GetClient())
	if err != nil {
		return ctrl.Result{}, nil
	}
	if mgh == nil || config.IsPaused(mgh) {
		return ctrl.Result{}, nil
	}
	if mgh.DeletionTimestamp != nil {
		log.Debug("deleteting mgh in manager reconciler")
		err = utils.HandleMghDelete(ctx, &isResourceRemoved, mgh.Namespace, r.pruneResources)
		log.Debug("deleted mgh in manager reconciler, isResourceRemoved:%v", isResourceRemoved)
		return ctrl.Result{}, err
	}
	isResourceRemoved = false
	var reconcileErr error
	defer func() {
		// Skip status update if MGH was deleted during reconciliation
		if mgh == nil {
			return
		}
		err = config.UpdateMGHComponent(ctx, r.GetClient(),
			config.GetComponentStatusWithReconcileError(ctx, r.GetClient(),
				mgh.Namespace, config.COMPONENTS_MANAGER_NAME, reconcileErr),
			false,
		)
		if err != nil {
			log.Errorf("failed to update mgh status, err:%v", err)
		}
	}()

	// generate random session secret for oauth-proxy
	proxySessionSecret, err := config.GetOauthSessionSecret()
	if err != nil {
		reconcileErr = fmt.Errorf("failed to get random session secret for oauth-proxy: %v", err)
		return ctrl.Result{}, reconcileErr
	}

	// create new HoHRenderer and HoHDeployer
	hohRenderer, hohDeployer := renderer.NewHoHRenderer(fs), deployer.NewHoHDeployer(r.GetClient())

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

	// dataRetention should at least be 1 month, otherwise it will deleted the current month partitions and records
	months, err := commonutils.ParseRetentionMonth(mgh.Spec.DataLayerSpec.Postgres.Retention)
	if err != nil {
		reconcileErr = fmt.Errorf("failed to parse month retention: %v", err)
		return ctrl.Result{}, reconcileErr
	}
	if months < 1 {
		months = 1
	}

	replicas := int32(1)
	if mgh.Spec.AvailabilityConfig == v1alpha4.HAHigh {
		replicas = 2
	}

	kafkaConfig := config.GetTransporterConn()
	if kafkaConfig == nil || kafkaConfig.BootstrapServer == "" {
		log.Debug("Wait kafka connection created")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	kafkaConfig.ConsumerGroupID = config.GetManagerConsumerGroupID(mgh)

	storageConn := config.GetStorageConnection()
	if storageConn == nil || !config.GetDatabaseReady() {
		reconcileErr = fmt.Errorf("the storage connection or database isn't ready")
		return ctrl.Result{}, reconcileErr
	}

	if storageConnectionUpdated(storageConn) {
		log.Infof("restarting manager pod")
		err = commonutils.RestartPod(ctx, r.kubeClient, mgh.Namespace, constants.ManagerDeploymentName)
		if err != nil {
			reconcileErr = fmt.Errorf("failed to restart manager pod: %w", err)
			return ctrl.Result{}, reconcileErr
		}
	}
	electionConfig, err := config.GetElectionConfig()
	if err != nil {
		reconcileErr = fmt.Errorf("failed to get the electionConfig %w", err)
		return ctrl.Result{}, reconcileErr
	}

	kafkaConfigYaml, err := kafkaConfig.YamlMarshal(true)
	if err != nil {
		reconcileErr = fmt.Errorf("failed to marshall kafka connetion for config: %w", err)
		return ctrl.Result{}, reconcileErr
	}
	var inventoryConfigYaml []byte
	if config.WithInventory(mgh) {
		inventoryConn, reconcileErr := certificates.GetInventoryCredential(r.GetClient())
		if reconcileErr != nil {
			return ctrl.Result{}, reconcileErr
		}
		inventoryConfigYaml, err = inventoryConn.YamlMarshal(true)
		if err != nil {
			reconcileErr = fmt.Errorf("failed to marshalling the inventory config yaml: %w", err)
			return ctrl.Result{}, reconcileErr
		}
	}

	log.Infof("transport-config updating: manager controller reconcile the transportConfig secret")
	managerObjects, err := hohRenderer.Render("manifests", "", func(profile string) (interface{}, error) {
		return ManagerVariables{
			Image:              config.GetImage(config.GlobalHubManagerImageKey),
			Replicas:           replicas,
			ProxyImage:         config.GetImage(config.OauthProxyImageKey),
			ImagePullSecret:    mgh.Spec.ImagePullSecret,
			ImagePullPolicy:    string(imagePullPolicy),
			ProxySessionSecret: proxySessionSecret,
			DatabaseURL: base64.StdEncoding.EncodeToString(
				[]byte(storageConn.SuperuserDatabaseURI)),
			PostgresCACert:            base64.StdEncoding.EncodeToString(storageConn.CACert),
			TransportType:             string(transport.Kafka),
			TransportConfigSecret:     constants.GHTransportConfigSecret,
			StorageConfigSecret:       constants.GHStorageConfigSecret,
			KafkaConfigYaml:           base64.StdEncoding.EncodeToString(kafkaConfigYaml),
			InventoryConfigYaml:       base64.StdEncoding.EncodeToString(inventoryConfigYaml),
			Namespace:                 mgh.Namespace,
			LeaseDuration:             strconv.Itoa(electionConfig.LeaseDuration),
			RenewDeadline:             strconv.Itoa(electionConfig.RenewDeadline),
			RetryPeriod:               strconv.Itoa(electionConfig.RetryPeriod),
			SchedulerInterval:         config.GetSchedulerInterval(mgh),
			SkipAuth:                  config.SkipAuth(mgh),
			LaunchJobNames:            config.GetLaunchJobNames(mgh),
			NodeSelector:              mgh.Spec.NodeSelector,
			Tolerations:               mgh.Spec.Tolerations,
			RetentionMonth:            months,
			StatisticLogInterval:      config.GetStatisticLogInterval(),
			EnableGlobalResource:      r.operatorConfig.GlobalResourceEnabled,
			EnableInventoryAPI:        config.WithInventory(mgh),
			EnablePprof:               r.operatorConfig.EnablePprof,
			Resources:                 utils.GetResources(operatorconstants.Manager, mgh.Spec.AdvancedSpec),
			WithACM:                   config.IsACMResourceReady(),
			TransportFailureThreshold: r.operatorConfig.TransportFailureThreshold,
		}, nil
	})
	if err != nil {
		reconcileErr = fmt.Errorf("failed to render manager objects: %v", err)
		return ctrl.Result{}, reconcileErr
	}
	if err = utils.ManipulateGlobalHubObjects(managerObjects, mgh, hohDeployer, mapper, r.GetScheme()); err != nil {
		reconcileErr = fmt.Errorf("failed to create/update manager objects: %v", err)
		return ctrl.Result{}, reconcileErr
	}
	return r.setUpMetrics(ctx, mgh)
}

func (r *ManagerReconciler) pruneResources(ctx context.Context, namespace string) error {
	// Remove the migrations if exists
	mcms := &migrationv1alpha1.ManagedClusterMigrationList{}
	if err := r.GetClient().List(ctx, mcms, client.InNamespace(namespace)); err != nil {
		return err
	}
	if len(mcms.Items) > 0 {
		for _, mcm := range mcms.Items {
			if err := r.GetClient().Delete(ctx, &mcm, &client.DeleteOptions{}); err != nil {
				return err
			}
		}
		log.Info("removing the migration resources")
	}

	// Remove ServiceMonitor regardless of migration existence
	mghServiceMonitor := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconstants.GHServiceMonitorName,
			Namespace: namespace,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
		},
	}
	if err := r.GetClient().Delete(ctx, mghServiceMonitor); err != nil && !errors.IsNotFound(err) {
		return err
	}

	return nil
}

func (r *ManagerReconciler) setUpMetrics(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub) (ctrl.Result, error) {
	// add label openshift.io/cluster-monitoring: "true" to the ns, so that the prometheus can detect the ServiceMonitor.
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: mgh.Namespace,
		},
	}
	if err := r.GetClient().Get(ctx, client.ObjectKeyFromObject(namespace), namespace); err != nil {
		return ctrl.Result{}, err
	}
	labels := namespace.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	val, ok := labels[operatorconstants.ClusterMonitoringLabelKey]
	if !ok || val != operatorconstants.ClusterMonitoringLabelVal {
		labels[operatorconstants.ClusterMonitoringLabelKey] = operatorconstants.ClusterMonitoringLabelVal
	}
	namespace.SetLabels(labels)
	if err := r.GetClient().Update(ctx, namespace); err != nil {
		return ctrl.Result{}, err
	}

	// create ServiceMonitor under global hub namespace
	expectedServiceMonitor := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconstants.GHServiceMonitorName,
			Namespace: mgh.Namespace,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
		},
		Spec: promv1.ServiceMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": "multicluster-global-hub-manager",
				},
			},
			NamespaceSelector: promv1.NamespaceSelector{
				MatchNames: []string{
					mgh.Namespace,
				},
			},
			Endpoints: []promv1.Endpoint{
				{
					Port:     "metrics",
					Path:     "/metrics",
					Interval: promv1.Duration(config.GetMetricsScrapeInterval(mgh)),
				},
			},
		},
	}

	serviceMonitor := &promv1.ServiceMonitor{}
	err := r.GetClient().Get(ctx, client.ObjectKeyFromObject(expectedServiceMonitor), serviceMonitor)
	if err != nil && errors.IsNotFound(err) {
		return ctrl.Result{}, r.GetClient().Create(ctx, expectedServiceMonitor)
	} else if err != nil {
		return ctrl.Result{}, err
	}

	if !equality.Semantic.DeepDerivative(expectedServiceMonitor.Spec, serviceMonitor.Spec) ||
		!equality.Semantic.DeepDerivative(expectedServiceMonitor.GetLabels(), serviceMonitor.GetLabels()) {
		expectedServiceMonitor.ResourceVersion = serviceMonitor.ResourceVersion
		return ctrl.Result{}, r.GetClient().Update(ctx, expectedServiceMonitor)
	}

	return ctrl.Result{}, nil
}

func storageConnectionUpdated(storageConn *config.PostgresConnection) bool {
	updated := storageConnectionCache == nil
	if !reflect.DeepEqual(storageConn, storageConnectionCache) {
		updated = true
		storageConnectionCache = storageConn
	}
	return updated
}

type ManagerVariables struct {
	Image                     string
	Replicas                  int32
	ProxyImage                string
	ImagePullSecret           string
	ImagePullPolicy           string
	ProxySessionSecret        string
	DatabaseURL               string
	PostgresCACert            string
	TransportConfigSecret     string
	StorageConfigSecret       string
	KafkaConfigYaml           string
	InventoryConfigYaml       string
	TransportType             string
	Namespace                 string
	LeaseDuration             string
	RenewDeadline             string
	RetryPeriod               string
	SchedulerInterval         string
	SkipAuth                  bool
	LaunchJobNames            string
	NodeSelector              map[string]string
	Tolerations               []corev1.Toleration
	RetentionMonth            int
	StatisticLogInterval      string
	EnableGlobalResource      bool
	EnableInventoryAPI        bool
	EnablePprof               bool
	Resources                 *corev1.ResourceRequirements
	WithACM                   bool
	TransportFailureThreshold int
}
