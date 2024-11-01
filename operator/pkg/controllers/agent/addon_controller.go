package agent

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stolostron/cluster-lifecycle-api/helpers/imageregistry"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
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
	agentcert "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/agent/certificates"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

//go:embed manifests/templates
//go:embed manifests/templates/agent
//go:embed manifests/templates/hostedagent
//go:embed manifests/templates/hubcluster
var FS embed.FS

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

type GlobalHubAddonController struct {
	ctx            context.Context
	addonManager   addonmanager.AddonManager
	kubeConfig     *rest.Config
	client         client.Client
	kubeClient     *kubernetes.Clientset
	dynamicClient  dynamic.Interface
	operatorConfig *config.OperatorConfig
}

var (
	once            sync.Once
	addonController *GlobalHubAddonController
)

// used to create addon manager
func AddGlobalHubAddonController(ctx context.Context, mgr ctrl.Manager, kubeConfig *rest.Config, client client.Client,
	operatorConfig *config.OperatorConfig,
) (*GlobalHubAddonController, error) {
	if addonController != nil {
		return addonController, nil
	}
	addonMgr, err := addonmanager.New(kubeConfig)
	if err != nil {
		log.Error(err, "failed to create addon manager")
		return nil, err
	}
	config.SetAddonManager(addonMgr)
	c := &GlobalHubAddonController{
		ctx:            ctx,
		kubeConfig:     kubeConfig,
		client:         client,
		addonManager:   addonMgr,
		operatorConfig: operatorConfig,
	}
	err = mgr.Add(c)
	if err != nil {
		return nil, err
	}
	addonController = c
	return c, nil
}

func (a *GlobalHubAddonController) Start(ctx context.Context) error {
	addonScheme := runtime.NewScheme()
	utilruntime.Must(mchv1.AddToScheme(addonScheme))
	utilruntime.Must(globalhubv1alpha4.AddToScheme(addonScheme))
	utilruntime.Must(operatorsv1.AddToScheme(addonScheme))
	utilruntime.Must(operatorsv1alpha1.AddToScheme(addonScheme))

	dynamicClient, err := dynamic.NewForConfig(a.kubeConfig)
	if err != nil {
		log.Error(err, "failed to create dynamic client")
		return err
	}
	a.dynamicClient = dynamicClient

	kubeClient, err := kubernetes.NewForConfig(a.kubeConfig)
	if err != nil {
		log.Error(err, "failed to create kube client")
		return err
	}
	a.kubeClient = kubeClient

	addonClient, err := addonv1alpha1client.NewForConfig(a.kubeConfig)
	if err != nil {
		log.Error(err, "failed to create addon client")
		return err
	}

	_, err = utils.WaitGlobalHubReady(ctx, a.client, 5*time.Second)
	if err != nil {
		return err
	}
	factory := addonfactory.NewAgentAddonFactory(
		operatorconstants.GHManagedClusterAddonName, FS, "manifests").
		WithAgentHostedModeEnabledOption().
		WithGetValuesFuncs(a.GetValues,
			addonfactory.GetValuesFromAddonAnnotation,
			addonfactory.GetAddOnDeploymentConfigValues(
				addonfactory.NewAddOnDeploymentConfigGetter(addonClient),
				addonfactory.ToAddOnDeploymentConfigValues,
				addonfactory.ToAddOnCustomizedVariableValues,
			)).
		WithScheme(addonScheme)

	// enable the certificate signing feature: strimzi kafka, inventory api
	if config.TransporterProtocol() == transport.StrimziTransporter || config.EnableInventory() {
		factory.WithAgentRegistrationOption(newRegistrationOption())
	}
	agentAddon, err := factory.BuildTemplateAgentAddon()
	if err != nil {
		log.Error(err, "failed to create agent addon")
		return err
	}

	once.Do(func() {
		err = a.addonManager.AddAgent(agentAddon)
		if err != nil {
			log.Errorw("failed to add agent addon to manager", "error", err)
		}
	})

	log.Info("starting addon manager")
	return a.addonManager.Start(ctx)
}

func (a *GlobalHubAddonController) AddonManager() addonmanager.AddonManager {
	return a.addonManager
}

func newRegistrationOption() *agent.RegistrationOption {
	return &agent.RegistrationOption{
		CSRConfigurations: agentcert.SignerAndCsrConfigurations,
		CSRApproveCheck:   agentcert.Approve,
		PermissionConfig: func(cluster *clusterv1.ManagedCluster, addon *addonapiv1alpha1.ManagedClusterAddOn) error {
			return nil
		},
		CSRSign: func(csr *certificatesv1.CertificateSigningRequest) []byte {
			key, cert := config.GetKafkaClientCA()
			if config.EnableInventory() {
				key, cert = config.GetInventoryClientCA()
			}
			return agentcert.Sign(csr, key, cert)
		},
	}
}

func (a *GlobalHubAddonController) GetValues(cluster *clusterv1.ManagedCluster,
	addon *addonapiv1alpha1.ManagedClusterAddOn,
) (addonfactory.Values, error) {
	installNamespace := addon.Spec.InstallNamespace
	if len(installNamespace) == 0 {
		installNamespace = operatorconstants.GHAgentInstallNamespace
	}
	mgh, err := config.GetMulticlusterGlobalHub(a.ctx, a.client)
	if err != nil {
		log.Error(err, "failed to get MulticlusterGlobalHub")
		return nil, err
	}

	image, err := config.GetAgentImage(cluster)
	if err != nil {
		return nil, err
	}

	imagePullPolicy := corev1.PullAlways
	if mgh.Spec.ImagePullPolicy != "" {
		imagePullPolicy = mgh.Spec.ImagePullPolicy
	}

	agentQPS, agentBurst := config.GetAgentRestConfig()

	agentResReq := utils.GetResources(operatorconstants.Agent, mgh.Spec.AdvancedSpec)
	agentRes := &config.Resources{}
	jsonData, err := json.Marshal(agentResReq)
	if err != nil {
		log.Error(err, "failed to marshal agent resources")
	}
	err = json.Unmarshal(jsonData, agentRes)
	if err != nil {
		log.Error(err, "failed to unmarshal to agent resources")
	}

	electionConfig, err := config.GetElectionConfig()
	if err != nil {
		log.Error(err, "failed to get election config")
	}

	manifestsConfig := config.ManifestsConfig{
		HoHAgentImage:             image,
		ImagePullPolicy:           string(imagePullPolicy),
		LeafHubID:                 cluster.Name,
		TransportConfigSecret:     constants.GHTransportConfigSecret,
		LeaseDuration:             strconv.Itoa(electionConfig.LeaseDuration),
		RenewDeadline:             strconv.Itoa(electionConfig.RenewDeadline),
		RetryPeriod:               strconv.Itoa(electionConfig.RetryPeriod),
		KlusterletNamespace:       "open-cluster-management-agent",
		KlusterletWorkSA:          "klusterlet-work-sa",
		EnableGlobalResource:      a.operatorConfig.GlobalResourceEnabled,
		AgentQPS:                  agentQPS,
		AgentBurst:                agentBurst,
		LogLevel:                  a.operatorConfig.LogLevel,
		EnablePprof:               a.operatorConfig.EnablePprof,
		Resources:                 agentRes,
		EnableStackroxIntegration: config.WithStackroxIntegration(mgh),
		StackroxPollInterval:      config.GetStackroxPollInterval(mgh),
	}

	if err := setTransportConfigs(a.ctx, &manifestsConfig, cluster, a.client); err != nil {
		return nil, err
	}

	if err := a.setImagePullSecret(mgh, cluster, &manifestsConfig); err != nil {
		return nil, err
	}
	log.Debugw("rendering manifests", "pullSecret", manifestsConfig.ImagePullSecretName,
		"image", manifestsConfig.HoHAgentImage)

	manifestsConfig.AggregationLevel = config.AggregationLevel
	manifestsConfig.EnableLocalPolicies = config.EnableLocalPolicies
	manifestsConfig.Tolerations = mgh.Spec.Tolerations
	manifestsConfig.NodeSelector = mgh.Spec.NodeSelector

	if err := setACMPackageConfigs(a.ctx, &manifestsConfig, cluster, a.dynamicClient); err != nil {
		return nil, err
	}

	a.setInstallHostedMode(cluster, &manifestsConfig)

	return addonfactory.StructToValues(manifestsConfig), nil
}

func (a *GlobalHubAddonController) setInstallHostedMode(cluster *clusterv1.ManagedCluster,
	manifestsConfig *config.ManifestsConfig,
) {
	annotations := cluster.GetAnnotations()
	labels := cluster.GetLabels()
	if annotations[constants.AnnotationClusterDeployMode] !=
		constants.ClusterDeployModeHosted {
		return
	}
	if labels[operatorconstants.GHAgentDeployModeLabelKey] !=
		operatorconstants.GHAgentDeployModeHosted {
		return
	}

	manifestsConfig.InstallHostedMode = true
	if annotations[constants.AnnotationClusterKlusterletDeployNamespace] != "" {
		manifestsConfig.KlusterletNamespace = annotations[constants.AnnotationClusterKlusterletDeployNamespace]
	}
	manifestsConfig.KlusterletWorkSA = fmt.Sprintf("klusterlet-%s-work-sa", cluster.GetName())
}

// GetImagePullSecret returns the image pull secret name and data
func (a *GlobalHubAddonController) setImagePullSecret(mgh *globalhubv1alpha4.MulticlusterGlobalHub,
	cluster *clusterv1.ManagedCluster, manifestsConfig *config.ManifestsConfig,
) error {
	imagePullSecret := &corev1.Secret{}
	// pull secret from the mgh
	if len(mgh.Spec.ImagePullSecret) > 0 {
		err := a.client.Get(a.ctx, client.ObjectKey{
			Namespace: mgh.Namespace,
			Name:      mgh.Spec.ImagePullSecret,
		}, imagePullSecret, &client.GetOptions{})
		if err != nil {
			return err
		}
	}

	// pull secret from the cluster annotation(added by ManagedClusterImageRegistry controller)
	c := imageregistry.NewClient(a.kubeClient)
	if pullSecret, err := c.Cluster(cluster).PullSecret(); err != nil {
		return err
	} else if pullSecret != nil {
		imagePullSecret = pullSecret
	}

	if len(imagePullSecret.Name) > 0 && len(imagePullSecret.Data[corev1.DockerConfigJsonKey]) > 0 {
		manifestsConfig.ImagePullSecretName = imagePullSecret.GetName()
		manifestsConfig.ImagePullSecretData = base64.StdEncoding.EncodeToString(
			imagePullSecret.Data[corev1.DockerConfigJsonKey])
	}
	return nil
}
