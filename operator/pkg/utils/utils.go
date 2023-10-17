/*
Copyright 2023

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

package utils

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
)

// Remove is used to remove string from a string array
func Remove(list []string, s string) []string {
	result := []string{}
	for _, v := range list {
		if v != s {
			result = append(result, v)
		}
	}
	return result
}

// Contains is used to check whether a list contains string s
func Contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// GetAnnotation returns the annotation value for a given key, or an empty string if not set
func GetAnnotation(annotations map[string]string, key string) string {
	if annotations == nil {
		return ""
	}
	return annotations[key]
}

func RemoveDuplicates(elements []string) []string {
	// Use map to record duplicates as we find them.
	encountered := map[string]struct{}{}
	result := []string{}

	for _, v := range elements {
		if _, found := encountered[v]; found {
			continue
		}
		encountered[v] = struct{}{}
		result = append(result, v)
	}
	// Return the new slice.
	return result
}

func UpdateObject(ctx context.Context, runtimeClient client.Client, obj client.Object) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return runtimeClient.Update(ctx, obj, &client.UpdateOptions{})
	})
}

// Finds subscription by name. Returns nil if none found.
func GetSubscriptionByName(ctx context.Context, k8sClient client.Client, name string) (
	*subv1alpha1.Subscription, error,
) {
	found := &subv1alpha1.Subscription{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: config.GetDefaultNamespace(),
	}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return found, nil
}

// IsCommunityMode returns true if operator is running in community mode
func IsCommunityMode() bool {
	image := os.Getenv("RELATED_IMAGE_MULTICLUSTER_GLOBAL_HUB_MANAGER")
	if strings.Contains(image, "quay.io/stolostron") {
		// image has quay.io/stolostron treat as community version
		return true
	} else {
		return false
	}
}

func ApplyConfigMap(ctx context.Context, kubeClient kubernetes.Interface, required *corev1.ConfigMap) (bool, error) {
	curAlertConfigMap, err := kubeClient.CoreV1().
		ConfigMaps(required.Namespace).
		Get(ctx,
			required.Name,
			metav1.GetOptions{},
		)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("creating configmap, namespace: %v, name: %v", required.Namespace, required.Name)
			_, err := kubeClient.CoreV1().
				ConfigMaps(required.Namespace).
				Create(ctx,
					required,
					metav1.CreateOptions{},
				)
			if err != nil {
				return false, fmt.Errorf("Failed to create alert configmap, namespace: %v, name: %v, error:%v",
					required.Namespace, required.Name, err)
			}
			return true, err
		}
		return false, nil
	}

	if reflect.DeepEqual(curAlertConfigMap.Data, required.Data) {
		return false, nil
	}

	klog.Infof("Update alert configmap, namespace: %v, name: %v", required.Namespace, required.Name)
	_, err = kubeClient.CoreV1().
		ConfigMaps(required.Namespace).
		Update(ctx,
			required,
			metav1.UpdateOptions{},
		)
	if err != nil {
		return false, fmt.Errorf("Failed to update alert configmap, namespace: %v, name: %v, error:%v",
			required.Namespace, required.Name, err)
	}
	return true, nil
}

func ApplySecret(ctx context.Context, kubeClient kubernetes.Interface, requiredSecret *corev1.Secret) (bool, error) {
	curSecret, err := kubeClient.CoreV1().
		Secrets(requiredSecret.Namespace).
		Get(ctx,
			requiredSecret.Name,
			metav1.GetOptions{},
		)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("creating secret, namespace: %v, name: %v", requiredSecret.Namespace, requiredSecret.Name)
			_, err := kubeClient.CoreV1().
				Secrets(requiredSecret.Namespace).
				Create(ctx,
					requiredSecret,
					metav1.CreateOptions{},
				)
			if err != nil {
				return false, fmt.Errorf("Failed to create secret, namespace: %v, name: %v, error:%v",
					requiredSecret.Namespace, requiredSecret.Name, err)
			}
			return true, err
		}
		return false, nil
	}

	if reflect.DeepEqual(curSecret.Data, requiredSecret.Data) {
		return false, nil
	}

	klog.Infof("Update secret, namespace: %v, name: %v", requiredSecret.Namespace, requiredSecret.Name)
	_, err = kubeClient.CoreV1().
		Secrets(requiredSecret.Namespace).
		Update(ctx,
			requiredSecret,
			metav1.UpdateOptions{},
		)
	if err != nil {
		return false, fmt.Errorf("Failed to update secret, namespace: %v, name: %v, error:%v",
			requiredSecret.Namespace, requiredSecret.Name, err)
	}
	return true, nil
}

// getAlertGPCcount count the groupCount, policyCount, contactCount for the alert
func GetAlertGPCcount(a []byte) (int, int, int, error) {
	var o1 map[string]interface{}
	var groupCount, policyCount, contactCount int
	if len(a) == 0 {
		return groupCount, policyCount, contactCount, nil
	}
	if err := yaml.Unmarshal(a, &o1); err != nil {
		return groupCount, policyCount, contactCount, err
	}
	for k, v := range o1 {
		if !(k == "groups" || k == "policies" || k == "contactPoints") {
			continue
		}
		vArray, _ := v.([]interface{})
		if k == "groups" {
			groupCount = len(vArray)
		}
		if k == "policies" {
			policyCount = len(vArray)
		}
		if k == "contactPoints" {
			contactCount = len(vArray)
		}
	}
	return groupCount, policyCount, contactCount, nil
}

func IsAlertGPCcountEqual(a, b []byte) (bool, error) {
	ag, ap, ac, err := GetAlertGPCcount(a)
	if err != nil {
		return false, err
	}
	bg, bp, bc, err := GetAlertGPCcount(b)
	if err != nil {
		return false, err
	}
	if ag == bg && ap == bp && ac == bc {
		return true, nil
	}
	return false, nil
}

func CopyMap(newMap, originalMap map[string]string) {
	for key, value := range originalMap {
		newMap[key] = value
	}
}

func WaitGlobalHubReady(ctx context.Context,
	client client.Client,
	interval time.Duration) (*globalhubv1alpha4.MulticlusterGlobalHub, error) {
	mghInstance := &globalhubv1alpha4.MulticlusterGlobalHub{}
	klog.Info("Wait MulticlusterGlobalHub ready")

	if err := wait.PollImmediate(interval, 10*time.Minute, func() (bool, error) {
		mghList := &globalhubv1alpha4.MulticlusterGlobalHubList{}
		err := client.List(ctx, mghList)

		if err != nil {
			klog.Error(err, "Failed to list MulticlusterGlobalHub")
			return false, nil
		}
		if len(mghList.Items) != 1 {
			klog.V(2).Info("the count of the mgh instance is not 1 in the cluster.", "count", len(mghList.Items))
			return false, nil
		}
		mghInstance = &mghList.Items[0]
		if !mghList.Items[0].DeletionTimestamp.IsZero() {
			klog.V(2).Info("MulticlusterGlobalHub is deleting")
			return false, nil
		}

		if meta.IsStatusConditionTrue(mghInstance.Status.Conditions, condition.CONDITION_TYPE_GLOBALHUB_READY) {
			klog.V(2).Info("MulticlusterGlobalHub ready condition is not true")
			return true, nil
		}
		return false, nil
	}); err != nil {
		return mghInstance, err
	}
	klog.Info("MulticlusterGlobalHub is ready")
	return mghInstance, nil
}
