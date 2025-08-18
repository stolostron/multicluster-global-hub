package grafana

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"net/url"
	"strings"
	"time"

	imagev1 "github.com/openshift/api/image/v1"
	routev1 "github.com/openshift/api/route/v1"
	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
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
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs,verbs=get;list;watch;
// +kubebuilder:rbac:groups="route.openshift.io",resources=routes,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=image.openshift.io,resources=imagestreams,verbs=get;list;watch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterrolebindings,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheuses/api,resourceNames=k8s,verbs=get;create;update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

//go:embed manifests
var fs embed.FS

const (
	datasourceKey  = "datasources.yaml"
	datasourceName = "multicluster-global-hub-grafana-datasources"

	mergedAlertName   = "multicluster-global-hub-alerting"
	DefaultAlertName  = "multicluster-global-hub-default-alerting"
	AlertConfigMapKey = "alerting.yaml"

	mergedGrafanaIniName  = "multicluster-global-hub-grafana-config"
	defaultGrafanaIniName = "multicluster-global-hub-default-grafana-config"
	grafanaIniKey         = "grafana.ini"

	grafanaDeploymentName = "multicluster-global-hub-grafana"
)

var WatchedSecret = sets.NewString(
	constants.CustomGrafanaIniName,
)

var WatchedConfigMap = sets.NewString(
	constants.CustomAlertName,
)

var (
	supportAlertConfig = sets.NewString(
		"groups",
		"policies",
		"contactPoints",
		"muteTimes",
		"templates",
		"resetPolicies",
	)
	supportDeleteAlertConfig = sets.NewString(
		"deleteRules",
		"deleteContactPoints",
		"deleteTemplates",
		"deleteMuteTimes",
	)
)

var log = logger.DefaultZapLogger()

type GrafanaReconciler struct {
	ctrl.Manager
	client     client.Client
	kubeClient kubernetes.Interface
	scheme     *runtime.Scheme
}

func NewGrafanaReconciler(mgr ctrl.Manager, kubeClient kubernetes.Interface) *GrafanaReconciler {
	return &GrafanaReconciler{
		Manager:    mgr,
		client:     mgr.GetClient(),
		kubeClient: kubeClient,
		scheme:     mgr.GetScheme(),
	}
}

var grafanaController *GrafanaReconciler

func (r *GrafanaReconciler) IsResourceRemoved() bool {
	return true
}

func StartController(initOption config.ControllerOption) (config.ControllerInterface, error) {
	if grafanaController != nil {
		return grafanaController, nil
	}
	if !config.IsACMResourceReady() {
		return nil, nil
	}
	if config.GetStorageConnection() == nil {
		return nil, nil
	}
	log.Info("start grafana controller")

	grafanaController = NewGrafanaReconciler(initOption.Manager,
		initOption.KubeClient)
	err := grafanaController.SetupWithManager(initOption.Manager)
	if err != nil {
		grafanaController = nil
		return grafanaController, err
	}
	log.Infof("Inited grafana controller")
	return grafanaController, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GrafanaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	managedby := ctrl.NewControllerManagedBy(mgr).Named("grafanaController").
		For(&v1alpha4.MulticlusterGlobalHub{},
			builder.WithPredicates(config.MGHPred)).
		Watches(&corev1.Secret{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(secretPred)).
		Watches(&corev1.ConfigMap{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(configmapPred)).
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
		Watches(&routev1.Route{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate))

	if _, err := mgr.GetRESTMapper().KindFor(schema.GroupVersionResource{
		Group:    "image.openshift.io",
		Version:  "v1",
		Resource: "imagestreams",
	}); err != nil {
		if meta.IsNoMatchError(err) {
			return managedby.Complete(r)
		}
		return err
	}
	return managedby.Watches(&imagev1.ImageStream{},
		&handler.EnqueueRequestForObject{}, builder.WithPredicates(imageStreamPred)).Complete(r)
}

var deploymentPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.Object.GetName() == config.COMPONENTS_GRAFANA_NAME
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.ObjectNew.GetName() == config.COMPONENTS_GRAFANA_NAME
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.Object.GetName() == config.COMPONENTS_GRAFANA_NAME
	},
}

var imageStreamPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return e.Object.GetName() == operatorconstants.OauthProxyImageStreamName
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		if e.ObjectNew.GetName() != operatorconstants.OauthProxyImageStreamName {
			return false
		}
		return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}

var configmapPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return WatchedConfigMap.Has(e.Object.GetName())
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		if e.ObjectNew.GetLabels()[constants.GlobalHubOwnerLabelKey] ==
			constants.GHOperatorOwnerLabelVal {
			return true
		}
		return WatchedConfigMap.Has(e.ObjectNew.GetName())
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		if e.Object.GetLabels()[constants.GlobalHubOwnerLabelKey] ==
			constants.GHOperatorOwnerLabelVal {
			return true
		}
		return WatchedConfigMap.Has(e.Object.GetName())
	},
}

var secretPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return WatchedSecret.Has(e.Object.GetName())
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return WatchedSecret.Has(e.ObjectNew.GetName())
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return WatchedSecret.Has(e.Object.GetName())
	},
}

func (r *GrafanaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Debugf("reconcile grafana controller")
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.GetClient())
	if err != nil {
		return ctrl.Result{}, err
	}
	if mgh == nil || config.IsPaused(mgh) || mgh.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	var reconcileErr error
	defer func() {
		err = config.UpdateMGHComponent(ctx, r.GetClient(),
			config.GetComponentStatusWithReconcileError(ctx, r.GetClient(),
				mgh.Namespace, config.COMPONENTS_GRAFANA_NAME, reconcileErr),
			false,
		)
		if err != nil {
			log.Errorf("failed to update mgh status, err:%v", err)
		}
	}()
	// generate random session secret for oauth-proxy
	proxySessionSecret, err := config.GetOauthSessionSecret()
	if err != nil {
		reconcileErr = fmt.Errorf("failed to generate random session secret for grafana oauth-proxy: %v", err)
		return ctrl.Result{}, reconcileErr
	}
	imagePullPolicy := corev1.PullAlways
	if mgh.Spec.ImagePullPolicy != "" {
		imagePullPolicy = mgh.Spec.ImagePullPolicy
	}

	replicas := int32(1)
	if mgh.Spec.AvailabilityConfig == v1alpha4.HAHigh {
		replicas = 2
	}
	// get the grafana objects
	grafanaRenderer, grafanaDeployer := renderer.NewHoHRenderer(fs), deployer.NewHoHDeployer(r.GetClient())
	grafanaObjects, err := grafanaRenderer.Render("manifests", "", func(profile string) (interface{}, error) {
		return struct {
			Namespace                 string
			Replicas                  int32
			SessionSecret             string
			ProxyImage                string
			GrafanaImage              string
			ImagePullSecret           string
			ImagePullPolicy           string
			DatasourceSecretName      string
			NodeSelector              map[string]string
			Tolerations               []corev1.Toleration
			Resources                 *corev1.ResourceRequirements
			EnableKafkaMetrics        bool
			EnablePostgresMetrics     bool
			EnableMetrics             bool
			EnableStackroxIntegration bool
		}{
			Namespace:                 mgh.GetNamespace(),
			Replicas:                  replicas,
			SessionSecret:             proxySessionSecret,
			ProxyImage:                config.GetImage(config.OauthProxyImageKey),
			GrafanaImage:              config.GetImage(config.GrafanaImageKey),
			ImagePullSecret:           mgh.Spec.ImagePullSecret,
			ImagePullPolicy:           string(imagePullPolicy),
			DatasourceSecretName:      datasourceName,
			NodeSelector:              mgh.Spec.NodeSelector,
			Tolerations:               mgh.Spec.Tolerations,
			EnableKafkaMetrics:        (!config.IsBYOKafka()) && mgh.Spec.EnableMetrics,
			EnablePostgresMetrics:     (!config.IsBYOPostgres()) && mgh.Spec.EnableMetrics,
			EnableMetrics:             mgh.Spec.EnableMetrics,
			EnableStackroxIntegration: config.WithStackroxIntegration(mgh),
			Resources:                 operatorutils.GetResources(operatorconstants.Grafana, mgh.Spec.AdvancedSpec),
		}, nil
	})
	if err != nil {
		reconcileErr = fmt.Errorf("failed to render grafana manifests: %w", err)
		return ctrl.Result{}, reconcileErr
	}
	// create restmapper for deployer to find GVR
	dc, err := discovery.NewDiscoveryClientForConfig(r.GetConfig())
	if err != nil {
		reconcileErr = err
		return ctrl.Result{}, reconcileErr
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))
	if err = operatorutils.ManipulateGlobalHubObjects(grafanaObjects, mgh, grafanaDeployer,
		mapper, r.GetScheme()); err != nil {
		reconcileErr = fmt.Errorf("failed to create/update grafana objects: %w", err)
		return ctrl.Result{}, reconcileErr
	}

	// generate datasource secret: must before the grafana objects
	changedDatasourceSecret, err := r.GenerateGrafanaDataSourceSecret(ctx, mgh)
	if err != nil {
		reconcileErr = fmt.Errorf("failed to generate grafana datasource secret: %v", err)
		return ctrl.Result{}, reconcileErr
	}

	changedAlert, err := r.generateAlertConfigMap(ctx, mgh)
	if err != nil {
		reconcileErr = fmt.Errorf("failed to generate merged alert configmap. err:%v", err)
		return ctrl.Result{}, reconcileErr
	}

	changedGrafanaIni, err := r.generateGrafanaIni(ctx, mgh)
	if err != nil {
		reconcileErr = fmt.Errorf("failed to generate grafana init. err:%v", err)
		return ctrl.Result{}, reconcileErr
	}

	if changedAlert || changedGrafanaIni || changedDatasourceSecret {
		err = commonutils.RestartPod(ctx, r.kubeClient, commonutils.GetDefaultNamespace(), grafanaDeploymentName)
		if err != nil {
			reconcileErr = fmt.Errorf("failed to restart grafana pod. err:%v", err)
			return ctrl.Result{}, reconcileErr
		}
	}

	return ctrl.Result{}, nil
}

// generateGranafaIni append the custom grafana.ini to default grafana.ini
func (r *GrafanaReconciler) generateGrafanaIni(
	ctx context.Context,
	mgh *v1alpha4.MulticlusterGlobalHub,
) (bool, error) {
	configNamespace := mgh.GetNamespace()

	defaultGrafanaIniSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultGrafanaIniName,
			Namespace: configNamespace,
		},
	}
	err := r.client.Get(ctx, client.ObjectKeyFromObject(defaultGrafanaIniSecret), defaultGrafanaIniSecret)
	if err != nil {
		return false, fmt.Errorf(
			"failed to get default grafana.ini secret. Namespace:%v, Name:%v, Error: %w",
			configNamespace,
			defaultGrafanaIniName,
			err,
		)
	}

	foundCustomGrafanaSecret := true
	customGrafanaIniSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.CustomGrafanaIniName,
			Namespace: configNamespace,
		},
	}
	err = r.client.Get(ctx, client.ObjectKeyFromObject(customGrafanaIniSecret), customGrafanaIniSecret)
	if err != nil && !errors.IsNotFound(err) {
		return false, fmt.Errorf(
			"failed to get custom grafana.ini secret. Namespace:%v, Name:%v, Error: %w",
			configNamespace,
			constants.CustomGrafanaIniName,
			err,
		)
	}
	if errors.IsNotFound(err) {
		foundCustomGrafanaSecret = false
	}

	mergedGrafanaIniSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mergedGrafanaIniName,
			Namespace: configNamespace,
			Labels: map[string]string{
				"name":                           grafanaDeploymentName,
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
		},
	}

	// Replace the grafana domain to grafana route url
	grafanaRoute := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      grafanaDeploymentName,
			Namespace: configNamespace,
		},
	}
	err = r.client.Get(ctx, client.ObjectKeyFromObject(grafanaRoute), grafanaRoute)
	if err != nil {
		log.Errorf("Failed to get grafana route: %v", err)
	} else {
		if len(grafanaRoute.Spec.Host) != 0 {
			defaultGrafanaIniSecret.Data[grafanaIniKey] = []byte(
				strings.ReplaceAll(
					string(defaultGrafanaIniSecret.Data[grafanaIniKey]),
					"localhost",
					grafanaRoute.Spec.Host,
				),
			)
		}
	}

	if !foundCustomGrafanaSecret {
		mergedGrafanaIniSecret.Data = map[string][]byte{
			grafanaIniKey: defaultGrafanaIniSecret.Data[grafanaIniKey],
		}
	} else {
		var mergedBytes []byte
		mergedBytes, err = mergeGrafanaIni(
			defaultGrafanaIniSecret.Data[grafanaIniKey],
			customGrafanaIniSecret.Data[grafanaIniKey],
		)
		if err != nil {
			log.Errorf("Failed to merge default and custom grafana.ini, err:%v", err)
			mergedBytes = defaultGrafanaIniSecret.Data[grafanaIniKey]
		}
		mergedGrafanaIniSecret.Data = map[string][]byte{
			grafanaIniKey: mergedBytes,
		}
	}

	if err = controllerutil.SetControllerReference(mgh, mergedGrafanaIniSecret, r.scheme); err != nil {
		return false, err
	}
	return operatorutils.ApplySecret(ctx, r.client, mergedGrafanaIniSecret)
}

// generateAlertConfigMap generate the alert configmap which grafana direclly use
// if there is no custom configmap, apply the alert configmap based on default alert configmap data.
// if there is the custom configmap, merge the custom configmap and default configmap then apply the merged configmap
func (r *GrafanaReconciler) generateAlertConfigMap(
	ctx context.Context,
	mgh *v1alpha4.MulticlusterGlobalHub,
) (bool, error) {
	configNamespace := mgh.GetNamespace()
	defaultAlertConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: configNamespace,
			Name:      DefaultAlertName,
		},
	}
	err := r.client.Get(ctx, client.ObjectKeyFromObject(defaultAlertConfigMap), defaultAlertConfigMap)
	if err != nil {
		return false, fmt.Errorf("failed to get default alert configmap: %w", err)
	}

	customAlertConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: configNamespace,
			Name:      constants.CustomAlertName,
		},
	}
	err = r.client.Get(ctx, client.ObjectKeyFromObject(customAlertConfigMap), customAlertConfigMap)
	if err != nil && !errors.IsNotFound(err) {
		return false, err
	}
	if errors.IsNotFound(err) {
		customAlertConfigMap = nil
	}

	var mergedAlertConfigMap *corev1.ConfigMap
	mergedAlertConfigMap, err = mergeAlertConfigMap(defaultAlertConfigMap, customAlertConfigMap)
	if err != nil {
		log.Errorf("Failed to merge custom alert configmap:%v. err: %v", customAlertConfigMap.Name, err)
		mergedAlertConfigMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mergedAlertName,
				Namespace: configNamespace,
				Labels: map[string]string{
					constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
				},
			},
			Data: defaultAlertConfigMap.Data,
		}
	}

	// Set MGH instance as the owner and controller
	if err = controllerutil.SetControllerReference(mgh, mergedAlertConfigMap, r.scheme); err != nil {
		return false, err
	}
	return operatorutils.ApplyConfigMap(ctx, r.client, mergedAlertConfigMap)
}

// mergeGrafanaIni merge the default grafana.ini and custom grafana.ini
// If a section exsts in both default and custom, use the default one
func mergeGrafanaIni(defaultIni, customIni []byte) ([]byte, error) {
	if len(defaultIni) == 0 {
		return customIni, nil
	}
	if len(customIni) == 0 {
		return defaultIni, nil
	}
	defaultCfg, err := ini.Load(defaultIni)
	if err != nil {
		return nil, err
	}
	customCfg, err := ini.Load(customIni)
	if err != nil {
		return nil, err
	}

	// delete sections from custom grafan.ini if the section exist in default grafana.ini
	for _, v := range customCfg.Sections() {
		if !defaultCfg.HasSection(v.Name()) {
			continue
		}
		customCfg.DeleteSection(v.Name())
	}

	var defaultBuf bytes.Buffer
	_, err = defaultCfg.WriteTo(&defaultBuf)
	if err != nil {
		return nil, err
	}

	var customBuf bytes.Buffer
	_, err = customCfg.WriteTo(&customBuf)
	if err != nil {
		return nil, err
	}
	mergedBuf := defaultBuf.String() + "\n" + customBuf.String()

	return []byte(mergedBuf), err
}

// mergeAlertYaml merge two grafana alert config file
func mergeAlertYaml(a, b []byte) ([]byte, error) {
	var o1, o2 map[string]interface{}
	if len(a) == 0 {
		return b, nil
	}
	if len(b) == 0 {
		return a, nil
	}

	if err := yaml.Unmarshal(a, &o1); err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(b, &o2); err != nil {
		return nil, err
	}

	for k, v := range o2 {
		if !supportAlertConfig.Has(k) && !supportDeleteAlertConfig.Has(k) {
			continue
		}
		o1Array, _ := o1[k].([]interface{})
		o2Array, _ := v.([]interface{})

		o1[k] = append(o1Array, o2Array...)
	}
	return yaml.Marshal(o1)
}

// mergeAlertConfigMap merge the default alert configmap and custom alert configmap
func mergeAlertConfigMap(defaultAlertConfigMap, customAlertConfigMap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	if defaultAlertConfigMap == nil && customAlertConfigMap == nil {
		return nil, nil
	}

	var mergedAlertConfigMap corev1.ConfigMap
	mergedAlertConfigMap.Name = mergedAlertName
	mergedAlertConfigMap.Namespace = commonutils.GetDefaultNamespace()
	if defaultAlertConfigMap.Namespace != "" {
		mergedAlertConfigMap.Namespace = defaultAlertConfigMap.Namespace
	}

	if defaultAlertConfigMap == nil {
		mergedAlertConfigMap.Data = customAlertConfigMap.Data
		return &mergedAlertConfigMap, nil
	}
	if customAlertConfigMap == nil {
		mergedAlertConfigMap.Data = defaultAlertConfigMap.Data
		return &mergedAlertConfigMap, nil
	}

	mergedAlertYaml, err := mergeAlertYaml(
		[]byte(defaultAlertConfigMap.Data[AlertConfigMapKey]),
		[]byte(customAlertConfigMap.Data[AlertConfigMapKey]))
	if err != nil {
		return nil, err
	}
	mergedAlertConfigMap.Data = map[string]string{
		AlertConfigMapKey: string(mergedAlertYaml),
	}
	return &mergedAlertConfigMap, nil
}

// GenerateGrafanaDataSource is used to generate the GrafanaDatasource as a secret.
// the GrafanaDatasource points to multicluster-global-hub cr
func (r *GrafanaReconciler) GenerateGrafanaDataSourceSecret(
	ctx context.Context,
	mgh *v1alpha4.MulticlusterGlobalHub,
) (bool, error) {
	storageConn := config.GetStorageConnection()
	if storageConn == nil {
		return false, fmt.Errorf("PgConnection config is null")
	}

	saToken := ""
	if mgh.Spec.EnableMetrics {
		saSecret := &corev1.Secret{}
		err := r.GetClient().Get(ctx, types.NamespacedName{
			Name:      "multicluster-global-hub-grafana-sa-secret",
			Namespace: mgh.GetNamespace(),
		}, saSecret)
		if err != nil {
			return false, err
		}
		saToken = string(saSecret.Data["token"])
	}

	datasourceVal, err := GrafanaDataSource(storageConn.ReadonlyUserDatabaseURI, storageConn.CACert, saToken)
	if err != nil {
		datasourceVal, err = GrafanaDataSource(storageConn.SuperuserDatabaseURI, storageConn.CACert, saToken)
		if err != nil {
			return false, err
		}
	}

	dsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      datasourceName,
			Namespace: mgh.GetNamespace(),
			Labels: map[string]string{
				"datasource/time-tarted":         time.Now().Format("2006-1-2.1504"),
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
		},
		Type: "Opaque",
		Data: map[string][]byte{
			datasourceKey: datasourceVal,
		},
	}

	// Set MGH instance as the owner and controller
	if err = controllerutil.SetControllerReference(mgh, dsSecret, r.GetScheme()); err != nil {
		return false, err
	}

	// Check if this already exists
	grafanaDSFound := &corev1.Secret{}
	err = r.GetClient().Get(ctx, types.NamespacedName{
		Name:      dsSecret.Name,
		Namespace: dsSecret.Namespace,
	}, grafanaDSFound)
	if err != nil {
		if errors.IsNotFound(err) {
			err = r.GetClient().Create(ctx, dsSecret)
			return true, err
		}
		return false, err
	}

	if !bytes.Equal(grafanaDSFound.Data[datasourceKey], datasourceVal) {
		grafanaDSFound.Data[datasourceKey] = datasourceVal
		err = r.GetClient().Update(ctx, grafanaDSFound)
		return true, err
	}
	return false, nil
}

func GrafanaDataSource(databaseURI string, cert []byte, serviceAccountToken string) ([]byte, error) {
	postgresURI := string(databaseURI)
	objURI, err := url.Parse(postgresURI)
	if err != nil {
		return nil, err
	}
	password, ok := objURI.User.Password()
	if !ok {
		return nil, fmt.Errorf("failed to get password from database_uri: %s", postgresURI)
	}

	// get the database from the objURI
	database := "hoh"
	paths := strings.Split(objURI.Path, "/")
	if len(paths) > 1 {
		database = paths[1]
	}

	postgresDS := &GrafanaDatasource{
		Name:      "Global-Hub-DataSource",
		Type:      "postgres",
		Access:    "proxy",
		IsDefault: true,
		URL:       objURI.Host,
		User:      objURI.User.Username(),
		Database:  database,
		Editable:  false,
		JSONData: &JsonData{
			QueryTimeout: "300s",
			TimeInterval: "30s",
		},
		SecureJSONData: &SecureJsonData{
			Password: password,
		},
	}
	datasource := []*GrafanaDatasource{postgresDS}

	if serviceAccountToken != "" {
		prometheusDS := &GrafanaDatasource{
			Name:      "Prometheus",
			Type:      "prometheus",
			Access:    "proxy",
			IsDefault: false,
			URL:       "https://thanos-querier.openshift-monitoring.svc.cluster.local:9091",
			BasicAuth: false,
			Editable:  false,
			JSONData: &JsonData{
				QueryTimeout:    "300s",
				TimeInterval:    "30s",
				TLSSkipVerify:   true,
				HttpHeaderName1: "Authorization",
			},
			SecureJSONData: &SecureJsonData{
				HttpHeaderValue1: "Bearer " + serviceAccountToken,
			},
		}
		datasource = append(datasource, prometheusDS)
	}

	if len(cert) > 0 {
		postgresDS.JSONData.SSLMode = objURI.Query().Get("sslmode") // sslmode == "verify-full" || sslmode == "verify-ca"
		postgresDS.JSONData.TLSAuth = true
		postgresDS.JSONData.TLSAuthWithCACert = true
		postgresDS.JSONData.TLSSkipVerify = true
		postgresDS.JSONData.TLSConfigurationMethod = "file-content"
		postgresDS.SecureJSONData.TLSCACert = string(cert)
	}
	grafanaDatasources, err := yaml.Marshal(GrafanaDatasources{
		APIVersion:  1,
		Datasources: datasource,
	})
	if err != nil {
		return nil, err
	}
	return grafanaDatasources, nil
}

type GrafanaDatasources struct {
	APIVersion  int                  `yaml:"apiVersion,omitempty"`
	Datasources []*GrafanaDatasource `yaml:"datasources,omitempty"`
}

type GrafanaDatasource struct {
	Access            string          `yaml:"access,omitempty"`
	BasicAuth         bool            `yaml:"basicAuth,omitempty"`
	BasicAuthPassword string          `yaml:"basicAuthPassword,omitempty"`
	BasicAuthUser     string          `yaml:"basicAuthUser,omitempty"`
	Editable          bool            `yaml:"editable,omitempty"`
	IsDefault         bool            `yaml:"isDefault,omitempty"`
	Name              string          `yaml:"name,omitempty"`
	OrgID             int             `yaml:"orgId,omitempty"`
	Type              string          `yaml:"type,omitempty"`
	URL               string          `yaml:"url,omitempty"`
	Database          string          `yaml:"database,omitempty"`
	User              string          `yaml:"user,omitempty"`
	Version           int             `yaml:"version,omitempty"`
	JSONData          *JsonData       `yaml:"jsonData,omitempty"`
	SecureJSONData    *SecureJsonData `yaml:"secureJsonData,omitempty"`
}

type JsonData struct {
	SSLMode                string `yaml:"sslmode,omitempty"`
	TLSAuth                bool   `yaml:"tlsAuth,omitempty"`
	TLSAuthWithCACert      bool   `yaml:"tlsAuthWithCACert,omitempty"`
	TLSConfigurationMethod string `yaml:"tlsConfigurationMethod,omitempty"`
	TLSSkipVerify          bool   `yaml:"tlsSkipVerify,omitempty"`
	QueryTimeout           string `yaml:"queryTimeout,omitempty"`
	HttpMethod             string `yaml:"httpMethod,omitempty"`
	TimeInterval           string `yaml:"timeInterval,omitempty"`
	HttpHeaderName1        string `yaml:"httpHeaderName1,omitempty"`
}

type SecureJsonData struct {
	Password         string `yaml:"password,omitempty"`
	TLSCACert        string `yaml:"tlsCACert,omitempty"`
	TLSClientCert    string `yaml:"tlsClientCert,omitempty"`
	TLSClientKey     string `yaml:"tlsClientKey,omitempty"`
	HttpHeaderValue1 string `yaml:"httpHeaderValue1,omitempty"`
}
