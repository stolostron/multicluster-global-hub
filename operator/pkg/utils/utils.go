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

package utils

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"reflect"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// GeneratePassword returns a base64 encoded securely random bytes.
func GeneratePassword(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(b), err
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

// GetKafkaConfig retrieves kafka server, caCert, clientCert, clientKey from the secret
func GetKafkaConfig(ctx context.Context, kubeClient kubernetes.Interface,
	namespace string, name string,
) (string, string, string, string, error) {
	kafkaSecret, err := kubeClient.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", "", "", "", err
	}
	return string(kafkaSecret.Data[filepath.Join("bootstrap_server")]),
		base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("ca.crt")]),
		base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("client.crt")]),
		base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("client.key")]),
		nil
}

func UpdateObject(ctx context.Context, runtimeClient client.Client, obj client.Object) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return runtimeClient.Update(ctx, obj, &client.UpdateOptions{})
	})
}

func ApplyConfigMap(ctx context.Context, kubeClient kubernetes.Interface, requiredConfigMap *corev1.ConfigMap) (bool, error) {
	curAlertConfigMap, err := kubeClient.CoreV1().ConfigMaps(requiredConfigMap.Namespace).Get(ctx, requiredConfigMap.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("creating configmap, namespace: %v, name: %v", requiredConfigMap.Namespace, requiredConfigMap.Name)
			_, err := kubeClient.CoreV1().ConfigMaps(requiredConfigMap.Namespace).Create(ctx, requiredConfigMap, metav1.CreateOptions{})
			if err != nil {
				return false, fmt.Errorf("Failed to create alert configmap, namespace: %v, name: %v, error:%v", requiredConfigMap.Namespace, requiredConfigMap.Name, err)
			}
			return true, err
		}
		return false, nil
	}

	if reflect.DeepEqual(curAlertConfigMap.Data, requiredConfigMap.Data) {
		return false, nil
	}

	klog.Infof("Update alert configmap, namespace: %v, name: %v", requiredConfigMap.Namespace, requiredConfigMap.Name)
	_, err = kubeClient.CoreV1().ConfigMaps(requiredConfigMap.Namespace).Update(ctx, requiredConfigMap, metav1.UpdateOptions{})
	if err != nil {
		return false, fmt.Errorf("Failed to update alert configmap, namespace: %v, name: %v, error:%v", requiredConfigMap.Namespace, requiredConfigMap.Name, err)
	}
	return true, nil
}

func ApplySecret(ctx context.Context, kubeClient kubernetes.Interface, requiredSecret *corev1.Secret) (bool, error) {
	curSecret, err := kubeClient.CoreV1().Secrets(requiredSecret.Namespace).Get(ctx, requiredSecret.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("creating secret, namespace: %v, name: %v", requiredSecret.Namespace, requiredSecret.Name)
			_, err := kubeClient.CoreV1().Secrets(requiredSecret.Namespace).Create(ctx, requiredSecret, metav1.CreateOptions{})
			if err != nil {
				return false, fmt.Errorf("Failed to create secret, namespace: %v, name: %v, error:%v", requiredSecret.Namespace, requiredSecret.Name, err)
			}
			return true, err
		}
		return false, nil
	}

	if reflect.DeepEqual(curSecret.Data, requiredSecret.Data) {
		return false, nil
	}

	klog.Infof("Update secret, namespace: %v, name: %v", requiredSecret.Namespace, requiredSecret.Name)
	_, err = kubeClient.CoreV1().Secrets(requiredSecret.Namespace).Update(ctx, requiredSecret, metav1.UpdateOptions{})
	if err != nil {
		return false, fmt.Errorf("Failed to update secret, namespace: %v, name: %v, error:%v", requiredSecret.Namespace, requiredSecret.Name, err)
	}
	return true, nil
}
