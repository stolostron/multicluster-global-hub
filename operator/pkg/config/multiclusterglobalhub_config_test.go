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
	"context"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configv1 "github.com/openshift/api/config/v1"
	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSetImageOverrides(t *testing.T) {
	mghInstanceName := "multiclusterglobalhub"
	tests := []struct {
		desc               string
		initImageManifests map[string]string
		operandImagesEnv   map[string]string
		mghInstance        *globalhubv1alpha4.MulticlusterGlobalHub
		wantImageManifests map[string]string
		wantErr            error
	}{
		{
			desc: "no any override",
			initImageManifests: map[string]string{
				"multicluster_global_hub_agent":   "quay.io/stolostron/multicluster-global-hub-agent:latest",
				"multicluster_global_hub_manager": "quay.io/stolostron/multicluster-global-hub-manager:latest",
				"oauth_proxy":                     "quay.io/stolostron/multicluster-global-hub-operator:latest",
			},
			operandImagesEnv: map[string]string{
				"RELATED_IMAGE_MULTICLUSTER_GLOBAL_HUB_MANAGER": "quay.io/stolostron/multicluster-global-hub-manager:v0.6.0",
				"RELATED_IMAGE_MULTICLUSTER_GLOBAL_HUB_AGENT":   "quay.io/stolostron/multicluster-global-hub-agent:v0.6.0",
				"RELATED_IMAGE_OAUTH_PROXY_415_AND_DOWN":        "quay.io/stolostron/origin-oauth-proxy:4.9",
				"RELATED_IMAGE_OAUTH_PROXY_416_AND_UP":          "quay.io/stolostron/origin-oauth-proxy:4.16",
			},
			mghInstance: &globalhubv1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: utils.GetDefaultNamespace(),
					Name:      mghInstanceName,
				},
				Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
			},
			wantImageManifests: map[string]string{
				"multicluster_global_hub_agent":   "quay.io/stolostron/multicluster-global-hub-agent:v0.6.0",
				"multicluster_global_hub_manager": "quay.io/stolostron/multicluster-global-hub-manager:v0.6.0",
				"oauth_proxy":                     "quay.io/stolostron/origin-oauth-proxy:4.16",
			},
			wantErr: nil,
		},
		{
			desc: "override image repo",
			initImageManifests: map[string]string{
				"multicluster_global_hub_agent":   "quay.io/stolostron/multicluster-global-hub-agent:latest",
				"multicluster_global_hub_manager": "quay.io/stolostron/multicluster-global-hub-manager:latest",
				"oauth_proxy":                     "quay.io/stolostron/multicluster-global-hub-operator:latest",
			},
			operandImagesEnv: map[string]string{
				"RELATED_IMAGE_MULTICLUSTER_GLOBAL_HUB_MANAGER": "quay.io/stolostron/multicluster-global-hub-manager:v0.6.0",
				"RELATED_IMAGE_MULTICLUSTER_GLOBAL_HUB_AGENT":   "quay.io/stolostron/multicluster-global-hub-agent:v0.6.0",
				"RELATED_IMAGE_OAUTH_PROXY_415_AND_DOWN":        "quay.io/stolostron/origin-oauth-proxy:4.9",
				"RELATED_IMAGE_OAUTH_PROXY_416_AND_UP":          "quay.io/stolostron/origin-oauth-proxy:4.16",
			},
			mghInstance: &globalhubv1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: utils.GetDefaultNamespace(),
					Name:      mghInstanceName,
					Annotations: map[string]string{
						operatorconstants.AnnotationImageRepo: "quay.io/testing",
					},
				},
				Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
			},
			wantImageManifests: map[string]string{
				"multicluster_global_hub_agent":   "quay.io/testing/multicluster-global-hub-agent:v0.6.0",
				"multicluster_global_hub_manager": "quay.io/testing/multicluster-global-hub-manager:v0.6.0",
				"oauth_proxy":                     "quay.io/testing/origin-oauth-proxy:4.9",
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		// set env for operand images
		for k, v := range tt.operandImagesEnv {
			t.Setenv(k, v)
		}
		// set init imageManifests
		imageOverrides = tt.initImageManifests
		t.Run(tt.desc, func(t *testing.T) {
			err := SetImageOverrides(tt.mghInstance)
			if ((err != nil && tt.wantErr == nil) || (err == nil && tt.wantErr != nil)) ||
				!reflect.DeepEqual(imageOverrides, tt.wantImageManifests) {
				t.Errorf("%s:\nwanted imageManifests & err:\n%+v\n%v\ngot imageManifests & err \n%+v\n%v",
					tt.desc, tt.wantImageManifests, tt.wantErr, imageOverrides, err)
			}
		})
	}
}

func TestGetOauthSessionSecret(t *testing.T) {
	secret1, err := GetOauthSessionSecret()
	if err != nil {
		t.Errorf("failed to get oauth session secret: %v", err)
	}
	secret2, err := GetOauthSessionSecret()
	if err != nil {
		t.Errorf("failed to get oauth session secret: %v", err)
	}
	if secret1 != secret2 {
		t.Errorf("oauth session secret is not consistent")
	}
}

func TestOauthProxyImage(t *testing.T) {
	cases := []struct {
		desc           string
		clusterVersion runtime.Object
		expectErr      string
		expectImage    string
	}{
		{
			desc:           "Retrieve image without clusterVersion",
			clusterVersion: &configv1.ClusterVersion{},
			expectErr:      "not found",
			expectImage:    imageOverrides[OauthProxyImage415DownKey],
		},
		{
			desc: "Retrieve image without clusterVersion status",
			clusterVersion: &configv1.ClusterVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: "version",
				},
			},
			expectErr:   "failed to get the status history form clusterversions(version)",
			expectImage: imageOverrides[OauthProxyImage415DownKey],
		},
		{
			desc: "Retrieve image with clusterVersion 4.14.20",
			clusterVersion: &configv1.ClusterVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: "version",
				},
				Status: configv1.ClusterVersionStatus{
					History: []configv1.UpdateHistory{{
						Version: "4.14.20",
					}},
				},
			},
			expectErr:   "",
			expectImage: imageOverrides[OauthProxyImage415DownKey],
		},
		{
			desc: "Retrieve image with clusterVersion 4.16.20",
			clusterVersion: &configv1.ClusterVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: "version",
				},
				Status: configv1.ClusterVersionStatus{
					History: []configv1.UpdateHistory{{
						Version: "4.16.20",
					}},
				},
			},
			expectErr:   "",
			expectImage: imageOverrides[OauthProxyImage416UpKey],
		},
	}

	for _, tc := range cases {
		fakeClient := fake.NewClientBuilder().WithScheme(GetRuntimeScheme()).WithRuntimeObjects(tc.clusterVersion).Build()
		image, err := GetOauthProxyImage(context.Background(), fakeClient)
		if err != nil {
			assert.ErrorContains(t, err, tc.expectErr)
		}
		assert.Equal(t, tc.expectImage, image)
	}
}
