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

	"github.com/go-logr/logr"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
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

// GetKafkaConfig retrieves kafka server and CA from kafka secret
func GetKafkaConfig(ctx context.Context, kubeClient kubernetes.Interface, log logr.Logger,
	mgh *operatorv1alpha2.MulticlusterGlobalHub,
) (string, string, error) {
	// for local dev/test
	kafkaBootstrapServer, ok := mgh.GetAnnotations()[constants.AnnotationKafkaBootstrapServer]
	if ok && kafkaBootstrapServer != "" {
		log.Info("Kafka bootstrap server from annotation", "server", kafkaBootstrapServer, "certificate", "")
		return kafkaBootstrapServer, "", nil
	}

	kafkaSecret, err := kubeClient.CoreV1().Secrets(config.GetDefaultNamespace()).Get(ctx,
		mgh.Spec.DataLayer.LargeScale.Kafka.Name, metav1.GetOptions{})
	if err != nil {
		log.Error(err, "failed to get transport secret",
			"namespace", config.GetDefaultNamespace(),
			"name", mgh.Spec.DataLayer.LargeScale.Kafka.Name)
		return "", "", err
	}

	return string(kafkaSecret.Data["bootstrap_server"]),
		base64.RawStdEncoding.EncodeToString(kafkaSecret.Data["CA"]), nil
}
