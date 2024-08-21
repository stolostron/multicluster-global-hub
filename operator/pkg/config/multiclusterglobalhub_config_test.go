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

	imagev1 "github.com/openshift/api/image/v1"
	fakeimageclient "github.com/openshift/client-go/image/clientset/versioned/fake"
	fakeimagev1client "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1/fake"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
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
			},
			operandImagesEnv: map[string]string{
				"RELATED_IMAGE_MULTICLUSTER_GLOBAL_HUB_MANAGER": "quay.io/stolostron/multicluster-global-hub-manager:v0.6.0",
				"RELATED_IMAGE_MULTICLUSTER_GLOBAL_HUB_AGENT":   "quay.io/stolostron/multicluster-global-hub-agent:v0.6.0",
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
			},
			wantErr: nil,
		},
		{
			desc: "override image repo",
			initImageManifests: map[string]string{
				"multicluster_global_hub_agent":   "quay.io/stolostron/multicluster-global-hub-agent:latest",
				"multicluster_global_hub_manager": "quay.io/stolostron/multicluster-global-hub-manager:latest",
				"oauth_proxy":                     "quay.io/testing/origin-oauth-proxy:4.9",
			},
			operandImagesEnv: map[string]string{
				"RELATED_IMAGE_MULTICLUSTER_GLOBAL_HUB_MANAGER": "quay.io/stolostron/multicluster-global-hub-manager:v0.6.0",
				"RELATED_IMAGE_MULTICLUSTER_GLOBAL_HUB_AGENT":   "quay.io/stolostron/multicluster-global-hub-agent:v0.6.0",
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

func TestSetMulticlusterGlobalHubConfig(t *testing.T) {
	oauthImage := "quay.io/testing/origin-oauth-proxy:4.9"
	mghInstance := &globalhubv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: utils.GetDefaultNamespace(),
			Name:      "test",
			Annotations: map[string]string{
				operatorconstants.AnnotationImageRepo: "quay.io/testing",
			},
		},
		Spec: globalhubv1alpha4.MulticlusterGlobalHubSpec{},
	}

	err := SetMulticlusterGlobalHubConfig(context.Background(), mghInstance, nil)
	if err != nil {
		t.Fatal(err)
	}

	if GetImage(OauthProxyImageKey) != oauthImage {
		t.Fatalf("oauth proxy image is not expected one")
	}

	imageClient := &fakeimagev1client.FakeImageV1{Fake: &(fakeimageclient.NewSimpleClientset().Fake)}
	err = SetMulticlusterGlobalHubConfig(context.Background(), mghInstance, imageClient)
	if err != nil {
		t.Fatal(err)
	}
	if GetImage(OauthProxyImageKey) != oauthImage {
		t.Fatalf("oauth proxy image is not expected one")
	}

	_, err = imageClient.ImageStreams(operatorconstants.OauthProxyImageStreamNamespace).Create(context.Background(),
		&imagev1.ImageStream{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operatorconstants.OauthProxyImageStreamName,
				Namespace: operatorconstants.OauthProxyImageStreamNamespace,
			},
			Spec: imagev1.ImageStreamSpec{
				Tags: []imagev1.TagReference{
					{
						Name: "v4.4",
						From: &corev1.ObjectReference{
							Kind: "DockerImage",
							Name: "quay.io/openshift-release-dev/ocp-v4.0-art-dev",
						},
					},
				},
			},
		}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	err = SetMulticlusterGlobalHubConfig(context.Background(), mghInstance, imageClient)
	if err != nil {
		t.Fatal(err)
	}
	if GetImage(OauthProxyImageKey) != "quay.io/openshift-release-dev/ocp-v4.0-art-dev" {
		t.Fatalf("oauth proxy image is not expected one")
	}
}
