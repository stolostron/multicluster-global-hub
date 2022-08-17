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
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/apis/operator/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
)

func TestSetImageOverrides(t *testing.T) {
	mghInstanceName := "multiclusterglobalhub"
	tests := []struct {
		desc               string
		initImageManifests map[string]string
		operandImagesEnv   map[string]string
		mghInstance        *operatorv1alpha1.MultiClusterGlobalHub
		imageOverrideCM    *corev1.ConfigMap
		wantImageManifests map[string]string
		wantErr            error
	}{
		{
			desc: "no any override",
			initImageManifests: map[string]string{
				"multicluster_global_hub_agent":    "quay.io/stolostron/multicluster-global-hub-agent:latest",
				"multicluster_global_hub_manager":  "quay.io/stolostron/multicluster-global-hub-manager:latest",
				"multicluster_global_hub_operator": "quay.io/stolostron/multicluster-global-hub-operator:latest",
				"multicluster_global_hub_rbac":     "quay.io/stolostron/multicluster-global-hub-rbac:latest",
			},
			operandImagesEnv: map[string]string{
				"OPERAND_IMAGE_MULTICLUSTER_GLOBAL_HUB_MANAGER":  "quay.io/stolostron/multicluster-global-hub-manager:v0.6.0",
				"OPERAND_IMAGE_MULTICLUSTER_GLOBAL_HUB_AGENT":    "quay.io/stolostron/multicluster-global-hub-agent:v0.6.0",
				"OPERAND_IMAGE_MULTICLUSTER_GLOBAL_HUB_OPERATOR": "quay.io/stolostron/multicluster-global-hub-operator:v0.6.0",
				"OPERAND_IMAGE_MULTICLUSTER_GLOBAL_HUB_RBAC":     "quay.io/stolostron/multicluster-global-hub-rbac:v0.6.0",
			},
			mghInstance: &operatorv1alpha1.MultiClusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: constants.HOHDefaultNamespace,
					Name:      mghInstanceName,
				},
				Spec: operatorv1alpha1.MultiClusterGlobalHubSpec{},
			},
			imageOverrideCM: nil,
			wantImageManifests: map[string]string{
				"multicluster_global_hub_agent":    "quay.io/stolostron/multicluster-global-hub-agent:v0.6.0",
				"multicluster_global_hub_manager":  "quay.io/stolostron/multicluster-global-hub-manager:v0.6.0",
				"multicluster_global_hub_operator": "quay.io/stolostron/multicluster-global-hub-operator:v0.6.0",
				"multicluster_global_hub_rbac":     "quay.io/stolostron/multicluster-global-hub-rbac:v0.6.0",
			},
			wantErr: nil,
		},
		{
			desc: "override image repo",
			initImageManifests: map[string]string{
				"multicluster_global_hub_agent":    "quay.io/stolostron/multicluster-global-hub-agent:latest",
				"multicluster_global_hub_manager":  "quay.io/stolostron/multicluster-global-hub-manager:latest",
				"multicluster_global_hub_operator": "quay.io/stolostron/multicluster-global-hub-operator:latest",
				"multicluster_global_hub_rbac":     "quay.io/stolostron/multicluster-global-hub-rbac:latest",
			},
			operandImagesEnv: map[string]string{
				"OPERAND_IMAGE_MULTICLUSTER_GLOBAL_HUB_MANAGER":  "quay.io/stolostron/multicluster-global-hub-manager:v0.6.0",
				"OPERAND_IMAGE_MULTICLUSTER_GLOBAL_HUB_AGENT":    "quay.io/stolostron/multicluster-global-hub-agent:v0.6.0",
				"OPERAND_IMAGE_MULTICLUSTER_GLOBAL_HUB_OPERATOR": "quay.io/stolostron/multicluster-global-hub-operator:v0.6.0",
				"OPERAND_IMAGE_MULTICLUSTER_GLOBAL_HUB_RBAC":     "quay.io/stolostron/multicluster-global-hub-rbac:v0.6.0",
			},
			mghInstance: &operatorv1alpha1.MultiClusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: constants.HOHDefaultNamespace,
					Name:      mghInstanceName,
					Annotations: map[string]string{
						constants.AnnotationImageRepo: "quay.io/testing",
					},
				},
				Spec: operatorv1alpha1.MultiClusterGlobalHubSpec{},
			},
			imageOverrideCM: nil,
			wantImageManifests: map[string]string{
				"multicluster_global_hub_agent":    "quay.io/testing/multicluster-global-hub-agent:v0.6.0",
				"multicluster_global_hub_manager":  "quay.io/testing/multicluster-global-hub-manager:v0.6.0",
				"multicluster_global_hub_operator": "quay.io/testing/multicluster-global-hub-operator:v0.6.0",
				"multicluster_global_hub_rbac":     "quay.io/testing/multicluster-global-hub-rbac:v0.6.0",
			},
			wantErr: nil,
		},
		{
			desc: "override image from configmap",
			initImageManifests: map[string]string{
				"multicluster_global_hub_agent":    "quay.io/stolostron/multicluster-global-hub-agent:latest",
				"multicluster_global_hub_manager":  "quay.io/stolostron/multicluster-global-hub-manager:latest",
				"multicluster_global_hub_operator": "quay.io/stolostron/multicluster-global-hub-operator:latest",
				"multicluster_global_hub_rbac":     "quay.io/stolostron/multicluster-global-hub-rbac:latest",
			},
			operandImagesEnv: map[string]string{
				"OPERAND_IMAGE_MULTICLUSTER_GLOBAL_HUB_MANAGER":  "quay.io/stolostron/multicluster-global-hub-manager:v0.6.0",
				"OPERAND_IMAGE_MULTICLUSTER_GLOBAL_HUB_AGENT":    "quay.io/stolostron/multicluster-global-hub-agent:v0.6.0",
				"OPERAND_IMAGE_MULTICLUSTER_GLOBAL_HUB_OPERATOR": "quay.io/stolostron/multicluster-global-hub-operator:v0.6.0",
				"OPERAND_IMAGE_MULTICLUSTER_GLOBAL_HUB_RBAC":     "quay.io/stolostron/multicluster-global-hub-rbac:v0.6.0",
			},
			mghInstance: &operatorv1alpha1.MultiClusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: constants.HOHDefaultNamespace,
					Name:      mghInstanceName,
					Annotations: map[string]string{
						constants.AnnotationImageOverridesCM: "mgh-images-config",
					},
				},
				Spec: operatorv1alpha1.MultiClusterGlobalHubSpec{},
			},
			imageOverrideCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: constants.HOHDefaultNamespace,
					Name:      "mgh-images-config",
				},
				Data: map[string]string{
					"manifest.json": `[
  {
	"image-name": "mgh-manager",
	"image-remote": "quay.io/foo",
	"image-digest": "sha256:abcd",
	"image-key": "multicluster_global_hub_manager"
  },
  {
	"image-name": "mgh-agent",
	"image-remote": "quay.io/bar",
	"image-tag": "dev",
	"image-key": "multicluster_global_hub_agent"
  },
  {
	"image-name": "mgh-operator",
	"image-remote": "quay.io/goo",
	"image-digest": "sha256:efgh",
	"image-tag": "test",
	"image-key": "multicluster_global_hub_operator"
  }
]`,
				},
			},
			wantImageManifests: map[string]string{
				"multicluster_global_hub_agent":    "quay.io/bar/mgh-agent:dev",
				"multicluster_global_hub_manager":  "quay.io/foo/mgh-manager@sha256:abcd",
				"multicluster_global_hub_operator": "quay.io/goo/mgh-operator@sha256:efgh",
				"multicluster_global_hub_rbac":     "quay.io/stolostron/multicluster-global-hub-rbac:v0.6.0",
			},
			wantErr: nil,
		},
		{
			desc: "override image repo image manifests from configmap",
			initImageManifests: map[string]string{
				"multicluster_global_hub_agent":    "quay.io/stolostron/multicluster-global-hub-agent:latest",
				"multicluster_global_hub_manager":  "quay.io/stolostron/multicluster-global-hub-manager:latest",
				"multicluster_global_hub_operator": "quay.io/stolostron/multicluster-global-hub-operator:latest",
				"multicluster_global_hub_rbac":     "quay.io/stolostron/multicluster-global-hub-rbac:latest",
			},
			operandImagesEnv: map[string]string{
				"OPERAND_IMAGE_MULTICLUSTER_GLOBAL_HUB_MANAGER":  "quay.io/stolostron/multicluster-global-hub-manager:v0.6.0",
				"OPERAND_IMAGE_MULTICLUSTER_GLOBAL_HUB_AGENT":    "quay.io/stolostron/multicluster-global-hub-agent:v0.6.0",
				"OPERAND_IMAGE_MULTICLUSTER_GLOBAL_HUB_OPERATOR": "quay.io/stolostron/multicluster-global-hub-operator:v0.6.0",
				"OPERAND_IMAGE_MULTICLUSTER_GLOBAL_HUB_RBAC":     "quay.io/stolostron/multicluster-global-hub-rbac:v0.6.0",
			},
			mghInstance: &operatorv1alpha1.MultiClusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: constants.HOHDefaultNamespace,
					Name:      mghInstanceName,
					Annotations: map[string]string{
						constants.AnnotationImageOverridesCM: "mgh-images-config",
						constants.AnnotationImageRepo:        "quay.io/testing",
					},
				},
				Spec: operatorv1alpha1.MultiClusterGlobalHubSpec{},
			},
			imageOverrideCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: constants.HOHDefaultNamespace,
					Name:      "mgh-images-config",
				},
				Data: map[string]string{
					"manifest.json": `[
  {
	"image-name": "mgh-manager",
	"image-remote": "quay.io/foo",
	"image-digest": "sha256:abcd",
	"image-key": "multicluster_global_hub_manager"
  },
  {
	"image-name": "mgh-agent",
	"image-remote": "quay.io/bar",
	"image-tag": "dev",
	"image-key": "multicluster_global_hub_agent"
  },
  {
	"image-name": "mgh-operator",
	"image-remote": "quay.io/goo",
	"image-digest": "sha256:efgh",
	"image-tag": "test",
	"image-key": "multicluster_global_hub_operator"
  }
]`,
				},
			},
			wantImageManifests: map[string]string{
				"multicluster_global_hub_agent":    "quay.io/bar/mgh-agent:dev",
				"multicluster_global_hub_manager":  "quay.io/foo/mgh-manager@sha256:abcd",
				"multicluster_global_hub_operator": "quay.io/goo/mgh-operator@sha256:efgh",
				"multicluster_global_hub_rbac":     "quay.io/testing/multicluster-global-hub-rbac:v0.6.0",
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
			err := SetImageOverrides(tt.mghInstance, tt.imageOverrideCM)
			if ((err != nil && tt.wantErr == nil) || (err == nil && tt.wantErr != nil)) ||
				!reflect.DeepEqual(imageOverrides, tt.wantImageManifests) {
				t.Errorf("%s:\nwanted imageManifests & err:\n%+v\n%v\ngot imageManifests & err \n%+v\n%v",
					tt.desc, tt.wantImageManifests, tt.wantErr, imageOverrides, err)
			}
		})
	}
}
