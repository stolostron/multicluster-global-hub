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
	operatorsv1 "github.com/operator-framework/operator-lifecycle-manager/pkg/package-server/apis/operators/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-globalhub/operator/pkg/constants"
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

func getPackageManifestConfig(ctx context.Context, c client.Client, log logr.Logger) (*packageManifestConfig, error) {
	packageManifestList := &operatorsv1.PackageManifestList{}
	if err := c.List(ctx, packageManifestList,
		client.MatchingLabels{"catalog": constants.ACMSubscriptionPublicSource}); err != nil {
		return nil, err
	}

	for _, pm := range packageManifestList.Items {
		if pm.Name == constants.ACMPackageManifestName {
			log.Info("found ACM PackageManifest")
			acmDefaultChannel, acmCurrentCSV, ACMImages, err := getSinglePackageManifestConfig(pm)
			if err != nil {
				return nil, err
			}

			packageManifestConfigInstance.ACMDefaultChannel = acmDefaultChannel
			packageManifestConfigInstance.ACMCurrentCSV = acmCurrentCSV
			packageManifestConfigInstance.ACMImages = ACMImages
		}
		if pm.Name == constants.MCEPackageManifestName {
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

func getSinglePackageManifestConfig(pm operatorsv1.PackageManifest) (string, string, map[string]string, error) {
	defaultChannel := pm.Status.DefaultChannel
	currentCSV := ""
	images := map[string]string{}
	for _, channel := range pm.Status.Channels {
		if channel.Name == defaultChannel {
			currentCSV = channel.CurrentCSV
			relatedImages := channel.CurrentCSVDesc.RelatedImages
			for _, img := range relatedImages {
				var imgStrs []string
				if strings.Contains(img, "@") {
					imgStrs = strings.Split(img, "@")
				} else if strings.Contains(img, ":") {
					imgStrs = strings.Split(img, ":")
				} else {
					return "", "", images, fmt.Errorf("invalid image format: %s in packagemanifest", img)
				}
				if len(imgStrs) != 2 {
					return "", "", images, fmt.Errorf("invalid image format: %s in packagemanifest", img)
				}
				imgNameStrs := strings.Split(imgStrs[0], "/")
				imageName := imgNameStrs[len(imgNameStrs)-1]
				imageKey := strings.TrimSuffix(imageName, "-rhel8")
				images[imageKey] = img
			}
			break
		}
	}

	return defaultChannel, currentCSV, images, nil
}
