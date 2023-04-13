package addon

import (
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/cluster-lifecycle-api/helpers/imageregistry"
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
	ImagePullSecretData    string
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
	InstallHostedMode      bool
	LeaseDuration          string
	RenewDeadline          string
	RetryPeriod            string
	KlusterletNamespace    string
	KlusterletWorkSA       string
	NodeSelector           map[string]string
	Tolerations            []corev1.Toleration
}

type HohAgentAddon struct {
	ctx                  context.Context
	client               client.Client
	kubeClient           kubernetes.Interface
	dynamicClient        dynamic.Interface
	leaderElectionConfig *commonobjects.LeaderElectionConfig
	log                  logr.Logger
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
	return nil
}

func (a *HohAgentAddon) GetValues(cluster *clusterv1.ManagedCluster,
	addon *addonapiv1alpha1.ManagedClusterAddOn,
) (addonfactory.Values, error) {
	log := a.log.WithValues("cluster", cluster.Name)
	installNamespace := addon.Spec.InstallNamespace
	if len(installNamespace) == 0 {
		installNamespace = operatorconstants.GHAgentInstallNamespace
	}
	mgh, err := a.getMulticlusterGlobalHub()
	if err != nil {
		log.Error(err, "failed to get MulticlusterGlobalHub")
		return nil, err
	}

	kafkaBootstrapServer, kafkaCACert, err := utils.GetKafkaConfig(a.ctx, a.kubeClient, mgh)
	if err != nil {
		log.Error(err, "failed to get kafkaConfig")
		return nil, err
	}

	messageCompressionType := string(mgh.Spec.MessageCompressionType)
	if messageCompressionType == "" {
		messageCompressionType = string(operatorv1alpha2.GzipCompressType)
	}

	image, err := a.getOverrideImage(mgh, cluster)
	if err != nil {
		return nil, err
	}
	log.Info("rendering manifests", "image", image)

	imagePullPolicy := corev1.PullAlways
	if mgh.Spec.ImagePullPolicy != "" {
		imagePullPolicy = mgh.Spec.ImagePullPolicy
	}

	manifestsConfig := ManifestsConfig{
		HoHAgentImage:          image,
		ImagePullPolicy:        string(imagePullPolicy),
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

	if err := a.setImagePullSecret(mgh, cluster, &manifestsConfig); err != nil {
		return nil, err
	}
	log.Info("rendering manifests", "pullSecret", manifestsConfig.ImagePullSecretName)

	if a.installACMHub(cluster) {
		manifestsConfig.InstallACMHub = true
		log.Info("installing ACM on regional hub")
		if err := a.setACMPackageConfigs(&manifestsConfig); err != nil {
			return nil, err
		}
	}

	manifestsConfig.Tolerations = mgh.Spec.Tolerations
	manifestsConfig.NodeSelector = mgh.Spec.NodeSelector

	a.setInstallHostedMode(cluster, &manifestsConfig)

	return addonfactory.StructToValues(manifestsConfig), nil
}

// GetImagePullSecret returns the image pull secret name and data
func (a *HohAgentAddon) setImagePullSecret(mgh *operatorv1alpha2.MulticlusterGlobalHub,
	cluster *clusterv1.ManagedCluster, manifestsConfig *ManifestsConfig,
) error {
	imagePullSecret := &corev1.Secret{}
	// pull secret from the mgh
	if len(mgh.Spec.ImagePullSecret) > 0 {
		var err error
		imagePullSecret, err = a.kubeClient.CoreV1().Secrets(mgh.Namespace).Get(a.ctx, mgh.Spec.ImagePullSecret,
			metav1.GetOptions{})
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

func (a *HohAgentAddon) getOverrideImage(mgh *operatorv1alpha2.MulticlusterGlobalHub,
	cluster *clusterv1.ManagedCluster,
) (string, error) {
	// image registry override by operator environment variable and mgh annotation
	configOverrideImage, err := config.GetImage(mgh, config.GlobalHubAgentImageKey)
	if err != nil {
		return "", err
	}

	// image registry override by cluster annotation(added by the ManagedClusterImageRegistry)
	image, err := imageregistry.OverrideImageByAnnotation(cluster.GetAnnotations(), configOverrideImage)
	if err != nil {
		return "", err
	}
	return image, nil
}
