/*
Copyright Contributors to the Open Cluster Management project.

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

package protocol

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func resetBYOTransporter() {
	byoTransporter = nil
}

func byoSecret(name, namespace, cert string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"bootstrap_server": []byte("kafka.example:443"),
			"ca.crt":           []byte("ca-" + cert),
			"client.crt":       []byte(cert),
			"client.key":       []byte("key-" + cert),
		},
	}
}

func byoMGH(namespace string) *v1alpha4.MulticlusterGlobalHub {
	return &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mgh",
			Namespace: namespace,
		},
		Spec: v1alpha4.MulticlusterGlobalHubSpec{
			DataLayerSpec: v1alpha4.DataLayerSpec{
				Kafka: v1alpha4.KafkaSpec{
					ConsumerGroupPrefix: "gh-",
				},
			},
		},
	}
}

func TestGetConnCredentialPrefersPerHubSecret(t *testing.T) {
	resetBYOTransporter()
	t.Cleanup(resetBYOTransporter)

	ns := utils.GetDefaultNamespace()
	shared := byoSecret(constants.GHTransportSecretName, ns, "shared-cert")
	hub1 := byoSecret(constants.GHTransportSecretNameForCluster("hub1"), ns, "hub1-cert")
	c := fake.NewClientBuilder().WithScheme(config.GetRuntimeScheme()).
		WithObjects(byoMGH(ns), shared, hub1).Build()

	trans := NewBYOTransporter(context.Background(), types.NamespacedName{
		Name:      constants.GHTransportSecretName,
		Namespace: ns,
	}, c)

	conn, err := trans.GetConnCredential("hub1")
	if err != nil {
		t.Fatalf("GetConnCredential(hub1) error = %v", err)
	}
	if conn.ClientCert == "" {
		t.Fatal("expected per-hub client cert")
	}
	if !strings.Contains(conn.ConsumerGroupID, "hub1") {
		t.Fatalf("ConsumerGroupID = %q, want hub1", conn.ConsumerGroupID)
	}

	sharedConn, err := trans.GetConnCredential("")
	if err != nil {
		t.Fatalf("GetConnCredential() error = %v", err)
	}
	if sharedConn.ClientCert == conn.ClientCert {
		t.Fatal("manager shared secret must not reuse the per-hub client cert")
	}
}

func TestGetConnCredentialFallsBackToSharedSecret(t *testing.T) {
	resetBYOTransporter()
	t.Cleanup(resetBYOTransporter)

	ns := utils.GetDefaultNamespace()
	shared := byoSecret(constants.GHTransportSecretName, ns, "shared-cert")
	c := fake.NewClientBuilder().WithScheme(config.GetRuntimeScheme()).
		WithObjects(byoMGH(ns), shared).Build()

	trans := NewBYOTransporter(context.Background(), types.NamespacedName{
		Name:      constants.GHTransportSecretName,
		Namespace: ns,
	}, c)

	conn, err := trans.GetConnCredential("hub2")
	if err != nil {
		t.Fatalf("GetConnCredential(hub2) fallback error = %v", err)
	}
	if conn.BootstrapServer != "kafka.example:443" {
		t.Fatalf("BootstrapServer = %q", conn.BootstrapServer)
	}
}

func TestEnsureUserRejectsIdenticalPerHubCerts(t *testing.T) {
	resetBYOTransporter()
	t.Cleanup(resetBYOTransporter)

	ns := utils.GetDefaultNamespace()
	same := "duplicate-cert"
	hub1 := byoSecret(constants.GHTransportSecretNameForCluster("hub1"), ns, same)
	hub2 := byoSecret(constants.GHTransportSecretNameForCluster("hub2"), ns, same)
	c := fake.NewClientBuilder().WithScheme(config.GetRuntimeScheme()).
		WithObjects(byoMGH(ns), hub1, hub2).Build()

	trans := NewBYOTransporter(context.Background(), types.NamespacedName{
		Name:      constants.GHTransportSecretName,
		Namespace: ns,
	}, c)

	_, err := trans.EnsureUser("hub1")
	if err == nil {
		t.Fatal("EnsureUser(hub1) expected identical-cert error")
	}
	if !strings.Contains(err.Error(), "identical") {
		t.Fatalf("EnsureUser() error = %v, want identical cert message", err)
	}
}

func TestEnsureUserAllowsDistinctPerHubCerts(t *testing.T) {
	resetBYOTransporter()
	t.Cleanup(resetBYOTransporter)

	ns := utils.GetDefaultNamespace()
	hub1 := byoSecret(constants.GHTransportSecretNameForCluster("hub1"), ns, "hub1-cert")
	hub2 := byoSecret(constants.GHTransportSecretNameForCluster("hub2"), ns, "hub2-cert")
	c := fake.NewClientBuilder().WithScheme(config.GetRuntimeScheme()).
		WithObjects(byoMGH(ns), hub1, hub2).Build()

	trans := NewBYOTransporter(context.Background(), types.NamespacedName{
		Name:      constants.GHTransportSecretName,
		Namespace: ns,
	}, c)

	user, err := trans.EnsureUser("hub1")
	if err != nil {
		t.Fatalf("EnsureUser(hub1) error = %v", err)
	}
	if user != "hub1-kafka-user" {
		t.Fatalf("EnsureUser() = %q, want hub1-kafka-user", user)
	}
}

func TestEnsureUserSucceedsWithoutSecret(t *testing.T) {
	resetBYOTransporter()
	t.Cleanup(resetBYOTransporter)

	ns := utils.GetDefaultNamespace()
	c := fake.NewClientBuilder().WithScheme(config.GetRuntimeScheme()).
		WithObjects(byoMGH(ns)).Build()

	trans := NewBYOTransporter(context.Background(), types.NamespacedName{
		Name:      constants.GHTransportSecretName,
		Namespace: ns,
	}, c)

	user, err := trans.EnsureUser("hub1")
	if err != nil {
		t.Fatalf("EnsureUser(hub1) without secret error = %v", err)
	}
	if user != "hub1-kafka-user" {
		t.Fatalf("EnsureUser() = %q, want hub1-kafka-user", user)
	}
}

func TestGetConnCredentialMissingSecret(t *testing.T) {
	resetBYOTransporter()
	t.Cleanup(resetBYOTransporter)

	ns := utils.GetDefaultNamespace()
	c := fake.NewClientBuilder().WithScheme(config.GetRuntimeScheme()).
		WithObjects(byoMGH(ns)).Build()

	trans := NewBYOTransporter(context.Background(), types.NamespacedName{
		Name:      constants.GHTransportSecretName,
		Namespace: ns,
	}, c)

	_, err := trans.GetConnCredential("hub1")
	if err == nil {
		t.Fatal("GetConnCredential(hub1) expected missing secret error")
	}
}
