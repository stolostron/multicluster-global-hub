/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// ManifestImage contains details for a specific image version
type ManifestImage struct {
	ImageKey     string `json:"image-key"`
	ImageName    string `json:"image-name"`
	ImageVersion string `json:"image-version"`
	// remote registry where image is stored
	ImageRemote string `json:"image-remote"`
	// immutable sha version identifier
	ImageDigest string `json:"image-digest"`
	// image tag, exclude with image digest
	ImageTag string `json:"image-tag"`
}

const (
	GlobalHubAgentImageKey   = "multicluster_global_hub_agent"
	GlobalHubManagerImageKey = "multicluster_global_hub_manager"
	OauthProxyImageKey       = "oauth_proxy"
	GrafanaImageKey          = "grafana"
	ImagePullSecretsKey      = "image_pull_secrets"
	ImagePullPolicyKey       = "image_pull_policy"
)

var (
	hohMGHNamespacedName = types.NamespacedName{}
	imageOverrides       = map[string]string{
		GlobalHubAgentImageKey:   "quay.io/stolostron/multicluster-global-hub-agent:latest",
		GlobalHubManagerImageKey: "quay.io/stolostron/multicluster-global-hub-manager:latest",
		OauthProxyImageKey:       "quay.io/stolostron/origin-oauth-proxy:4.9",
		GrafanaImageKey:          "quay.io/stolostron/grafana:2.8.0-SNAPSHOT-2023-03-06-01-52-34",
		ImagePullSecretsKey:      "",
		ImagePullPolicyKey:       "Always",
	}
)

// GetDefaultNamespace returns default installation namespace
func GetDefaultNamespace() string {
	defaultNamespace, _ := os.LookupEnv("POD_NAMESPACE")
	if defaultNamespace == "" {
		defaultNamespace = constants.GHDefaultNamespace
	}
	return defaultNamespace
}

func SetHoHMGHNamespacedName(namespacedName types.NamespacedName) {
	hohMGHNamespacedName = namespacedName
}

func GetHoHMGHNamespacedName() types.NamespacedName {
	return hohMGHNamespacedName
}

// getAnnotation returns the annotation value for a given key, or an empty string if not set
func getAnnotation(mgh *operatorv1alpha2.MulticlusterGlobalHub, annotationKey string) string {
	annotations := mgh.GetAnnotations()
	if annotations == nil {
		return ""
	}

	return annotations[annotationKey]
}

// IsPaused returns true if the MulticlusterGlobalHub instance is annotated as paused, and false otherwise
func IsPaused(mgh *operatorv1alpha2.MulticlusterGlobalHub) bool {
	isPausedVal := getAnnotation(mgh, operatorconstants.AnnotationMGHPause)
	if isPausedVal != "" && strings.EqualFold(isPausedVal, "true") {
		return true
	}

	return false
}

// SkipDBInit returns true if the MulticlusterGlobalHub instance is annotated as skipping database initialization,
// and false otherwise, used in dev/test environment
func SkipDBInit(mgh *operatorv1alpha2.MulticlusterGlobalHub) bool {
	toSkipDBInit := getAnnotation(mgh, operatorconstants.AnnotationMGHSkipDBInit)
	if toSkipDBInit != "" && strings.EqualFold(toSkipDBInit, "true") {
		return true
	}

	return false
}

// GetImageOverridesConfigmap returns the images override configmap annotation, or an empty string if not set
func GetImageOverridesConfigmap(mgh *operatorv1alpha2.MulticlusterGlobalHub) string {
	return getAnnotation(mgh, operatorconstants.AnnotationImageOverridesCM)
}

func SetImageOverrides(mgh *operatorv1alpha2.MulticlusterGlobalHub, cm *corev1.ConfigMap) error {
	// image pull secret and policy
	if mgh.Spec.ImagePullSecret != "" {
		imageOverrides[ImagePullSecretsKey] = mgh.Spec.ImagePullSecret
	}
	if mgh.Spec.ImagePullPolicy != "" {
		imageOverrides[ImagePullPolicyKey] = string(mgh.Spec.ImagePullPolicy)
	}

	// first check for environment variables containing the 'OPERAND_IMAGE_' prefix
	for _, env := range os.Environ() {
		envKeyVal := strings.SplitN(env, "=", 2)
		if strings.HasPrefix(envKeyVal[0], operatorconstants.MGHOperandImagePrefix) {
			key := strings.ToLower(strings.Replace(envKeyVal[0],
				operatorconstants.MGHOperandImagePrefix, "", -1))
			imageOverrides[key] = envKeyVal[1]
		}
	}

	// second override image repo
	imageRepoOverride := getAnnotation(mgh, operatorconstants.AnnotationImageRepo)
	if imageRepoOverride != "" {
		for imageKey, imageRef := range imageOverrides {
			imageIndex := strings.LastIndex(imageRef, "/")
			imageOverrides[imageKey] = fmt.Sprintf("%s%s", imageRepoOverride, imageRef[imageIndex:])
		}
	}

	// third override images from a given configmap
	if cm == nil {
		// return if override configmap is nil
		return nil
	}

	if len(cm.Data) != 1 {
		return nil
	}

	for _, imageManifests := range cm.Data {
		var manifestImages []ManifestImage
		if err := json.Unmarshal([]byte(imageManifests), &manifestImages); err != nil {
			return err
		}

		for _, manifestImage := range manifestImages {
			if manifestImage.ImageDigest != "" {
				imageOverrides[manifestImage.ImageKey] = fmt.Sprintf("%s/%s@%s", manifestImage.ImageRemote,
					manifestImage.ImageName, manifestImage.ImageDigest)
			} else if manifestImage.ImageTag != "" {
				imageOverrides[manifestImage.ImageKey] = fmt.Sprintf("%s/%s:%s", manifestImage.ImageRemote,
					manifestImage.ImageName, manifestImage.ImageTag)
			}
		}
	}

	return nil
}

// GetImage is used to retrieve image for given component
func GetImage(componentName string) string {
	return imageOverrides[componentName]
}
