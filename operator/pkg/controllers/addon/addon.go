package addon

import (
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

//go:embed manifests/templates
//go:embed manifests/templates/agent
//go:embed manifests/templates/hostedagent
//go:embed manifests/templates/hubcluster
var FS embed.FS

type ManifestsConfig struct {
	HoHAgentImage          string
	ImagePullSecretName    string
	ImagePullSecretVal     string
	ImagePullPolicy        string
	LeafHubID              string
	KafkaBootstrapServer   string
	TransportType          string
	TransportFormat        string
	KafkaCACert            string
	MessageCompressionType string
	InstallACMHub          bool
	Channel                string
	CurrentCSV             string
	Source                 string
	SourceNamespace        string
	ACMImagePullSecretData string
	InstallHostedMode      bool
	LeaseDuration          string
	RenewDeadline          string
	RetryPeriod            string
	KlusterletNamespace    string
	KlusterletWorkSA       string
}

type HohAgentAddon struct {
	ctx                  context.Context
	client               client.Client
	kubeClient           kubernetes.Interface
	dynamicClient        dynamic.Interface
	leaderElectionConfig *commonobjects.LeaderElectionConfig
}

func NewHohAgentAddon(ctx context.Context, client client.Client, kubeClient kubernetes.Interface,
	leaderElectionConfig *commonobjects.LeaderElectionConfig,
) *HohAgentAddon {
	return &HohAgentAddon{
		ctx:                  ctx,
		client:               client,
		kubeClient:           kubeClient,
		leaderElectionConfig: leaderElectionConfig,
	}
}

func (a *HohAgentAddon) getMulticlusterGlobalHub() (*operatorv1alpha2.MulticlusterGlobalHub, error) {
	mghList := &operatorv1alpha2.MulticlusterGlobalHubList{}
	err := a.client.List(a.ctx, mghList)
	if err != nil {
		return nil, err
	}
	if len(mghList.Items) != 1 {
		return nil, fmt.Errorf("the count of the mgh instance is not 1 in the cluster.%v", len(mghList.Items))
	}

	return &mghList.Items[0], nil
}

func (a *HohAgentAddon) installACMHub(cluster *clusterv1.ManagedCluster) bool {
	if _, exist := cluster.GetLabels()[operatorconstants.GHAgentACMHubInstallLabelKey]; !exist {
		return false
	}

	for _, claim := range cluster.Status.ClusterClaims {
		if claim.Name != constants.HubClusterClaimName {
			continue
		}

		if claim.Value == constants.HubNotInstalled ||
			claim.Value == constants.HubInstalledByGlobalHub {
			return true
		}
	}
	return false
}

func (a *HohAgentAddon) setInstallHostedMode(cluster *clusterv1.ManagedCluster,
	manifestsConfig *ManifestsConfig,
) {
	annotations := cluster.GetAnnotations()
	labels := cluster.GetLabels()
	if annotations[operatorconstants.AnnotationClusterDeployMode] !=
		operatorconstants.ClusterDeployModeHosted {
		return
	}
	if labels[operatorconstants.GHAgentDeployModeLabelKey] !=
		operatorconstants.GHAgentDeployModeHosted {
		return
	}

	manifestsConfig.InstallHostedMode = true
	if annotations[operatorconstants.AnnotationClusterKlusterletDeployNamespace] != "" {
		manifestsConfig.KlusterletNamespace = annotations[operatorconstants.AnnotationClusterKlusterletDeployNamespace]
	}
	manifestsConfig.KlusterletWorkSA = fmt.Sprintf("klusterlet-%s-work-sa", cluster.GetName())
}

func (a *HohAgentAddon) setACMPackageConfigs(manifestsConfig *ManifestsConfig) error {
	pm, err := GetPackageManifestConfig(a.ctx, a.dynamicClient)
	if err != nil {
		return err
	}
	manifestsConfig.Channel = pm.ACMDefaultChannel
	manifestsConfig.CurrentCSV = pm.ACMCurrentCSV
	manifestsConfig.Source = operatorconstants.ACMSubscriptionPublicSource
	manifestsConfig.SourceNamespace = operatorconstants.OpenshiftMarketPlaceNamespace

	imagePullSecret, err := a.kubeClient.CoreV1().Secrets(config.GetDefaultNamespace()).Get(a.ctx,
		operatorconstants.DefaultImagePullSecretName, metav1.GetOptions{})
	switch {
	case err == nil:
		imagePullSecretDataBase64 := base64.StdEncoding.EncodeToString(
			imagePullSecret.Data[corev1.DockerConfigJsonKey])
		manifestsConfig.ACMImagePullSecretData = imagePullSecretDataBase64
	case err != nil && !errors.IsNotFound(err):
		return err
	}

	return nil
}

func (a *HohAgentAddon) GetValues(cluster *clusterv1.ManagedCluster,
	addon *addonapiv1alpha1.ManagedClusterAddOn,
) (addonfactory.Values, error) {
	installNamespace := addon.Spec.InstallNamespace
	if len(installNamespace) == 0 {
		installNamespace = operatorconstants.GHAgentInstallNamespace
	}

	mgh, err := a.getMulticlusterGlobalHub()
	if err != nil {
		klog.Errorf("failed to get MulticlusterGlobalHub. err: %v", err)
		return nil, err
	}

	kafkaBootstrapServer, kafkaCACert, err := utils.GetKafkaConfig(a.ctx, a.kubeClient, mgh)
	if err != nil {
		klog.Errorf("failed to get kafkaConfig. err: %v", err)
		return nil, err
	}

	messageCompressionType := string(mgh.Spec.MessageCompressionType)
	if messageCompressionType == "" {
		messageCompressionType = string(operatorv1alpha2.GzipCompressType)
	}

	// load image config
	imageConfigMap := &corev1.ConfigMap{}
	if imageConfigMapName := config.GetImageOverridesConfigmap(mgh); imageConfigMapName != "" {
		if err := a.client.Get(a.ctx, types.NamespacedName{
			Namespace: mgh.GetNamespace(),
			Name:      imageConfigMapName,
		}, imageConfigMap, &client.GetOptions{}); err != nil {
			return nil, err
		}
	}
	if err := config.SetImageOverrides(mgh, imageConfigMap); err != nil {
		return nil, err
	}

	manifestsConfig := ManifestsConfig{
		HoHAgentImage:          config.GetImage(config.GlobalHubAgentImageKey),
		ImagePullPolicy:        config.GetImage(config.ImagePullPolicyKey),
		LeafHubID:              cluster.Name,
		KafkaBootstrapServer:   kafkaBootstrapServer,
		KafkaCACert:            kafkaCACert,
		MessageCompressionType: messageCompressionType,
		TransportType:          string(transport.Kafka),
		TransportFormat:        string(mgh.Spec.DataLayer.LargeScale.Kafka.TransportFormat),
		LeaseDuration:          strconv.Itoa(a.leaderElectionConfig.LeaseDuration),
		RenewDeadline:          strconv.Itoa(a.leaderElectionConfig.RenewDeadline),
		RetryPeriod:            strconv.Itoa(a.leaderElectionConfig.RetryPeriod),
		KlusterletNamespace:    "open-cluster-management-agent",
		KlusterletWorkSA:       "klusterlet-work-sa",
	}

	if len(config.GetImage(config.ImagePullSecretKey)) > 0 {
		imagePullSecret := &corev1.Secret{}
		if err := a.client.Get(a.ctx, types.NamespacedName{
			Namespace: mgh.GetNamespace(),
			Name:      config.GetImage(config.ImagePullSecretKey),
		}, imagePullSecret, &client.GetOptions{}); err != nil && !errors.IsNotFound(err) {
			return nil, err
		}
		if err == nil {
			imagePullSecretDataBase64 := base64.StdEncoding.EncodeToString(
				imagePullSecret.Data[corev1.DockerConfigJsonKey])
			manifestsConfig.ImagePullSecretName = imagePullSecret.Name
			manifestsConfig.ImagePullSecretVal = imagePullSecretDataBase64
		}
	}

	if a.installACMHub(cluster) {
		manifestsConfig.InstallACMHub = true
		if err := a.setACMPackageConfigs(&manifestsConfig); err != nil {
			return nil, err
		}
	}

	a.setInstallHostedMode(cluster, &manifestsConfig)

	return addonfactory.StructToValues(manifestsConfig), nil
}
