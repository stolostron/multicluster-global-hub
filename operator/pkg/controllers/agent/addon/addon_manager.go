package addon

import (
	"context"
	"embed"

	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/rest"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	addonframeworkagent "open-cluster-management.io/addon-framework/pkg/agent"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	agentcert "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/agent/certificates"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

//go:embed manifests/templates
//go:embed manifests/templates/agent
//go:embed manifests/templates/hostedagent
//go:embed manifests/templates/hubcluster
var FS embed.FS

// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=addondeploymentconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=work.open-cluster-management.io,resources=manifestworks,verbs=create;update;get;list;watch;delete;deletecollection;patch
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests,verbs=create;update;get;list;watch;patch
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests/approval,verbs=create;update;get;list;watch;patch
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests/status,verbs=update;get;list;watch;patch
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=signers,verbs=approve
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=signers,resourceNames=open-cluster-management.io/globalhub-signer,verbs=sign
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=get;create
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=create;update;get;list;watch;patch
// +kubebuilder:rbac:groups=packages.operators.coreos.com,resources=packagemanifests,verbs=get;list;watch

var addonManagerController *AddonManagerController

type AddonManagerController struct{}

func (r *AddonManagerController) IsResourceRemoved() bool {
	return true
}

func ReadyToEnableAddonManager(mgh *v1alpha4.MulticlusterGlobalHub) bool {
	if !config.IsACMResourceReady() {
		return false
	}
	if config.GetTransporterConn() == nil {
		return false
	}
	if !meta.IsStatusConditionTrue(mgh.Status.Conditions, config.CONDITION_TYPE_GLOBALHUB_READY) {
		return false
	}
	return true
}

func StartAddonManagerController(initOption config.ControllerOption) (config.ControllerInterface, error) {
	if addonManagerController != nil {
		return addonManagerController, nil
	}
	log.Info("start addon manager controller")

	if !ReadyToEnableAddonManager(initOption.MulticlusterGlobalHub) {
		return nil, nil
	}
	if err := StartGlobalHubAddonManager(initOption.Ctx, initOption.Manager.GetConfig(),
		initOption.Manager.GetClient(), initOption.OperatorConfig); err != nil {
		return nil, err
	}

	log.Infof("inited GlobalHubAddonManager controller")
	addonManagerController = &AddonManagerController{}
	return addonManagerController, nil
}

// 1. Start the addon manager(goroutine controller) by the global hub operator
// 2. The addon manager register itself to the hub by creating ClusterManagementAddon on the global hub cluster
func StartGlobalHubAddonManager(ctx context.Context, kubeConfig *rest.Config, client client.Client,
	operatorConfig *config.OperatorConfig,
) error {
	addonAgentConfig, err := NewGlobalHubAddonAgent(ctx, client, kubeConfig, operatorConfig)
	if err != nil {
		log.Errorw("failed to get addon agent config", "error", err)
		return err
	}

	factory := addonfactory.NewAgentAddonFactory(
		constants.GHManagedClusterAddonName, FS, "manifests").
		WithAgentHostedModeEnabledOption().
		WithScheme(addonAgentConfig.addonScheme).
		WithAgentHealthProber(&addonframeworkagent.HealthProber{
			Type: addonframeworkagent.HealthProberTypeLease,
		}).
		WithGetValuesFuncs(addonAgentConfig.GetValues,
			addonfactory.GetValuesFromAddonAnnotation,
			addonfactory.GetAddOnDeploymentConfigValues(
				addonfactory.NewAddOnDeploymentConfigGetter(addonAgentConfig.addonClient),
				addonfactory.ToAddOnDeploymentConfigValues,
				addonfactory.ToAddOnCustomizedVariableValues,
			))

	// enable the certificate signing feature: strimzi kafka, inventory api
	if config.TransporterProtocol() == transport.StrimziTransporter || config.EnableInventory() {
		factory.WithAgentRegistrationOption(newRegistrationOption())
	}
	globalHubAddon, err := factory.BuildTemplateAgentAddon()
	if err != nil {
		log.Errorw("failed to build global hub addon agent", "error", err)
		return err
	}

	addonMgr, err := addonmanager.New(kubeConfig)
	if err != nil {
		log.Errorw("failed to create addon manager", "error", err)
		return err
	}
	config.SetAddonManager(addonMgr)

	err = addonMgr.AddAgent(globalHubAddon)
	if err != nil {
		log.Errorw("failed to add agent addon to manager", "error", err)
		return err
	}

	log.Info("starting addon manager")
	go func() {
		if err = addonMgr.Start(ctx); err != nil {
			log.Fatalw("failed to start the global hub addon manager", "error", err)
		}
	}()
	return nil
}

func newRegistrationOption() *addonframeworkagent.RegistrationOption {
	return &addonframeworkagent.RegistrationOption{
		CSRConfigurations: agentcert.SignerAndCsrConfigurations,
		CSRApproveCheck:   agentcert.Approve,
		PermissionConfig: func(cluster *clusterv1.ManagedCluster, addon *addonapiv1alpha1.ManagedClusterAddOn) error {
			return nil
		},
		CSRSign: func(csr *certificatesv1.CertificateSigningRequest) []byte {
			key, cert := config.GetKafkaClientCA()
			return agentcert.Sign(csr, key, cert)
		},
	}
}
