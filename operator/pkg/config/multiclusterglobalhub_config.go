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
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
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
	GlobalHubAgentImageKey       = "multicluster_global_hub_agent"
	GlobalHubManagerImageKey     = "multicluster_global_hub_manager"
	OauthProxyImageKey           = "oauth_proxy"
	GrafanaImageKey              = "grafana"
	PostgresImageKey             = "postgresql"
	PostgresExporterImageKey     = "postgres_exporter"
	GHPostgresDefaultStorageSize = "25Gi"
	// default values for the global hub configured by the operator
	// We may expose these as CRD fields in the future
	AggregationLevel       = "full"
	EnableLocalPolicies    = "true"
	AgentHeartbeatInterval = "60s"
)

var (
	mghNamespacedName  = types.NamespacedName{}
	oauthSessionSecret = ""
	imageOverrides     = map[string]string{
		GlobalHubAgentImageKey:   "quay.io/stolostron/multicluster-global-hub-agent:latest",
		GlobalHubManagerImageKey: "quay.io/stolostron/multicluster-global-hub-manager:latest",
		OauthProxyImageKey:       "quay.io/stolostron/origin-oauth-proxy:4.16",
		GrafanaImageKey:          "quay.io/stolostron/grafana:globalhub-1.2",
		PostgresImageKey:         "quay.io/stolostron/postgresql-13:1-101",
		PostgresExporterImageKey: "quay.io/prometheuscommunity/postgres-exporter:v0.15.0",
	}
	statisticLogInterval  = "1m"
	metricsScrapeInterval = "1m"
	imagePullSecretName   = ""
)

func SetMGHNamespacedName(namespacedName types.NamespacedName) {
	mghNamespacedName = namespacedName
}

func GetMGHNamespacedName() types.NamespacedName {
	return mghNamespacedName
}

func GetOauthSessionSecret() (string, error) {
	if oauthSessionSecret == "" {
		b := make([]byte, 16)
		_, err := rand.Read(b)
		if err != nil {
			return "", err
		}
		oauthSessionSecret = base64.StdEncoding.EncodeToString(b)
	}
	return oauthSessionSecret, nil
}

// getAnnotation returns the annotation value for a given key, or an empty string if not set
func getAnnotation(mgh *v1alpha4.MulticlusterGlobalHub, annotationKey string) string {
	annotations := mgh.GetAnnotations()
	if annotations == nil {
		return ""
	}

	return annotations[annotationKey]
}

// IsPaused returns true if the MulticlusterGlobalHub instance is annotated as paused, and false otherwise
func IsPaused(mgh *v1alpha4.MulticlusterGlobalHub) bool {
	isPausedVal := getAnnotation(mgh, operatorconstants.AnnotationMGHPause)
	if isPausedVal != "" && strings.EqualFold(isPausedVal, "true") {
		return true
	}

	return false
}

// GetSchedulerInterval returns the scheduler interval for moving policy compliance history
func GetSchedulerInterval(mgh *v1alpha4.MulticlusterGlobalHub) string {
	return getAnnotation(mgh, operatorconstants.AnnotationMGHSchedulerInterval)
}

// SkipAuth returns true to skip authenticate for non-k8s api
func SkipAuth(mgh *v1alpha4.MulticlusterGlobalHub) bool {
	toSkipAuth := getAnnotation(mgh, operatorconstants.AnnotationMGHSkipAuth)
	if toSkipAuth != "" && strings.EqualFold(toSkipAuth, "true") {
		return true
	}

	return false
}

func GetInstallCrunchyOperator(mgh *v1alpha4.MulticlusterGlobalHub) bool {
	toInstallCrunchyOperator := getAnnotation(mgh, operatorconstants.AnnotationMGHInstallCrunchyOperator)
	if toInstallCrunchyOperator != "" && strings.EqualFold(toInstallCrunchyOperator, "true") {
		return true
	}

	return false
}

// GetLaunchJobNames returns the jobs concatenated using "," wchich will run once the constainer is started
func GetLaunchJobNames(mgh *v1alpha4.MulticlusterGlobalHub) string {
	return getAnnotation(mgh, operatorconstants.AnnotationLaunchJobNames)
}

// GetImageOverridesConfigmap returns the images override configmap annotation, or an empty string if not set
func GetImageOverridesConfigmap(mgh *v1alpha4.MulticlusterGlobalHub) string {
	return getAnnotation(mgh, operatorconstants.AnnotationImageOverridesCM)
}

func SetImageOverrides(mgh *v1alpha4.MulticlusterGlobalHub) error {
	// first check for environment variables containing the 'RELATED_IMAGE_' prefix
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
	return nil
}

// GetImage is used to retrieve image for given component
func GetImage(componentName string) string {
	return imageOverrides[componentName]
}

func SetStatisticLogInterval(mgh *v1alpha4.MulticlusterGlobalHub) error {
	interval := getAnnotation(mgh, operatorconstants.AnnotationStatisticInterval)
	if interval == "" {
		return nil
	}

	_, err := time.ParseDuration(interval)
	if err != nil {
		return err
	}
	statisticLogInterval = interval
	return nil
}

func GetStatisticLogInterval() string {
	return statisticLogInterval
}

func GetMetricsScrapeInterval(mgh *v1alpha4.MulticlusterGlobalHub) string {
	interval := getAnnotation(mgh, operatorconstants.AnnotationMetricsScrapeInterval)
	if interval == "" {
		interval = metricsScrapeInterval
	}
	return interval
}

func GetPostgresStorageSize(mgh *v1alpha4.MulticlusterGlobalHub) string {
	if mgh.Spec.DataLayer.Postgres.StorageSize != "" {
		return mgh.Spec.DataLayer.Postgres.StorageSize
	}
	return GHPostgresDefaultStorageSize
}

func SetImagePullSecretName(mgh *v1alpha4.MulticlusterGlobalHub) {
	if mgh.Spec.ImagePullSecret != imagePullSecretName {
		imagePullSecretName = mgh.Spec.ImagePullSecret
	}
}

func GetImagePullSecretName() string {
	return imagePullSecretName
}

// GetMulticlusterGlobalHub will get the CR and also update the configuration based on it
func GetMulticlusterGlobalHub(ctx context.Context, req ctrl.Request,
	c client.Client,
) (*v1alpha4.MulticlusterGlobalHub, error) {
	mgh := &v1alpha4.MulticlusterGlobalHub{}
	err := c.Get(ctx, req.NamespacedName, mgh)
	if err != nil {
		return nil, err
	}
	err = SetMulticlusterGlobalHubConfig(mgh)
	if err != nil {
		return nil, err
	}
	err = SetBYOKafka(ctx, c, mgh.GetNamespace())
	if err != nil {
		return nil, err
	}
	err = SetBYOPostgres(ctx, c, mgh.GetNamespace())
	if err != nil {
		return nil, err
	}
	return mgh, nil
}

// SetMulticlusterGlobalHubConfig extract the namespacedName, image, and log configurations from CR
func SetMulticlusterGlobalHubConfig(mgh *v1alpha4.MulticlusterGlobalHub) error {
	// set request name to be used in leafhub controller
	SetMGHNamespacedName(types.NamespacedName{
		Namespace: mgh.GetNamespace(), Name: mgh.GetName(),
	})

	// set image overrides
	if err := SetImageOverrides(mgh); err != nil {
		return err
	}

	// set image pull secret
	SetImagePullSecretName(mgh)

	// set statistic log interval
	if err := SetStatisticLogInterval(mgh); err != nil {
		return err
	}
	return nil
}
