package addon

import (
	"context"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
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
//+kubebuilder:rbac:groups=packages.operators.coreos.com,resources=packagemanifests,verbs=get;list;watch

type HoHAddonController struct {
	kubeConfig     *rest.Config
	client         client.Client
	leaderElection *commonobjects.LeaderElectionConfig
}

func NewHoHAddonController(kubeConfig *rest.Config, client client.Client,
	leaderElection *commonobjects.LeaderElectionConfig,
) *HoHAddonController {
	return &HoHAddonController{
		kubeConfig:     kubeConfig,
		client:         client,
		leaderElection: leaderElection,
	}
}

func (a *HoHAddonController) Start(ctx context.Context) error {
	addonScheme := runtime.NewScheme()
	utilruntime.Must(mchv1.AddToScheme(addonScheme))
	utilruntime.Must(v1alpha2.AddToScheme(addonScheme))
	utilruntime.Must(operatorsv1.AddToScheme(addonScheme))
	utilruntime.Must(operatorsv1alpha1.AddToScheme(addonScheme))

	kubeClient, err := kubernetes.NewForConfig(a.kubeConfig)
	if err != nil {
		klog.Errorf("failed to create kube client. err:%v", err)
		return err
	}
	dynamicClient, err := dynamic.NewForConfig(a.kubeConfig)
	if err != nil {
		klog.Errorf("failed to create dynamic client. err:%v", err)
		return err
	}

	hohAgentAddon := HohAgentAddon{
		ctx:                  ctx,
		kubeClient:           kubeClient,
		client:               a.client,
		dynamicClient:        dynamicClient,
		leaderElectionConfig: a.leaderElection,
	}

	mgr, err := addonmanager.New(a.kubeConfig)
	if err != nil {
		klog.Errorf("failed to create agent manager. err:%v", err)
		return err
	}

	agentAddon, err := addonfactory.NewAgentAddonFactory(constants.HoHManagedClusterAddonName, FS, "manifests").
		WithAgentHostedModeEnabledOption().
		WithGetValuesFuncs(hohAgentAddon.GetValues).
		WithScheme(addonScheme).
		BuildTemplateAgentAddon()
	if err != nil {
		klog.Errorf("failed to create agent addon. err:%v", err)
		return err
	}

	err = mgr.AddAgent(agentAddon)
	if err != nil {
		klog.Errorf("failed to add agent addon. err:%v", err)
		return err
	}

	return mgr.Start(ctx)
}
