package addon

import (
	"context"
	"embed"
	"encoding/base64"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	globalconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
)

//go:embed manifests/templates
//go:embed manifests/templates/agent
//go:embed manifests/templates/hostedagent
//go:embed manifests/templates/hubcluster
var FS embed.FS

type ManifestsConfig struct {
	HoHAgentImage          string
	LeafHubID              string
	KafkaBootstrapServer   string
	KafkaCA                string
	InstallACMHub          bool
	Channel                string
	CurrentCSV             string
	Source                 string
	SourceNamespace        string
	ACMImagePullSecretData string
	InstallHostedMode      bool
}

type HohAgentAddon struct {
	ctx           context.Context
	client        client.Client
	kubeClient    kubernetes.Interface
	dynamicClient dynamic.Interface
}

func NewHohAgentAddon(ctx context.Context, client client.Client, kubeClient kubernetes.Interface) *HohAgentAddon {
	return &HohAgentAddon{
		ctx:        ctx,
		client:     client,
		kubeClient: kubeClient,
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
	for _, claim := range cluster.Status.ClusterClaims {
		if claim.Name != globalconstants.VersionClusterClaimName {
			continue
		}

		// only install ACM when the versionclaim is created and has empty value
		if claim.Value == "" {
			return true
		}
		return false
	}
	return false
}

func (a *HohAgentAddon) setInstallHostedMode(addon *addonapiv1alpha1.ManagedClusterAddOn,
	manifestsConfig *ManifestsConfig,
) {
	annotations := addon.GetAnnotations()
	if annotations[constants.AnnotationAddonHostingClusterName] != "" {
		manifestsConfig.InstallHostedMode = true
	}
	return
}

func (a *HohAgentAddon) setACMPackageConfigs(manifestsConfig *ManifestsConfig) error {
	pm, err := GetPackageManifestConfig(a.ctx, a.dynamicClient)
	if err != nil {
		return err
	}
	manifestsConfig.Channel = pm.ACMDefaultChannel
	manifestsConfig.CurrentCSV = pm.ACMCurrentCSV
	manifestsConfig.Source = constants.ACMSubscriptionPublicSource
	manifestsConfig.SourceNamespace = constants.OpenshiftMarketPlaceNamespace

	imagePullSecret, err := a.kubeClient.CoreV1().Secrets(config.GetDefaultNamespace()).Get(a.ctx,
		constants.DefaultImagePullSecretName, metav1.GetOptions{})
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
		installNamespace = constants.HoHAgentInstallNamespace
	}

	mgh, err := a.getMulticlusterGlobalHub()
	if err != nil {
		klog.Errorf("failed to get MulticlusterGlobalHub. err: %v", err)
		return nil, err
	}

	kafkaBootstrapServer, kafkaCA, err := utils.GetKafkaConfig(a.ctx, a.kubeClient, mgh)
	if err != nil {
		klog.Errorf("failed to get kafkaConfig. err: %v", err)
		return nil, err
	}

	manifestsConfig := ManifestsConfig{
		HoHAgentImage:        config.GetImage("multicluster_global_hub_agent"),
		LeafHubID:            cluster.Name,
		KafkaBootstrapServer: kafkaBootstrapServer,
		KafkaCA:              kafkaCA,
	}

	if a.installACMHub(cluster) {
		manifestsConfig.InstallACMHub = true
		if err := a.setACMPackageConfigs(&manifestsConfig); err != nil {
			return nil, err
		}
	}

	a.setInstallHostedMode(addon, &manifestsConfig)

	return addonfactory.StructToValues(manifestsConfig), nil
}
