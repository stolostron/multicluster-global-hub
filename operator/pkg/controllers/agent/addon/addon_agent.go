package addon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"

	addonoperatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	addonoperatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stolostron/cluster-lifecycle-api/helpers/imageregistry"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	addonv1alpha1client "open-cluster-management.io/api/client/addon/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

// GlobalHubAddonAgent defines the manifests of agent deployed on managed cluster
type GlobalHubAddonAgent struct {
	ctx           context.Context
	addonScheme   *runtime.Scheme
	client        client.Client
	dynamicClient dynamic.Interface
	kubeClient    kubernetes.Interface
	addonClient   *addonv1alpha1client.Clientset

	operatorConfig *config.OperatorConfig
}

func NewGlobalHubAddonAgent(ctx context.Context, c client.Client, kubeConfig *rest.Config,
	operatorConfig *config.OperatorConfig,
) (*GlobalHubAddonAgent, error) {
	addonScheme := runtime.NewScheme()
	utilruntime.Must(mchv1.AddToScheme(addonScheme))
	utilruntime.Must(globalhubv1alpha4.AddToScheme(addonScheme))
	utilruntime.Must(addonoperatorsv1.AddToScheme(addonScheme))
	utilruntime.Must(addonoperatorsv1alpha1.AddToScheme(addonScheme))

	dynamicClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	addonClient, err := addonv1alpha1client.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	addonAgent := &GlobalHubAddonAgent{
		ctx:            ctx,
		addonScheme:    addonScheme,
		client:         c,
		dynamicClient:  dynamicClient,
		kubeClient:     kubeClient,
		addonClient:    addonClient,
		operatorConfig: operatorConfig,
	}

	return addonAgent, nil
}

func (a *GlobalHubAddonAgent) GetValues(cluster *clusterv1.ManagedCluster,
	addon *addonapiv1alpha1.ManagedClusterAddOn,
) (addonfactory.Values, error) {
	installNamespace := addon.Spec.InstallNamespace
	if len(installNamespace) == 0 {
		installNamespace = operatorconstants.GHAgentInstallNamespace
	}
	mgh, err := config.GetMulticlusterGlobalHub(a.ctx, a.client)
	if err != nil {
		log.Errorw("failed to get MulticlusterGlobalHub", "error", err)
		return nil, err
	}

	image, err := config.GetAgentImage(cluster)
	if err != nil {
		log.Errorw("failed to get agent image", "error", err)
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
		log.Errorw("failed to marshal agent resource", "error", err)
		return nil, err
	}
	err = json.Unmarshal(jsonData, agentRes)
	if err != nil {
		log.Errorw("failed to unmarshal agent resource", "error", err)
		return nil, err
	}

	electionConfig, err := config.GetElectionConfig()
	if err != nil {
		log.Errorw("failed to get election config", "error", err)
		return nil, err
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
		TransportFailureThreshold: a.operatorConfig.TransportFailureThreshold,
		AgentQPS:                  agentQPS,
		AgentBurst:                agentBurst,
		LogLevel:                  string(logger.GetLogLevel()),
		EnablePprof:               a.operatorConfig.EnablePprof,
		Resources:                 agentRes,
		EnableStackroxIntegration: config.WithStackroxIntegration(mgh),
		StackroxPollInterval:      config.GetStackroxPollInterval(mgh),
	}

	if err := setTransportConfigs(&manifestsConfig, cluster, a.client); err != nil {
		log.Errorw("failed to set transport config", "error", err)
		return nil, err
	}

	if err := a.setImagePullSecret(mgh, cluster, &manifestsConfig); err != nil {
		log.Errorw("failed to set image pull secret", "error", err)
		return nil, err
	}
	log.Debugw("rendering manifests", "pullSecret", manifestsConfig.ImagePullSecretName,
		"image", manifestsConfig.HoHAgentImage)

	manifestsConfig.AggregationLevel = config.AggregationLevel
	manifestsConfig.EnableLocalPolicies = config.EnableLocalPolicies
	manifestsConfig.Tolerations = mgh.Spec.Tolerations
	manifestsConfig.NodeSelector = mgh.Spec.NodeSelector

	if err := setACMPackageConfigs(a.ctx, &manifestsConfig, cluster, a.dynamicClient); err != nil {
		log.Errorw("failed to set ACM package configs", "error", err)
		return nil, err
	}

	a.setInstallHostedMode(cluster, &manifestsConfig)

	return addonfactory.StructToValues(manifestsConfig), nil
}

// GetImagePullSecret returns the image pull secret name and data
func (a *GlobalHubAddonAgent) setImagePullSecret(mgh *globalhubv1alpha4.MulticlusterGlobalHub,
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

func (a *GlobalHubAddonAgent) setInstallHostedMode(cluster *clusterv1.ManagedCluster,
	manifestsConfig *config.ManifestsConfig,
) {
	// cluster hosted: feature gate is enabled
	if !config.GetImportClusterInHosted() {
		return
	}
	// cluster hosted: the annotation 'klusterlet-deploy-mode' = hosted is added in the webhook by the gh hosted config
	annotations := cluster.GetAnnotations()
	if annotations[constants.AnnotationClusterDeployMode] != constants.ClusterDeployModeHosted {
		return
	}

	// gh addon hosted: 'agent-deploy-mode' = 'Hosted' or ''
	if cluster.Labels == nil {
		return
	}
	deployMode, ok := cluster.Labels[constants.GHAgentDeployModeLabelKey]
	if !ok || !(deployMode == "" || deployMode == constants.GHAgentDeployModeHosted) {
		return
	}

	manifestsConfig.InstallHostedMode = true
	if annotations[constants.AnnotationClusterKlusterletDeployNamespace] != "" {
		manifestsConfig.KlusterletNamespace = annotations[constants.AnnotationClusterKlusterletDeployNamespace]
	}
	manifestsConfig.KlusterletWorkSA = fmt.Sprintf("klusterlet-%s-work-sa", cluster.GetName())
}
