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
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/types"

	"github.com/stolostron/multicluster-globalhub/operator/pkg/constants"
)

var (
	hohMGHNamespacedName = types.NamespacedName{}
	imageManifests       = map[string]string{
		"hub_of_hubs_agent":       "quay.io/stolostron/multicluster-globalhub-agent:latest",
		"hub_of_hubs_manager":     "quay.io/stolostron/multicluster-globalhub-manager:latest",
		"hub_of_hubs_console_job": "quay.io/stolostron/multicluster-globalhub-operator:latest",
		"hub_of_hubs_rbac":        "quay.io/open-cluster-management-hub-of-hubs/hub-of-hubs-rbac:latest",
	}
)

func SetHoHMGHNamespacedName(namespacedName types.NamespacedName) {
	hohMGHNamespacedName = namespacedName
}

func GetHoHMGHNamespacedName() types.NamespacedName {
	return hohMGHNamespacedName
}

// GetImageManifests...
func GetImageManifests() map[string]string {
	return imageManifests
}

// SetImageManifests sets imageManifests
func SetImageManifests(images map[string]string) {
	imageManifests = images
}

// GetImage is used to retrieve image for given component based on multiclusterglobalhub annotation
// TODO: validate the image format is correct before return
func GetImage(annotations map[string]string, componentName string) string {
	if annotations != nil {
		// dev/test only. e.g.
		// use annotation key "hoh-hub_of_hubs_manager-image" to override multicluster-globalhub-manager image
		componentImage, overrideImage := annotations["hoh-"+componentName+"-image"]
		if overrideImage && componentImage != "" {
			return componentImage
		}

		annotationImageRepo, overrideImageRepo := annotations[constants.HoHImageRepoAnnotationKey]
		if !overrideImageRepo || annotationImageRepo == "" {
			annotationImageRepo = constants.HoHDefaultImageRepository
		}

		annotationImageTag, overrideImageTag := annotations[constants.HoHImageTagAnnotationKey]
		if overrideImageTag && annotationImageTag != "" {
			imageName := strings.ReplaceAll(componentName, "_", "-")
			return fmt.Sprintf("%s/%s:%s", annotationImageRepo, imageName, annotationImageTag)
		}
	}

	return imageManifests[componentName]
}
