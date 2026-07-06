package agent

import (
	"context"
	"embed"
	"fmt"
	"strconv"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/shared"
	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	nputils "github.com/stolostron/multicluster-global-hub/operator/pkg/networkpolicy"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	standaloneAgentStarted = false
	//go:embed manifests
	fs embed.FS
)

type StandaloneAgentController struct {
	ctrl.Manager
}

var deplomentPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.Object.GetName() == config.COMPONENTS_AGENT_NAME
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.ObjectNew.GetName() == config.COMPONENTS_AGENT_NAME
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.Object.GetName() == config.COMPONENTS_AGENT_NAME
	},
}

var configMapPredicate = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.Object.GetName() == constants.GHConfigCMName
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetNamespace() == commonutils.GetDefaultNamespace() &&
			(e.ObjectNew.GetName() == constants.GHConfigCMName || e.ObjectNew.GetName() == constants.GHAgentConfigCMName)
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			(e.Object.GetName() == constants.GHConfigCMName || e.Object.GetName() == constants.GHAgentConfigCMName)
	},
}

func StartStandaloneAgentController(ctx context.Context, mgr ctrl.Manager) error {
	if standaloneAgentStarted {
		return nil
	}
	agentReconciler := &StandaloneAgentController{
		Manager: mgr,
	}

	err := ctrl.NewControllerManagedBy(mgr).
		Named("standalone-agent-reconciler").
		Watches(&v1alpha1.MulticlusterGlobalHubAgent{},
			&handler.EnqueueRequestForObject{}).
		Watches(&appsv1.Deployment{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(deplomentPred)).
		Watches(&corev1.ConfigMap{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Watches(&corev1.ServiceAccount{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Watches(&rbacv1.ClusterRole{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Watches(&rbacv1.ClusterRoleBinding{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Complete(agentReconciler)
	if err != nil {
		return err
	}
	standaloneAgentStarted = true

	// trigger the reconciler at the beginning to apply resources
	if _, err := agentReconciler.Reconcile(ctx, reconcile.Request{}); err != nil {
		log.Error(err)
	}
	return nil
}

// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubagents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubagents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubagents/finalizers,verbs=update
// +kubebuilder:rbac:groups="config.openshift.io",resources=apiservers,verbs=get
// +kubebuilder:rbac:groups="config.openshift.io",resources=infrastructures;clusterversions,verbs=get;list;watch
// +kubebuilder:rbac:groups="policy.open-cluster-management.io",resources=policyautomations;policysets;placementbindings;policies,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups="cluster.open-cluster-management.io",resources=placements;managedclustersets;managedclustersetbindings,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups="cluster.open-cluster-management.io",resources=managedclusters;managedclusters/finalizers;placementdecisions;placementdecisions/finalizers;placements;placements/finalizers,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups="cluster.open-cluster-management.io",resources=clusterclaims,verbs=create;get;list;watch;patch;update;delete
// +kubebuilder:rbac:groups="",resources=namespaces;pods;configmaps;events;secrets,verbs=create;get;list;watch;patch;update;delete
// +kubebuilder:rbac:groups="apiextensions.k8s.io",resources=customresourcedefinitions,verbs=list;watch
// +kubebuilder:rbac:groups="route.openshift.io",resources=routes,verbs=get;list;watch
// +kubebuilder:rbac:groups="internal.open-cluster-management.io",resources=managedclusterinfos,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="coordination.k8s.io",resources=leases,verbs=create;get;list;watch;patch;update

func (s *StandaloneAgentController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	mgha, err := config.GetMulticlusterGlobalHubAgent(ctx, s.GetClient())
	if err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		} else {
			return ctrl.Result{}, nil
		}
	}

	if mgha == nil || config.IsAgentPaused(mgha) || mgha.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	infra := &configv1.Infrastructure{}
	namespacedName := types.NamespacedName{Name: "cluster"}
	err = s.GetClient().Get(ctx, namespacedName, infra)
	if err != nil {
		return ctrl.Result{}, err
	}

	clusterName := string(infra.GetUID())

	return renderAgentManifests(
		ctx,
		s.Manager,
		clusterName,
		constants.GHTransportConfigSecret,
		mgha, nil,
		"standalone", // deploy mode is standalone
	)
}

func renderAgentManifests(
	ctx context.Context,
	mgr ctrl.Manager,
	clusterName string,
	transportConfigSecretName string,
	mgha *v1alpha1.MulticlusterGlobalHubAgent,
	mgh *v1alpha4.MulticlusterGlobalHub,
	deployMode string,
) (ctrl.Result, error) {
	src := agentManifestSourceFromCR(mgha, mgh)

	hohRenderer, hohDeployer := renderer.NewHoHRenderer(fs), deployer.NewHoHDeployer(mgr.GetClient())

	dc, err := discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		return ctrl.Result{}, err
	}

	imagePullPolicy := corev1.PullAlways
	if src.agentImagePullPolicy != "" {
		imagePullPolicy = src.agentImagePullPolicy
	}
	agentQPS, agentBurst := config.GetAgentRestConfig()

	resourceReq := corev1.ResourceRequirements{}
	requests := corev1.ResourceList{
		corev1.ResourceName(corev1.ResourceMemory): resource.MustParse(operatorconstants.AgentMemoryRequest),
		corev1.ResourceName(corev1.ResourceCPU):    resource.MustParse(operatorconstants.AgentCPURequest),
	}
	utils.SetResourcesFromCR(src.resources, requests)
	resourceReq.Requests = requests

	electionConfig, err := config.GetElectionConfig()
	if err != nil {
		log.Errorw("failed to get election config", "error", err)
		return ctrl.Result{}, err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	logLevel, err := getLogLevel(mgr.GetClient())
	if err != nil {
		log.Errorw("failed to get log level", "error", err)
		return ctrl.Result{}, err
	}

	bootstrapServer := standaloneAgentBootstrapServer(clusterName)
	npValues := nputils.BuildAgentValues(ctx, mgr.GetClient(), src.namespace, bootstrapServer)

	agentObjects, err := hohRenderer.Render("manifests", "", func(profile string) (interface{}, error) {
		return buildStandaloneAgentManifestTemplate(standaloneAgentRenderInput{
			src:                       src,
			clusterName:               clusterName,
			transportConfigSecretName: transportConfigSecretName,
			deployMode:                deployMode,
			imagePullPolicy:           imagePullPolicy,
			resourceReq:               resourceReq,
			leaseDuration:             electionConfig.LeaseDuration,
			renewDeadline:             electionConfig.RenewDeadline,
			retryPeriod:               electionConfig.RetryPeriod,
			agentQPS:                  agentQPS,
			agentBurst:                agentBurst,
			logLevel:                  logLevel,
			npValues:                  npValues,
		}), nil
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to render standalone agent objects: %v", err)
	}
	if err = utils.ManipulateGlobalHubObjects(agentObjects, src.owner, hohDeployer, mapper, mgr.GetScheme()); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create/update standalone agent objects: %v", err)
	}
	return ctrl.Result{}, nil
}

type agentManifestSource struct {
	namespace                 string
	agentImagePullPolicy      corev1.PullPolicy
	resources                 *shared.ResourceRequirements
	imagePullSecret           string
	nodeSelector              map[string]string
	tolerations               []corev1.Toleration
	owner                     metav1.Object
	enableStackroxIntegration bool
	stackroxPollInterval      time.Duration
	eventSendMode             string
}

func agentManifestSourceFromCR(
	mgha *v1alpha1.MulticlusterGlobalHubAgent,
	mgh *v1alpha4.MulticlusterGlobalHub,
) agentManifestSource {
	var src agentManifestSource
	if mgh != nil {
		src.namespace = mgh.Namespace
		src.agentImagePullPolicy = mgh.Spec.ImagePullPolicy
		if mgh.Spec.AdvancedSpec != nil &&
			mgh.Spec.AdvancedSpec.Agent != nil &&
			mgh.Spec.AdvancedSpec.Agent.Resources != nil {
			src.resources = mgh.Spec.AdvancedSpec.Agent.Resources
		}
		src.imagePullSecret = mgh.Spec.ImagePullSecret
		src.nodeSelector = mgh.Spec.NodeSelector
		src.tolerations = mgh.Spec.Tolerations
		src.owner = mgh
		src.enableStackroxIntegration = config.WithStackroxIntegration(mgh)
		src.stackroxPollInterval = config.GetStackroxPollInterval(mgh)
		src.eventSendMode = config.GetEventSendMode(mgh)
	}
	if mgha != nil {
		src.namespace = mgha.Namespace
		src.agentImagePullPolicy = mgha.Spec.ImagePullPolicy
		src.resources = mgha.Spec.Resources
		src.imagePullSecret = mgha.Spec.ImagePullSecret
		src.nodeSelector = mgha.Spec.NodeSelector
		src.tolerations = mgha.Spec.Tolerations
		src.owner = mgha
		src.eventSendMode = config.GetEventSendMode(mgha)
	}
	return src
}

func standaloneAgentBootstrapServer(clusterName string) string {
	trans := config.GetTransporter()
	if trans == nil {
		return ""
	}
	conn, err := trans.GetConnCredential(clusterName)
	if err != nil {
		log.Warnw("failed to resolve kafka bootstrap for agent NetworkPolicy", "error", err)
		return ""
	}
	return conn.BootstrapServer
}

type standaloneAgentRenderInput struct {
	src                       agentManifestSource
	clusterName               string
	transportConfigSecretName string
	deployMode                string
	imagePullPolicy           corev1.PullPolicy
	resourceReq               corev1.ResourceRequirements
	leaseDuration             int
	renewDeadline             int
	retryPeriod               int
	agentQPS                  float32
	agentBurst                int
	logLevel                  string
	npValues                  nputils.AgentManifestValues
}

func buildStandaloneAgentManifestTemplate(in standaloneAgentRenderInput) interface{} {
	return struct {
		Image                     string
		ImagePullSecret           string
		ImagePullPolicy           string
		Namespace                 string
		NodeSelector              map[string]string
		Tolerations               []corev1.Toleration
		LeaseDuration             string
		RenewDeadline             string
		RetryPeriod               string
		AgentQPS                  float32
		AgentBurst                int
		LogLevel                  string
		ClusterId                 string
		Resources                 *corev1.ResourceRequirements
		TransportConfigSecretName string
		EnableStackroxIntegration bool
		StackroxPollInterval      time.Duration
		DeployMode                string
		EventSendMode             string
		HubRole                   string
		StandbyHub                string
		APIServerCIDRs            []string
		ExternalKafkaCIDRs        []string
		WebhookEgressCIDRs        []string
	}{
		Image:                     config.GetImage(config.GlobalHubAgentImageKey),
		ImagePullSecret:           in.src.imagePullSecret,
		ImagePullPolicy:           string(in.imagePullPolicy),
		Namespace:                 in.src.namespace,
		NodeSelector:              in.src.nodeSelector,
		Tolerations:               in.src.tolerations,
		LeaseDuration:             strconv.Itoa(in.leaseDuration),
		RenewDeadline:             strconv.Itoa(in.renewDeadline),
		RetryPeriod:               strconv.Itoa(in.retryPeriod),
		AgentQPS:                  in.agentQPS,
		AgentBurst:                in.agentBurst,
		LogLevel:                  in.logLevel,
		ClusterId:                 in.clusterName,
		Resources:                 &in.resourceReq,
		TransportConfigSecretName: in.transportConfigSecretName,
		EnableStackroxIntegration: in.src.enableStackroxIntegration,
		StackroxPollInterval:      in.src.stackroxPollInterval,
		DeployMode:                in.deployMode,
		EventSendMode:             in.src.eventSendMode,
		HubRole:                   constants.GHHubRoleStandby,
		StandbyHub:                in.clusterName,
		APIServerCIDRs:            in.npValues.APIServerCIDRs,
		ExternalKafkaCIDRs:        in.npValues.ExternalKafkaCIDRs,
		WebhookEgressCIDRs:        in.npValues.WebhookEgressCIDRs,
	}
}

func getLogLevel(c client.Client) (string, error) {
	// Get the log level from the config map
	configMap := &corev1.ConfigMap{}
	err := c.Get(context.TODO(), types.NamespacedName{
		Name:      constants.GHConfigCMName,
		Namespace: commonutils.GetDefaultNamespace(),
	}, configMap)
	if err != nil {
		if errors.IsNotFound(err) {
			return string(logger.Info), nil
		}
		return "", err
	}

	logLevel := configMap.Data[logger.LogLevelKey]
	if logLevel != "" {
		logger.SetLogLevel(logger.LogLevel(logLevel))
	}
	return logLevel, nil
}
