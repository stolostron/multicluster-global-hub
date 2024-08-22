package addon

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	"open-cluster-management.io/addon-framework/pkg/agent"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	addonv1alpha1client "open-cluster-management.io/api/client/addon/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/addon/certificates"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=managedclusteraddons,verbs=create;update;get;list;watch;delete;deletecollection;patch
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=managedclusteraddons/finalizers,verbs=update
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=managedclusteraddons/status,verbs=update;patch
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=addondeploymentconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=work.open-cluster-management.io,resources=manifestworks,verbs=create;update;get;list;watch;delete;deletecollection;patch
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests,verbs=create;update;get;list;watch;patch
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests/approval,verbs=create;update;get;list;watch;patch
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests/status,verbs=update;get;list;watch;patch
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=signers,verbs=approve
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=signers,resourceNames=open-cluster-management.io/globalhub-signer,verbs=sign
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=get;create
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=create;update;get;list;watch;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=create;update;get;list;watch;delete;deletecollection;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;update;get;list;watch;delete;deletecollection;patch
// +kubebuilder:rbac:groups=packages.operators.coreos.com,resources=packagemanifests,verbs=get;list;watch

type AddonController struct {
	addonManager   addonmanager.AddonManager
	kubeConfig     *rest.Config
	client         client.Client
	log            logr.Logger
	operatorConfig *config.OperatorConfig
}

// used to create addon manager
func NewAddonController(kubeConfig *rest.Config, client client.Client, operatorConfig *config.OperatorConfig,
) (*AddonController, error) {
	log := ctrl.Log.WithName("addon-controller")
	addonMgr, err := addonmanager.New(kubeConfig)
	if err != nil {
		log.Error(err, "failed to create addon manager")
		return nil, err
	}
	config.SetAddonManager(addonMgr)
	return &AddonController{
		kubeConfig:     kubeConfig,
		client:         client,
		log:            log,
		addonManager:   addonMgr,
		operatorConfig: operatorConfig,
	}, nil
}

func (a *AddonController) Start(ctx context.Context) error {
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
		ctx:            ctx,
		kubeClient:     kubeClient,
		client:         a.client,
		dynamicClient:  dynamicClient,
		log:            a.log.WithName("values"),
		operatorConfig: a.operatorConfig,
	}
	_, err = utils.WaitGlobalHubReady(ctx, a.client, 5*time.Second)
	if err != nil {
		return err
	}
	factory := addonfactory.NewAgentAddonFactory(
		operatorconstants.GHManagedClusterAddonName, FS, "manifests").
		WithAgentHostedModeEnabledOption().
		WithGetValuesFuncs(hohAgentAddon.GetValues,
			addonfactory.GetValuesFromAddonAnnotation,
			addonfactory.GetAddOnDeloymentConfigValues(
				addonfactory.NewAddOnDeloymentConfigGetter(addonClient),
				addonfactory.ToAddOnDeloymentConfigValues,
				addonfactory.ToAddOnCustomizedVariableValues,
			)).
		WithScheme(addonScheme)
	if config.TransporterProtocol() == transport.StrimziTransporter {
		factory.WithAgentRegistrationOption(newRegistrationOption())
	}
	agentAddon, err := factory.BuildTemplateAgentAddon()
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

func (a *AddonController) AddonManager() addonmanager.AddonManager {
	return a.addonManager
}

func newRegistrationOption() *agent.RegistrationOption {
	return &agent.RegistrationOption{
		CSRConfigurations: certificates.SignerAndCsrConfigurations,
		CSRApproveCheck:   certificates.Approve,
		PermissionConfig: func(cluster *clusterv1.ManagedCluster, addon *addonapiv1alpha1.ManagedClusterAddOn) error {
			return nil
		},
		CSRSign: func(csr *certificatesv1.CertificateSigningRequest) []byte {
			key, cert := config.GetClientCA()
			return certificates.Sign(csr, key, cert)
		},
	}
}
