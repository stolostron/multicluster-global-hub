package addon

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	addonv1alpha1client "open-cluster-management.io/api/client/addon/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	hubofhubs "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs"
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
)

// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=managedclusteraddons,verbs=create;update;get;list;watch;delete;deletecollection;patch
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=managedclusteraddons/finalizers,verbs=update
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=managedclusteraddons/status,verbs=update;patch
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=addondeploymentconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=work.open-cluster-management.io,resources=manifestworks,verbs=create;update;get;list;watch;delete;deletecollection;patch
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests,verbs=create;update;get;list;watch;patch
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests/approval,verbs=create;update;get;list;watch;patch
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=signers,verbs=approve
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=get;create
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=create;update;get;list;watch;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=create;update;get;list;watch;delete;deletecollection;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;update;get;list;watch;delete;deletecollection;patch
// +kubebuilder:rbac:groups=packages.operators.coreos.com,resources=packagemanifests,verbs=get;list;watch

type HoHAddonController struct {
	kubeConfig           *rest.Config
	client               client.Client
	leaderElection       *commonobjects.LeaderElectionConfig
	log                  logr.Logger
	addonManager         addonmanager.AddonManager
	MiddlewareConfig     *hubofhubs.MiddlewareConfig
	EnableGlobalResource bool
	ControllerConfig     *corev1.ConfigMap
	LogLevel             string
}

func NewHoHAddonController(kubeConfig *rest.Config, client client.Client,
	leaderElection *commonobjects.LeaderElectionConfig, middlewareCfg *hubofhubs.MiddlewareConfig,
	enableGlobalResource bool, controllerConfig *corev1.ConfigMap, logLevel string,
) (*HoHAddonController, error) {
	log := ctrl.Log.WithName("addon-controller")
	addonMgr, err := addonmanager.New(kubeConfig)
	if err != nil {
		log.Error(err, "failed to create addon manager")
		return nil, err
	}
	return &HoHAddonController{
		kubeConfig:           kubeConfig,
		client:               client,
		leaderElection:       leaderElection,
		log:                  log,
		addonManager:         addonMgr,
		MiddlewareConfig:     middlewareCfg,
		EnableGlobalResource: enableGlobalResource,
		ControllerConfig:     controllerConfig,
		LogLevel:             logLevel,
	}, nil
}

func (a *HoHAddonController) Start(ctx context.Context) error {
	addonScheme := runtime.NewScheme()
	utilruntime.Must(mchv1.AddToScheme(addonScheme))
	utilruntime.Must(globalhubv1alpha4.AddToScheme(addonScheme))
	utilruntime.Must(operatorsv1.AddToScheme(addonScheme))
	utilruntime.Must(operatorsv1alpha1.AddToScheme(addonScheme))

	kubeClient, err := kubernetes.NewForConfig(a.kubeConfig)
	if err != nil {
		a.log.Error(err, "failed to create kube client")
		return err
	}
	dynamicClient, err := dynamic.NewForConfig(a.kubeConfig)
	if err != nil {
		a.log.Error(err, "failed to create dynamic client")
		return err
	}

	addonClient, err := addonv1alpha1client.NewForConfig(a.kubeConfig)
	if err != nil {
		a.log.Error(err, "failed to create addon client")
		return err
	}

	hohAgentAddon := HohAgentAddon{
		ctx:                  ctx,
		kubeClient:           kubeClient,
		client:               a.client,
		dynamicClient:        dynamicClient,
		leaderElectionConfig: a.leaderElection,
		log:                  a.log.WithName("values"),
		MiddlewareConfig:     a.MiddlewareConfig,
		EnableGlobalResource: a.EnableGlobalResource,
		ControllerConfig:     a.ControllerConfig,
		LogLevel:             a.LogLevel,
	}
	_, err = utils.WaitGlobalHubReady(ctx, a.client, 5*time.Second)
	if err != nil {
		return err
	}
	agentAddon, err := addonfactory.NewAgentAddonFactory(
		operatorconstants.GHManagedClusterAddonName, FS, "manifests").
		WithAgentHostedModeEnabledOption().
		WithGetValuesFuncs(hohAgentAddon.GetValues,
			addonfactory.GetValuesFromAddonAnnotation,
			addonfactory.GetAddOnDeloymentConfigValues(
				addonfactory.NewAddOnDeloymentConfigGetter(addonClient),
				addonfactory.ToAddOnDeloymentConfigValues,
				addonfactory.ToAddOnCustomizedVariableValues,
			)).
		WithScheme(addonScheme).
		BuildTemplateAgentAddon()
	if err != nil {
		a.log.Error(err, "failed to create agent addon")
		return err
	}

	err = a.addonManager.AddAgent(agentAddon)
	if err != nil {
		a.log.Error(err, "failed to add agent addon to manager")
		return err
	}

	a.log.Info("starting addon manager")
	return a.addonManager.Start(ctx)
}

func (a *HoHAddonController) AddonManager() addonmanager.AddonManager {
	return a.addonManager
}
