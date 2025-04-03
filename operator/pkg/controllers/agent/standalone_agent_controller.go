package agent

import (
	"context"
	"embed"
	"fmt"
	"strconv"

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
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/shared"
	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
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
// +kubebuilder:rbac:groups="config.openshift.io",resources=infrastructures;clusterversions,verbs=get;list;watch
// +kubebuilder:rbac:groups="policy.open-cluster-management.io",resources=policyautomations;policysets;placementbindings;policies,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups="cluster.open-cluster-management.io",resources=placements;managedclustersets;managedclustersetbindings,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups="cluster.open-cluster-management.io",resources=managedclusters;managedclusters/finalizers;placementdecisions;placementdecisions/finalizers;placements;placements/finalizers,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups="cluster.open-cluster-management.io",resources=clusterclaims,verbs=create;get;list;watch;patch;update;delete
// +kubebuilder:rbac:groups="",resources=namespaces;pods;configmaps;events;secrets,verbs=create;get;list;watch;patch;update;delete
// +kubebuilder:rbac:groups="apiextensions.k8s.io",resources=customresourcedefinitions,verbs=list;watch
// +kubebuilder:rbac:groups="route.openshift.io",resources=routes,verbs=get;list;watch
// +kubebuilder:rbac:groups="internal.open-cluster-management.io",resources=managedclusterinfos,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="apps.open-cluster-management.io",resources=placementrules;subscriptionreports,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="coordination.k8s.io",resources=leases,verbs=create;get;list;watch;patch;update

func (s *StandaloneAgentController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	mgha, err := config.GetMulticlusterGlobalHubAgent(ctx, s.Manager.GetClient())
	if err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		} else {
			return ctrl.Result{}, nil
		}
	}

	if config.IsAgentPaused(mgha) || mgha.DeletionTimestamp != nil {
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
		mgha.Namespace,
		mgha.Spec.ImagePullPolicy,
		s.Manager,
		mgha.Spec.Resources,
		mgha.Spec.ImagePullSecret,
		mgha.Spec.NodeSelector,
		mgha.Spec.Tolerations,
		mgha,
		clusterName,
	)
}

func renderAgentManifests(
	ctx context.Context,
	namespace string,
	agentImagePullPolicy corev1.PullPolicy,
	mgr ctrl.Manager,
	resources *shared.ResourceRequirements,
	imagePullSecret string,
	nodeSelector map[string]string,
	tolerations []corev1.Toleration,
	owner metav1.Object,
	clusterName string,
) (ctrl.Result, error) {
	// create new HoHRenderer and HoHDeployer
	hohRenderer, hohDeployer := renderer.NewHoHRenderer(fs), deployer.NewHoHDeployer(mgr.GetClient())

	// create discovery client
	dc, err := discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		return ctrl.Result{}, err
	}

	imagePullPolicy := corev1.PullAlways
	if agentImagePullPolicy != "" {
		imagePullPolicy = agentImagePullPolicy
	}
	agentQPS, agentBurst := config.GetAgentRestConfig()

	// set resource requirements
	resourceReq := corev1.ResourceRequirements{}
	requests := corev1.ResourceList{
		corev1.ResourceName(corev1.ResourceMemory): resource.MustParse(operatorconstants.AgentMemoryRequest),
		corev1.ResourceName(corev1.ResourceCPU):    resource.MustParse(operatorconstants.AgentCPURequest),
	}
	utils.SetResourcesFromCR(resources, requests)
	resourceReq.Requests = requests

	electionConfig, err := config.GetElectionConfig()
	if err != nil {
		log.Errorw("failed to get election config", "error", err)
		return ctrl.Result{}, err
	}

	// create restmapper for deployer to find GVR
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	agentObjects, err := hohRenderer.Render("manifests", "", func(profile string) (interface{}, error) {
		return struct {
			Image           string
			ImagePullSecret string
			ImagePullPolicy string
			Namespace       string
			NodeSelector    map[string]string
			Tolerations     []corev1.Toleration
			LeaseDuration   string
			RenewDeadline   string
			RetryPeriod     string
			AgentQPS        float32
			AgentBurst      int
			LogLevel        string
			ClusterId       string
			Resources       *corev1.ResourceRequirements
		}{
			Image:           config.GetImage(config.GlobalHubAgentImageKey),
			ImagePullSecret: imagePullSecret,
			ImagePullPolicy: string(imagePullPolicy),
			Namespace:       namespace,
			NodeSelector:    nodeSelector,
			Tolerations:     tolerations,
			LeaseDuration:   strconv.Itoa(electionConfig.LeaseDuration),
			RenewDeadline:   strconv.Itoa(electionConfig.RenewDeadline),
			RetryPeriod:     strconv.Itoa(electionConfig.RetryPeriod),
			AgentQPS:        agentQPS,
			AgentBurst:      agentBurst,
			ClusterId:       clusterName,
			Resources:       &resourceReq,
		}, nil
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to render standalone agent objects: %v", err)
	}
	if err = utils.ManipulateGlobalHubObjects(agentObjects, owner, hohDeployer, mapper, mgr.GetScheme()); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create/update standalone agent objects: %v", err)
	}
	return ctrl.Result{}, nil
}
