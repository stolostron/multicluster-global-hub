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
	"github.com/stolostron/hub-of-hubs/operator/pkg/constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type packageManifestConfig struct {
	ACMDefaultChannel string
	ACMCurrentCSV     string
	ACMImages         map[string]string
	MCEDefaultChannel string
	MCECurrentCSV     string
	MCEImages         map[string]string
}

func getPackageManifestConfig(ctx context.Context, c client.Client, log logr.Logger) (*packageManifestConfig, error) {
	packageManifestInstance := &packageManifestConfig{}
	packageManifestList := &operatorsv1.PackageManifestList{}
	if err := c.List(ctx, packageManifestList, client.MatchingLabels{"catalog": constants.ACMSubscriptionPublicSource}); err != nil {
		return nil, err
	}

	for _, pm := range packageManifestList.Items {
		if pm.Name == constants.ACMPackageManifestName {
			log.Info("found ACM PackageManifest")
			acmDefaultChannel := pm.Status.DefaultChannel
			acmCurrentCSV := ""
			ACMImages := map[string]string{}
			for _, channel := range pm.Status.Channels {
				if channel.Name == acmDefaultChannel {
					acmCurrentCSV = channel.CurrentCSV
					acmRelatedImages := channel.CurrentCSVDesc.RelatedImages
					for _, img := range acmRelatedImages {
						var imgStrs []string
						if strings.Contains(img, "@") {
							imgStrs = strings.Split(img, "@")
						} else if strings.Contains(img, ":") {
							imgStrs = strings.Split(img, ":")
						} else {
							return nil, fmt.Errorf("invalid image format: %s in packagemanifest", img)
						}
						if len(imgStrs) != 2 {
							return nil, fmt.Errorf("invalid image format: %s in packagemanifest", img)
						}
						imgNameStrs := strings.Split(imgStrs[0], "/")
						imageName := imgNameStrs[len(imgNameStrs)-1]
						imageKey := strings.TrimSuffix(imageName, "-rhel8")
						ACMImages[imageKey] = img
					}
					break
				}
			}

			packageManifestInstance.ACMDefaultChannel = acmDefaultChannel
			packageManifestInstance.ACMCurrentCSV = acmCurrentCSV
			packageManifestInstance.ACMImages = ACMImages
		}
		if pm.Name == constants.MCEPackageManifestName {
			log.Info("found MCE PackageManifest")
			mceDefaultChannel := pm.Status.DefaultChannel
			mceCurrentCSV := ""
			MCEImages := map[string]string{}
			for _, channel := range pm.Status.Channels {
				if channel.Name == mceDefaultChannel {
					mceCurrentCSV = channel.CurrentCSV
					mceRelatedImages := channel.CurrentCSVDesc.RelatedImages
					for _, img := range mceRelatedImages {
						var imgStrs []string
						if strings.Contains(img, "@") {
							imgStrs = strings.Split(img, "@")
						} else if strings.Contains(img, ":") {
							imgStrs = strings.Split(img, ":")
						} else {
							return nil, fmt.Errorf("invalid image format: %s in packagemanifest", img)
						}
						if len(imgStrs) != 2 {
							return nil, fmt.Errorf("invalid image format: %s in packagemanifest", img)
						}
						imgNameStrs := strings.Split(imgStrs[0], "/")
						imageName := imgNameStrs[len(imgNameStrs)-1]
						imageKey := strings.TrimSuffix(imageName, "-rhel8")
						MCEImages[imageKey] = img
					}
					break
				}
			}

			packageManifestInstance.MCEDefaultChannel = mceDefaultChannel
			packageManifestInstance.MCECurrentCSV = mceCurrentCSV
			packageManifestInstance.MCEImages = MCEImages
		}
	}

	return packageManifestInstance, nil
}
