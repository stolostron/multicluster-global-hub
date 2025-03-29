package addon

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func setACMPackageConfigs(ctx context.Context, manifestsConfig *config.ManifestsConfig,
	cluster *clusterv1.ManagedCluster, dynamicClient dynamic.Interface,
) error {
	if _, exist := cluster.GetLabels()[operatorconstants.GHAgentACMHubInstallLabelKey]; !exist {
		return nil
	}

	manifestsConfig.InstallACMHub = false
	for _, claim := range cluster.Status.ClusterClaims {
		if claim.Name != constants.HubClusterClaimName {
			continue
		}
		if claim.Value == constants.HubNotInstalled ||
			claim.Value == constants.HubInstalledByGlobalHub {
			manifestsConfig.InstallACMHub = true
		}
	}

	if !manifestsConfig.InstallACMHub {
		return nil
	}

	log.Infow("installing ACM on managed hub", "cluster", cluster.Name)

	pm, err := GetPackageManifestConfig(ctx, dynamicClient)
	if err != nil {
		return err
	}
	manifestsConfig.Channel = pm.ACMDefaultChannel
	manifestsConfig.CurrentCSV = pm.ACMCurrentCSV
	manifestsConfig.Source = operatorconstants.ACMSubscriptionPublicSource
	manifestsConfig.SourceNamespace = operatorconstants.OpenshiftMarketPlaceNamespace
	return nil
}

type packageManifestConfig struct {
	ACMDefaultChannel string
	ACMCurrentCSV     string
	ACMImages         map[string]string
	MCEDefaultChannel string
	MCECurrentCSV     string
	MCEImages         map[string]string
}

var packageManifestConfigInstance = &packageManifestConfig{}

func SetPackageManifestConfig(acmDefaultChannel, acmCurrentCSV, mceDefaultChannel, mceCurrentCSV string,
	acmImages, mceImages map[string]string,
) {
	packageManifestConfigInstance = &packageManifestConfig{
		ACMDefaultChannel: acmDefaultChannel,
		ACMCurrentCSV:     acmCurrentCSV,
		ACMImages:         acmImages,
		MCEDefaultChannel: mceDefaultChannel,
		MCECurrentCSV:     mceCurrentCSV,
		MCEImages:         mceImages,
	}
}

func GetPackageManifestConfig(ctx context.Context, dynClient dynamic.Interface) (*packageManifestConfig, error) {
	if packageManifestConfigInstance.ACMDefaultChannel != "" &&
		packageManifestConfigInstance.ACMCurrentCSV != "" &&
		len(packageManifestConfigInstance.ACMImages) != 0 &&
		packageManifestConfigInstance.MCEDefaultChannel != "" &&
		packageManifestConfigInstance.MCECurrentCSV != "" &&
		len(packageManifestConfigInstance.MCEImages) != 0 {
		return packageManifestConfigInstance, nil
	}

	pmClient := dynClient.Resource(schema.GroupVersionResource{
		Group:    "packages.operators.coreos.com",
		Version:  "v1",
		Resource: "packagemanifests",
	})

	packageManifestList, err := pmClient.List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("catalog=%s", operatorconstants.ACMSubscriptionPublicSource),
	})
	if err != nil {
		return nil, err
	}

	for _, pm := range packageManifestList.Items {
		if pm.GetName() == operatorconstants.ACMPackageManifestName {
			log.Debug("found ACM PackageManifest")
			acmDefaultChannel, acmCurrentCSV, ACMImages, err := getSinglePackageManifestConfig(pm)
			if err != nil {
				return nil, err
			}

			packageManifestConfigInstance.ACMDefaultChannel = acmDefaultChannel
			packageManifestConfigInstance.ACMCurrentCSV = acmCurrentCSV
			packageManifestConfigInstance.ACMImages = ACMImages
		}
		if pm.GetName() == operatorconstants.MCEPackageManifestName {
			log.Debug("found MCE PackageManifest")
			mceDefaultChannel, mceCurrentCSV, MCEImages, err := getSinglePackageManifestConfig(pm)
			if err != nil {
				return nil, err
			}

			packageManifestConfigInstance.MCEDefaultChannel = mceDefaultChannel
			packageManifestConfigInstance.MCECurrentCSV = mceCurrentCSV
			packageManifestConfigInstance.MCEImages = MCEImages
		}
	}

	return packageManifestConfigInstance, nil
}

func getSinglePackageManifestConfig(pm unstructured.Unstructured) (string, string, map[string]string, error) {
	statusObj := pm.Object["status"].(map[string]interface{})
	defaultChannel := statusObj["defaultChannel"].(string)
	channels := statusObj["channels"].([]interface{})
	currentCSV := ""
	images := map[string]string{}
	for _, channel := range channels {
		if channel.(map[string]interface{})["name"].(string) == defaultChannel {
			currentCSV = channel.(map[string]interface{})["currentCSV"].(string)
			// retrieve the related images
			currentCSVDesc := channel.(map[string]interface{})["currentCSVDesc"].(map[string]interface{})
			relatedImages := currentCSVDesc["relatedImages"].([]interface{})
			for _, img := range relatedImages {
				imgStr := img.(string)
				var imgStrs []string
				if strings.Contains(imgStr, "@") {
					imgStrs = strings.Split(imgStr, "@")
				} else if strings.Contains(imgStr, ":") {
					imgStrs = strings.Split(imgStr, ":")
				} else {
					return "", "", images, fmt.Errorf("invalid image format: %s in packagemanifest", img)
				}
				if len(imgStrs) != 2 {
					return "", "", images, fmt.Errorf("invalid image format: %s in packagemanifest", img)
				}
				imgNameStrs := strings.Split(imgStrs[0], "/")
				imageName := imgNameStrs[len(imgNameStrs)-1]
				imageKey := strings.TrimSuffix(imageName, "-rhel8")
				images[imageKey] = imgStr
			}
			break
		}
	}

	return defaultChannel, currentCSV, images, nil
}
