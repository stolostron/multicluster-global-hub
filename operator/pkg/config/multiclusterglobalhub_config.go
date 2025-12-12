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
	"reflect"
	"strings"
	"sync"
	"time"

	imagev1client "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
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
	InventoryImageKey            = "inventory_api"
	SpiceDBOperatorImageKey      = "spicedb_operator"
	SpiceDBInstanceImageKey      = "spicedb_instance"
	SpiceDBRelationsAPIImageKey  = "relations_api"
	OauthProxyImageKey           = "oauth_proxy"
	GrafanaImageKey              = "grafana"
	PostgresImageKey             = "postgresql"
	PostgresExporterImageKey     = "postgres_exporter"
	GHPostgresDefaultStorageSize = "25Gi"
	// default values for the global hub configured by the operator
	// We may expose these as CRD fields in the future
	AggregationLevel           = "full"
	EnableLocalPolicies        = "true"
	AgentHeartbeatInterval     = "60s"
	RedHatKesselQuayIORegistry = "quay.io/redhat-services-prod/project-kessel-tenant"
)

var (
	mghNamespacedName  = types.NamespacedName{}
	oauthSessionSecret = ""
	imageOverrides     = map[string]string{
		GlobalHubAgentImageKey:      "quay.io/stolostron/multicluster-global-hub-agent:latest",
		GlobalHubManagerImageKey:    "quay.io/stolostron/multicluster-global-hub-manager:latest",
		OauthProxyImageKey:          "quay.io/stolostron/origin-oauth-proxy:4.9",
		GrafanaImageKey:             "quay.io/stolostron/grafana:2.12.0-SNAPSHOT-2024-09-03-21-11-25",
		PostgresImageKey:            "quay.io/stolostron/postgresql-16:9.5-1732622748",
		PostgresExporterImageKey:    "quay.io/prometheuscommunity/postgres-exporter:v0.15.0",
		InventoryImageKey:           fmt.Sprintf("%s/kessel-inventory/inventory-api@sha256:c443e7494d7b1dd4bb24234cf265a3f0fb5e9c3c0e2edeb2f00285a2286ff24f", RedHatKesselQuayIORegistry),
		SpiceDBOperatorImageKey:     fmt.Sprintf("%s/kessel-relations/spicedb-operator@sha256:91813db7bad47fc4fefa8ab4a3a2a2713eea8f8021e5b1039ec79905e3082122", RedHatKesselQuayIORegistry),
		SpiceDBInstanceImageKey:     fmt.Sprintf("%s/kessel-relations/spicedb@sha256:710687d846ad808266bac11e7e2726437cb9faec2cc96c9c3c3883025fb9d9a0", RedHatKesselQuayIORegistry),
		SpiceDBRelationsAPIImageKey: fmt.Sprintf("%s/kessel-relations/relations-api@sha256:fff1d072580a65ee78a82a845eb256a669f67a0b0c1b810811c2a66e6c73b10d", RedHatKesselQuayIORegistry),
	}
	statisticLogInterval  = "1m"
	metricsScrapeInterval = "1m"
	imagePullSecretName   = ""
	addonMgr              addonmanager.AddonManager
	mu                    sync.Mutex
	log                   = logger.DefaultZapLogger()
	localClusterName      = ""
)

func SetLocalClusterName(name string) {
	localClusterName = name
}

func GetLocalClusterName() string {
	return localClusterName
}

func SetAddonManager(addonManager addonmanager.AddonManager) {
	addonMgr = addonManager
}

func GetAddonManager() addonmanager.AddonManager {
	return addonMgr
}

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

var MGHPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return true
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return true
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return true
	},
}

// watch globalhub applied services
var GeneralPredicate = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return false
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		if e.ObjectNew.GetLabels()[constants.GlobalHubOwnerLabelKey] !=
			constants.GHOperatorOwnerLabelVal {
			return false
		}
		return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetLabels()[constants.GlobalHubOwnerLabelKey] ==
			constants.GHOperatorOwnerLabelVal
	},
}

// getAnnotation returns the annotation value for a given key, or an empty string if not set
func getAnnotation(obj client.Object, annotationKey string) string {
	annotations := obj.GetAnnotations()
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

// IsAgentPaused returns true if the MulticlusterGlobalHubAgent instance is annotated as paused, and false otherwise
func IsAgentPaused(mgha *v1alpha1.MulticlusterGlobalHubAgent) bool {
	annotations := mgha.GetAnnotations()
	if annotations == nil {
		return false
	}
	if annotations[operatorconstants.AnnotationMGHPause] != "" &&
		strings.EqualFold(annotations[operatorconstants.AnnotationMGHPause], "true") {
		return true
	}
	return false
}

// WithStackroxIntegration returns true if the integration with Stackrox is enabled.
func WithStackroxIntegration(mgh *v1alpha4.MulticlusterGlobalHub) bool {
	_, ok := mgh.GetAnnotations()[operatorconstants.AnnotationMGHWithStackroxIntegration]
	return ok
}

// GetStackroxPollInterval returns the StackRox API poll interval specified in the annotations of the given object. The
// value should be a string that can be parsed with the time.ParseDuration function. If it isn't specified or the format
// isn't valid, then it returns zero.
func GetStackroxPollInterval(mgh *v1alpha4.MulticlusterGlobalHub) time.Duration {
	text, ok := mgh.GetAnnotations()[operatorconstants.AnnotationMGHWithStackroxPollInterval]
	if !ok {
		return 0
	}
	value, err := time.ParseDuration(text)
	if err != nil {
		log.Errorf(
			"Failed to parse value '%s' of annotation '%s', will ignore it: %v",
			text, operatorconstants.AnnotationMGHWithStackroxPollInterval, err,
		)
		return 0
	}
	return value
}

// GetEventSendMode returns the event send mode from the annotations
func GetEventSendMode(obj client.Object) string {
	eventSendMode := getAnnotation(obj, operatorconstants.AnnotationMGHEventSendMode)
	if eventSendMode == "" {
		return string(constants.EventSendModeBatch) // default to batch mode
	}
	return eventSendMode
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
	mu.Lock()
	defer mu.Unlock()
	// first check for environment variables containing the 'RELATED_IMAGE_' prefix
	for _, env := range os.Environ() {
		envKeyVal := strings.SplitN(env, "=", 2)
		if strings.HasPrefix(envKeyVal[0], operatorconstants.MGHOperandImagePrefix) {
			key := strings.ToLower(strings.ReplaceAll(envKeyVal[0],
				operatorconstants.MGHOperandImagePrefix, ""))
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

func SetOauthProxyImage(image string) {
	mu.Lock()
	defer mu.Unlock()
	imageOverrides[OauthProxyImageKey] = image
}

// GetImage is used to retrieve image for given component
func GetImage(componentName string) string {
	mu.Lock()
	defer mu.Unlock()
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
	if mgh.Spec.DataLayerSpec.Postgres.StorageSize != "" {
		return mgh.Spec.DataLayerSpec.Postgres.StorageSize
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

// Update imageoverride, ImagePullSecret, StatisticLogInterval, SetTransportConfig, PostgresType
func SetMulticlusterGlobalHubConfig(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub,
	c client.Client, imageClient imagev1client.ImageV1Interface,
) error {
	// set request name to be used in leafhub controller
	SetMGHNamespacedName(types.NamespacedName{
		Namespace: mgh.GetNamespace(), Name: mgh.GetName(),
	})

	// set image overrides
	if err := SetImageOverrides(mgh); err != nil {
		return err
	}

	if imageClient != nil && !reflect.ValueOf(imageClient).IsNil() {
		// set oauth-proxy from imagestream.image.openshift.io
		oauthImageStream, err := imageClient.ImageStreams(operatorconstants.OauthProxyImageStreamNamespace).
			Get(ctx, operatorconstants.OauthProxyImageStreamName, metav1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
			// do not expect error = IsNotFound in OCP environment.
			// But for e2e test, it can be. for this case, just ignore
		} else {
			if oauthImageStream.Spec.Tags != nil {
				tag := oauthImageStream.Spec.Tags[0]
				if tag.From != nil && tag.From.Kind == "DockerImage" && len(tag.From.Name) > 0 {
					SetOauthProxyImage(tag.From.Name)
				}
			}
		}
	}

	// set image pull secret
	SetImagePullSecretName(mgh)

	// set statistic log interval
	if err := SetStatisticLogInterval(mgh); err != nil {
		return err
	}

	if c == nil {
		return nil
	}

	err := SetPostgresType(ctx, c, mgh.GetNamespace())
	if err != nil {
		return err
	}
	return nil
}

func GetMulticlusterGlobalHub(ctx context.Context, c client.Client) (*v1alpha4.MulticlusterGlobalHub, error) {
	mghList := &v1alpha4.MulticlusterGlobalHubList{}
	err := c.List(ctx, mghList)
	if err != nil {
		return nil, err
	}
	if len(mghList.Items) != 1 {
		log.Debugf("mgh may have 1 instance, but got %v", len(mghList.Items))
		return nil, nil
	}
	return &mghList.Items[0], nil
}

func GetMulticlusterGlobalHubAgent(ctx context.Context, c client.Client) (*v1alpha1.MulticlusterGlobalHubAgent, error) {
	mghaList := &v1alpha1.MulticlusterGlobalHubAgentList{}
	err := c.List(ctx, mghaList)
	if err != nil {
		return nil, err
	}
	if len(mghaList.Items) != 1 {
		log.Debugf("mgha may have 1 instance, but got %v", len(mghaList.Items))
		return nil, nil
	}
	return &mghaList.Items[0], nil
}

type OperandConfig struct {
	Replicas        int32
	ImagePullPolicy corev1.PullPolicy
	ImagePullSecret string
	NodeSelector    map[string]string
	Tolerations     []corev1.Toleration
}

func GetOperandConfig(mgh *v1alpha4.MulticlusterGlobalHub) *OperandConfig {
	operandConfig := &OperandConfig{
		Replicas:        1,
		ImagePullPolicy: corev1.PullAlways,
		NodeSelector:    mgh.Spec.NodeSelector,
		Tolerations:     mgh.Spec.Tolerations,
	}
	if mgh.Spec.AvailabilityConfig == v1alpha4.HAHigh {
		operandConfig.Replicas = 2
	}
	if mgh.Spec.ImagePullPolicy != "" {
		operandConfig.ImagePullPolicy = mgh.Spec.ImagePullPolicy
	}
	if mgh.Spec.ImagePullSecret != "" {
		operandConfig.ImagePullSecret = mgh.Spec.ImagePullSecret
	}
	return operandConfig
}
