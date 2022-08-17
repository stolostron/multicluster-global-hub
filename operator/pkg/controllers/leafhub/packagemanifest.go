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

package leafhub

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
)

type packageManifestConfig struct {
	ACMDefaultChannel string
	ACMCurrentCSV     string
	ACMImages         map[string]string
	MCEDefaultChannel string
	MCECurrentCSV     string
	MCEImages         map[string]string
}

var packageManifestConfigInstance = &packageManifestConfig{}

func getPackageManifestConfig(ctx context.Context, dynClient dynamic.Interface,
	log logr.Logger,
) (*packageManifestConfig, error) {
	pmClient := dynClient.Resource(schema.GroupVersionResource{
		Group:    "packages.operators.coreos.com",
		Version:  "v1",
		Resource: "packagemanifests",
	})

	packageManifestList, err := pmClient.List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("catalog=%s", constants.ACMSubscriptionPublicSource),
	})
	if err != nil {
		return nil, err
	}

	for _, pm := range packageManifestList.Items {
		if pm.GetName() == constants.ACMPackageManifestName {
			log.Info("found ACM PackageManifest")
			acmDefaultChannel, acmCurrentCSV, ACMImages, err := getSinglePackageManifestConfig(pm)
			if err != nil {
				return nil, err
			}

			packageManifestConfigInstance.ACMDefaultChannel = acmDefaultChannel
			packageManifestConfigInstance.ACMCurrentCSV = acmCurrentCSV
			packageManifestConfigInstance.ACMImages = ACMImages
		}
		if pm.GetName() == constants.MCEPackageManifestName {
			log.Info("found MCE PackageManifest")
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
