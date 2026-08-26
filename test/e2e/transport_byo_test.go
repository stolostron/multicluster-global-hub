// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tests

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	transportconfig "github.com/stolostron/multicluster-global-hub/pkg/transport/config"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/identity"
)

const (
	byoPerHubBootstrapMarker = "e2e-byo-per-hub-marker:443"
	byoIdenticalCertLog      = "must not be identical"
	byoAPITimeout            = 30 * time.Second
)

var _ = Describe("Transport BYO Kafka E2E", Serial, Label("e2e-test-transport-byo"), Ordered, func() {
	var (
		sourceHubName   string
		targetHubName   string
		sourceHubClient client.Client
		sharedBootstrap string
	)

	BeforeAll(func() {
		if isBYO != "true" {
			Skip("BYO Kafka e2e requires ISBYO=true")
		}
		Expect(len(managedHubNames)).To(BeNumerically(">=", 2),
			"BYO Kafka e2e requires two regional hubs")
		sourceHubName = managedHubNames[0]
		targetHubName = managedHubNames[1]

		var err error
		sourceHubClient, err = testClients.RuntimeClient(sourceHubName, agentScheme)
		Expect(err).NotTo(HaveOccurred(), "expected a kube client for the source managed hub")

		shared, err := byoSharedTransportSecret()
		Expect(err).NotTo(HaveOccurred(), "expected the shared BYO transport secret")
		sharedBootstrap = string(shared.Data["bootstrap_server"])
		Expect(sharedBootstrap).NotTo(BeEmpty(), "shared BYO transport secret must set bootstrap_server")
	})

	AfterAll(func() {
		if isBYO != "true" {
			return
		}
		deleteBYOPerHubSecret(sourceHubName)
		deleteBYOPerHubSecret(targetHubName)
	})

	It("allows a custom client certificate CN on the shared BYO secret", func() {
		shared, err := byoSharedTransportSecret()
		Expect(err).NotTo(HaveOccurred(), "expected the shared BYO transport secret")
		Expect(shared.Data["client.crt"]).NotTo(BeEmpty(), "shared BYO transport secret must set client.crt")

		cn := pemCertificateCommonName(shared.Data["client.crt"])
		Expect(identity.HubFromClientCertCN(cn)).To(BeEmpty(),
			"shared BYO client cert CN must be a custom name, not {hub}-kafka-user")
	})

	It("starts the managed hub agent with shared-secret fallback", func() {
		Eventually(func() error {
			return managedHubAgentKafkaReady(sourceHubClient, sourceHubName)
		}, 2*time.Minute, 5*time.Second).Should(Succeed(),
			"managed hub agent must start with the shared BYO transport secret")
	})

	It("selects a per-hub transport secret over the shared secret", func() {
		createBYOPerHubSecret(sourceHubName, func(secret *corev1.Secret) {
			secret.Data["bootstrap_server"] = []byte(byoPerHubBootstrapMarker)
		})
		DeferCleanup(func() {
			deleteBYOPerHubSecret(sourceHubName)
			Eventually(func() error {
				cfg, err := managedHubAgentKafkaConfig(sourceHubClient)
				if err != nil {
					return err
				}
				if cfg.BootstrapServer != sharedBootstrap {
					return fmt.Errorf("waiting for shared-secret bootstrap to be restored")
				}
				return nil
			}, 2*time.Minute, 5*time.Second).Should(Succeed(),
				"agent transport must fall back to the shared BYO secret after per-hub cleanup")
		})

		Eventually(func() error {
			cfg, err := managedHubAgentKafkaConfig(sourceHubClient)
			if err != nil {
				return err
			}
			if cfg.BootstrapServer != byoPerHubBootstrapMarker {
				return fmt.Errorf("agent transport still uses shared bootstrap")
			}
			return nil
		}, 2*time.Minute, 5*time.Second).Should(Succeed(),
			"agent transport credentials must come from the per-hub BYO secret")
	})

	It("rejects identical client certificates on two per-hub secrets", func() {
		since := metav1.Now()
		createBYOPerHubSecret(sourceHubName, nil)
		createBYOPerHubSecret(targetHubName, nil)
		DeferCleanup(func() {
			deleteBYOPerHubSecret(sourceHubName)
			deleteBYOPerHubSecret(targetHubName)
		})

		Eventually(func() error {
			return operatorLogsContain(byoIdenticalCertLog, &since)
		}, 2*time.Minute, 5*time.Second).Should(Succeed(),
			"operator must reject identical client certificates on per-hub BYO secrets")
	})
})

// byoAPIContext returns a short timeout for BYO secret and log API calls.
func byoAPIContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, byoAPITimeout)
}

// byoSharedTransportSecret loads the shared BYO transport secret used as a template.
func byoSharedTransportSecret() (*corev1.Secret, error) {
	apiCtx, cancel := byoAPIContext()
	defer cancel()
	return testClients.KubeClient().CoreV1().Secrets(testOptions.GlobalHub.Namespace).Get(
		apiCtx, constants.GHTransportSecretName, metav1.GetOptions{},
	)
}

// createBYOPerHubSecret creates or updates a per-hub BYO transport secret.
func createBYOPerHubSecret(clusterName string, mutate func(*corev1.Secret)) {
	shared, err := byoSharedTransportSecret()
	Expect(err).NotTo(HaveOccurred(), "expected the shared BYO transport secret as a template")

	data := make(map[string][]byte, len(shared.Data))
	for key, value := range shared.Data {
		copied := make([]byte, len(value))
		copy(copied, value)
		data[key] = copied
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.GHTransportSecretNameForCluster(clusterName),
			Namespace: testOptions.GlobalHub.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
	if mutate != nil {
		mutate(secret)
	}

	kube := testClients.KubeClient().CoreV1().Secrets(testOptions.GlobalHub.Namespace)
	apiCtx, cancel := byoAPIContext()
	defer cancel()
	_, err = kube.Create(apiCtx, secret, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := kube.Get(apiCtx, secret.Name, metav1.GetOptions{})
		Expect(getErr).NotTo(HaveOccurred(), "expected existing per-hub BYO secret")
		existing.Data = secret.Data
		_, err = kube.Update(apiCtx, existing, metav1.UpdateOptions{})
	}
	Expect(err).NotTo(HaveOccurred(), "expected to create or update the per-hub BYO secret")
}

// deleteBYOPerHubSecret deletes a leftover per-hub BYO transport secret.
func deleteBYOPerHubSecret(clusterName string) {
	apiCtx, cancel := byoAPIContext()
	defer cancel()
	err := testClients.KubeClient().CoreV1().Secrets(testOptions.GlobalHub.Namespace).Delete(
		apiCtx, constants.GHTransportSecretNameForCluster(clusterName), metav1.DeleteOptions{},
	)
	if err != nil && !apierrors.IsNotFound(err) {
		Expect(err).NotTo(HaveOccurred(), "expected to delete leftover per-hub BYO secret")
	}
}

// managedHubAgentKafkaConfig reads Kafka settings from the agent transport-config secret.
func managedHubAgentKafkaConfig(hubClient client.Client) (*transport.KafkaConfig, error) {
	apiCtx, cancel := byoAPIContext()
	defer cancel()
	secret := &corev1.Secret{}
	if err := hubClient.Get(apiCtx, types.NamespacedName{
		Name:      constants.GHTransportConfigSecret,
		Namespace: constants.GHAgentNamespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("get agent transport-config secret: %w", err)
	}
	cfg, err := transportconfig.GetKafkaCredentialBySecret(secret, hubClient)
	if err != nil {
		return nil, fmt.Errorf("parse agent transport-config: %w", err)
	}
	return cfg, nil
}

// managedHubAgentKafkaReady checks agent Kafka config and a ready agent Deployment.
func managedHubAgentKafkaReady(hubClient client.Client, hubName string) error {
	cfg, err := managedHubAgentKafkaConfig(hubClient)
	if err != nil {
		return err
	}
	if cfg.ClientCert == "" {
		return fmt.Errorf("agent transport-config is missing a client certificate")
	}
	apiCtx, cancel := byoAPIContext()
	defer cancel()
	deployment := &appsv1.Deployment{}
	if err := hubClient.Get(apiCtx, types.NamespacedName{
		Namespace: constants.GHAgentNamespace,
		Name:      "multicluster-global-hub-agent",
	}, deployment); err != nil {
		return fmt.Errorf("agent on hub %s: %w", hubName, err)
	}
	if deployment.Status.AvailableReplicas == 0 {
		return fmt.Errorf("deployment: %s is not ready", deployment.Name)
	}
	return nil
}

// pemCertificateCommonName returns the CommonName from a PEM client certificate.
func pemCertificateCommonName(certPEM []byte) string {
	block, _ := pem.Decode(certPEM)
	Expect(block).NotTo(BeNil(), "shared BYO client.crt must be PEM")
	cert, err := x509.ParseCertificate(block.Bytes)
	Expect(err).NotTo(HaveOccurred(), "shared BYO client.crt must parse as x509")
	return cert.Subject.CommonName
}

// operatorLogsContain reports whether operator logs since the given time contain substr.
func operatorLogsContain(substr string, since *metav1.Time) error {
	apiCtx, cancel := byoAPIContext()
	defer cancel()
	pods, err := testClients.KubeClient().CoreV1().Pods(testOptions.GlobalHub.Namespace).List(apiCtx, metav1.ListOptions{
		LabelSelector: "name=multicluster-global-hub-operator",
	})
	if err != nil {
		return err
	}
	if len(pods.Items) == 0 {
		return fmt.Errorf("operator pod not found")
	}

	var combined strings.Builder
	for i := range pods.Items {
		logs, logErr := testClients.KubeClient().CoreV1().Pods(testOptions.GlobalHub.Namespace).
			GetLogs(pods.Items[i].Name, &corev1.PodLogOptions{
				Container: "multicluster-global-hub-operator",
				SinceTime: since,
			}).DoRaw(apiCtx)
		if logErr != nil {
			return logErr
		}
		if _, writeErr := combined.Write(logs); writeErr != nil {
			return fmt.Errorf("append operator logs: %w", writeErr)
		}
	}
	if !strings.Contains(combined.String(), substr) {
		return fmt.Errorf("operator logs do not yet report identical per-hub client certificates")
	}
	return nil
}
