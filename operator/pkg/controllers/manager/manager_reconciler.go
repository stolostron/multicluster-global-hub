package manager

import (
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"reflect"
	"strconv"
	"time"

	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

//go:embed manifests
var fs embed.FS

var (
	storageConnectionCache   *config.PostgresConnection
	transportConnectionCache *transport.KafkaConfig
)

type ManagerReconciler struct {
	ctrl.Manager
	kubeClient     kubernetes.Interface
	operatorConfig *config.OperatorConfig
}

var started bool

func StartController(initOption config.ControllerOption) error {
	if started {
		return nil
	}
	if config.GetTransporterConn() == nil {
		return nil
	}
	if config.GetStorageConnection() == nil {
		return nil
	}
	err := NewManagerReconciler(initOption.Manager,
		initOption.KubeClient, initOption.OperatorConfig).SetupWithManager(initOption.Manager)
	if err != nil {
		return err
	}
	started = true
	klog.Infof("inited manager controller")
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("manager").
		For(&v1alpha4.MulticlusterGlobalHub{},
			builder.WithPredicates(config.MGHPred)).
		Watches(&appsv1.Deployment{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(deploymentPred)).
		Complete(r)
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

// +kubebuilder:rbac:groups="admissionregistration.k8s.io",resources=mutatingwebhookconfigurations,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors;prometheusrules;podmonitors,verbs=get;create;delete;update;list;watch

func (r *ManagerReconciler) Reconcile(ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
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
				mgh.Namespace, config.COMPONENTS_MANAGER_NAME, reconcileErr),
		)
		if err != nil {
			klog.Errorf("failed to update mgh status, err:%v", err)
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
	dc, err := discovery.NewDiscoveryClientForConfig(r.Manager.GetConfig())
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

	transportConn := config.GetTransporterConn()
	if transportConn == nil || transportConn.BootstrapServer == "" {
		klog.V(2).Infof("Wait kafka connection created")

		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	storageConn := config.GetStorageConnection()
	if storageConn == nil || !config.GetDatabaseReady() {
		reconcileErr = fmt.Errorf("the storage connection or database isn't ready")
		return ctrl.Result{}, reconcileErr
	}

	if isMiddlewareUpdated(transportConn, storageConn) {
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

	kafkaConfigYaml, err := transportConn.YamlMarshal(true)
	if err != nil {
		reconcileErr = fmt.Errorf("failed to marshall kafka connetion for config: %w", err)
		return ctrl.Result{}, reconcileErr
	}

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
			PostgresCACert:        base64.StdEncoding.EncodeToString(storageConn.CACert),
			TransportType:         string(transport.Kafka),
			TransportConfigSecret: constants.GHTransportConfigSecret,
			KafkaConfigYaml:       base64.StdEncoding.EncodeToString(kafkaConfigYaml),
			Namespace:             mgh.Namespace,
			LeaseDuration:         strconv.Itoa(electionConfig.LeaseDuration),
			RenewDeadline:         strconv.Itoa(electionConfig.RenewDeadline),
			RetryPeriod:           strconv.Itoa(electionConfig.RetryPeriod),
			SchedulerInterval:     config.GetSchedulerInterval(mgh),
			SkipAuth:              config.SkipAuth(mgh),
			LaunchJobNames:        config.GetLaunchJobNames(mgh),
			NodeSelector:          mgh.Spec.NodeSelector,
			Tolerations:           mgh.Spec.Tolerations,
			RetentionMonth:        months,
			StatisticLogInterval:  config.GetStatisticLogInterval(),
			EnableGlobalResource:  r.operatorConfig.GlobalResourceEnabled,
			ImportClusterInHosted: config.GetImportClusterInHosted(),
			EnablePprof:           r.operatorConfig.EnablePprof,
			LogLevel:              r.operatorConfig.LogLevel,
			Resources:             utils.GetResources(operatorconstants.Manager, mgh.Spec.AdvancedSpec),
			WithACM:               config.IsACMResourceReady(),
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
		expectedServiceMonitor.ObjectMeta.ResourceVersion = serviceMonitor.ObjectMeta.ResourceVersion
		return ctrl.Result{}, r.GetClient().Update(ctx, expectedServiceMonitor)
	}

	return ctrl.Result{}, nil
}

func isMiddlewareUpdated(transportConn *transport.KafkaConfig, storageConn *config.PostgresConnection) bool {
	updated := false
	if transportConnectionCache == nil || storageConnectionCache == nil {
		updated = true
	}
	if !reflect.DeepEqual(transportConn, transportConnectionCache) {
		updated = true
	}
	if !reflect.DeepEqual(storageConn, storageConnectionCache) {
		updated = true
	}
	if updated {
		setMiddlewareCache(transportConn, storageConn)
	}
	return updated
}

func setMiddlewareCache(transportConn *transport.KafkaConfig, storageConn *config.PostgresConnection) {
	if transportConn != nil {
		transportConnectionCache = transportConn
	}

	if storageConn != nil {
		storageConnectionCache = storageConn
	}
}

type ManagerVariables struct {
	Image                 string
	Replicas              int32
	ProxyImage            string
	ImagePullSecret       string
	ImagePullPolicy       string
	ProxySessionSecret    string
	DatabaseURL           string
	PostgresCACert        string
	TransportConfigSecret string
	KafkaConfigYaml       string
	TransportType         string
	Namespace             string
	LeaseDuration         string
	RenewDeadline         string
	RetryPeriod           string
	SchedulerInterval     string
	SkipAuth              bool
	LaunchJobNames        string
	NodeSelector          map[string]string
	Tolerations           []corev1.Toleration
	RetentionMonth        int
	StatisticLogInterval  string
	EnableGlobalResource  bool
	ImportClusterInHosted bool
	EnablePprof           bool
	LogLevel              string
	Resources             *corev1.ResourceRequirements
	WithACM               bool
}
