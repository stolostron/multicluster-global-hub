package agent

import (
	"context"
	"embed"
	"fmt"
	"strconv"

	configv1 "github.com/openshift/api/config/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	standaloneAgentStarted = false
	//go:embed manifests/templates/standalone-agent
	fs embed.FS
)

type StandaloneAgentController struct {
	ctrl.Manager
	client.Client
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

func AddStandaloneAgentController(ctx context.Context, mgr ctrl.Manager) error {
	if standaloneAgentStarted {
		return nil
	}
	agentReconciler := &StandaloneAgentController{
		Client: mgr.GetClient(),
	}

	err := ctrl.NewControllerManagedBy(mgr).
		Named("standalone-agent-reconciler").
		Watches(&appsv1.Deployment{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(deplomentPred)).
		Complete(agentReconciler)
	if err != nil {
		return err
	}
	standaloneAgentStarted = true
	log.Info("the standalone agent reconciler is started")

	// trigger the reconciler at the beginning to apply resources
	agentReconciler.Reconcile(ctx, reconcile.Request{})
	return nil
}

// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubagents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubagents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubagents/finalizers,verbs=update
// +kubebuilder:rbac:groups="config.openshift.io",resources=infrastructures,verbs=get

func (s *StandaloneAgentController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	mgha, err := config.GetMulticlusterGlobalHubAgent(ctx, s.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	if config.IsAgentPaused(mgha) || mgha.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}
	// create new HoHRenderer and HoHDeployer
	hohRenderer, hohDeployer := renderer.NewHoHRenderer(fs), deployer.NewHoHDeployer(s.GetClient())

	// create discovery client
	dc, err := discovery.NewDiscoveryClientForConfig(s.Manager.GetConfig())
	if err != nil {
		return ctrl.Result{}, err
	}

	imagePullPolicy := corev1.PullAlways
	if mgha.Spec.ImagePullPolicy != "" {
		imagePullPolicy = mgha.Spec.ImagePullPolicy
	}
	agentQPS, agentBurst := config.GetAgentRestConfig()
	// agentResReq := utils.GetResources(operatorconstants.Agent, mgha.Spec.Resources)
	// agentRes := &config.Resources{}
	// jsonData, err := json.Marshal(agentResReq)
	// if err != nil {
	// 	log.Errorw("failed to marshal agent resource", "error", err)
	// 	return ctrl.Result{}, err
	// }
	// err = json.Unmarshal(jsonData, agentRes)
	// if err != nil {
	// 	log.Errorw("failed to unmarshal agent resource", "error", err)
	// 	return ctrl.Result{}, err
	// }
	electionConfig, err := config.GetElectionConfig()
	if err != nil {
		log.Errorw("failed to get election config", "error", err)
		return ctrl.Result{}, err
	}

	infra := &configv1.Infrastructure{}
	namespacedName := types.NamespacedName{Name: "cluster"}
	err = s.Client.Get(ctx, namespacedName, infra)
	if err != nil {
		return ctrl.Result{}, err
	}

	// create restmapper for deployer to find GVR
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	inventoryObjects, err := hohRenderer.Render("manifests", "", func(profile string) (interface{}, error) {
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
		}{
			Image:           config.GetImage(config.GlobalHubAgentImageKey),
			ImagePullSecret: mgha.Spec.ImagePullSecret,
			ImagePullPolicy: string(imagePullPolicy),
			Namespace:       mgha.Namespace,
			NodeSelector:    mgha.Spec.NodeSelector,
			Tolerations:     mgha.Spec.Tolerations,
			LeaseDuration:   strconv.Itoa(electionConfig.LeaseDuration),
			RenewDeadline:   strconv.Itoa(electionConfig.RenewDeadline),
			RetryPeriod:     strconv.Itoa(electionConfig.RetryPeriod),
			AgentQPS:        agentQPS,
			AgentBurst:      agentBurst,
			ClusterId:       string(infra.GetUID()),
		}, nil
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to render inventory objects: %v", err)
	}
	if err = utils.ManipulateGlobalHubObjects(inventoryObjects, mgha, hohDeployer, mapper, s.GetScheme()); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create/update inventory objects: %v", err)
	}
	return ctrl.Result{}, nil
}
