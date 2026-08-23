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

package protocol

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

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

func newBYOTransporter(t *testing.T, objects ...client.Object) *BYOTransporter {
	t.Helper()
	ns := utils.GetDefaultNamespace()
	c := fake.NewClientBuilder().WithScheme(config.GetRuntimeScheme()).
		WithObjects(objects...).Build()
	return NewBYOTransporter(context.Background(), types.NamespacedName{
		Name:      constants.GHTransportSecretName,
		Namespace: ns,
	}, c)
}

type getErrorClient struct {
	client.Client
	err error
}

func (c getErrorClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object,
	opts ...client.GetOption,
) error {
	if c.err != nil {
		return c.err
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func TestNewBYOTransporterReturnsIsolatedInstance(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	c := fake.NewClientBuilder().WithScheme(config.GetRuntimeScheme()).
		WithObjects(byoMGH(ns)).Build()

	first := NewBYOTransporter(context.Background(), types.NamespacedName{
		Name:      constants.GHTransportSecretName,
		Namespace: ns,
	}, c)
	second := NewBYOTransporter(context.Background(), types.NamespacedName{
		Name:      "other-transport",
		Namespace: "other-ns",
	}, c)
	if first == second {
		t.Fatal("NewBYOTransporter must return a distinct instance per call")
	}
	if first.namespace == second.namespace || first.name == second.name {
		t.Fatal("NewBYOTransporter must not rewrite fields on a shared instance")
	}
}

func TestGetConnCredentialPrefersPerHubSecret(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	shared := byoSecret(constants.GHTransportSecretName, ns, "shared-cert")
	hub1 := byoSecret(constants.GHTransportSecretNameForCluster("hub1"), ns, "hub1-cert")
	trans := newBYOTransporter(t, byoMGH(ns), shared, hub1)

	conn, err := trans.GetConnCredential("hub1")
	if err != nil {
		t.Fatalf("GetConnCredential(hub1) error = %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(conn.ClientCert)
	if err != nil {
		t.Fatalf("decode client cert: %v", err)
	}
	if string(decoded) != "hub1-cert" {
		t.Fatalf("client cert = %q, want hub1-cert", decoded)
	}
	if !strings.Contains(conn.ConsumerGroupID, "hub1") {
		t.Fatalf("ConsumerGroupID = %q, want hub1", conn.ConsumerGroupID)
	}
}

func TestGetConnCredentialManagerUsesSharedSecret(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	shared := byoSecret(constants.GHTransportSecretName, ns, "shared-cert")
	hub1 := byoSecret(constants.GHTransportSecretNameForCluster("hub1"), ns, "hub1-cert")
	trans := newBYOTransporter(t, byoMGH(ns), shared, hub1)

	conn, err := trans.GetConnCredential(constants.CloudEventGlobalHubClusterName)
	if err != nil {
		t.Fatalf("GetConnCredential(global-hub) error = %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(conn.ClientCert)
	if err != nil {
		t.Fatalf("decode client cert: %v", err)
	}
	if string(decoded) != "shared-cert" {
		t.Fatalf("manager client cert = %q, want shared-cert", decoded)
	}
}

func TestGetConnCredentialManagerIgnoresGlobalHubNamedSecret(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	shared := byoSecret(constants.GHTransportSecretName, ns, "manager-cert")
	spoof := byoSecret(constants.GHTransportSecretNameForCluster(
		constants.CloudEventGlobalHubClusterName), ns, "spoof-cert")
	trans := newBYOTransporter(t, byoMGH(ns), shared, spoof)

	conn, err := trans.GetConnCredential(constants.CloudEventGlobalHubClusterName)
	if err != nil {
		t.Fatalf("GetConnCredential(global-hub) error = %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(conn.ClientCert)
	if err != nil {
		t.Fatalf("decode client cert: %v", err)
	}
	if string(decoded) != "manager-cert" {
		t.Fatalf("manager must use the shared secret, got cert %q", decoded)
	}
}

func TestGetConnCredentialFallsBackToSharedSecret(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	shared := byoSecret(constants.GHTransportSecretName, ns, "shared-cert")
	trans := newBYOTransporter(t, byoMGH(ns), shared)

	conn, err := trans.GetConnCredential("hub2")
	if err != nil {
		t.Fatalf("GetConnCredential(hub2) fallback error = %v", err)
	}
	if conn.BootstrapServer != "kafka.example:443" {
		t.Fatalf("BootstrapServer = %q", conn.BootstrapServer)
	}
}

func TestEnsureUserRejectsIdenticalPerHubCerts(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	same := "duplicate-cert"
	hub1 := byoSecret(constants.GHTransportSecretNameForCluster("hub1"), ns, same)
	hub2 := byoSecret(constants.GHTransportSecretNameForCluster("hub2"), ns, same)
	trans := newBYOTransporter(t, byoMGH(ns), hub1, hub2)

	_, err := trans.EnsureUser("hub1")
	if err == nil {
		t.Fatal("EnsureUser(hub1) expected identical-cert error")
	}
	if !strings.Contains(err.Error(), "identical") {
		t.Fatalf("EnsureUser() error = %v, want identical cert message", err)
	}
}

func TestEnsureUserAllowsDistinctPerHubCerts(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	hub1 := byoSecret(constants.GHTransportSecretNameForCluster("hub1"), ns, "hub1-cert")
	hub2 := byoSecret(constants.GHTransportSecretNameForCluster("hub2"), ns, "hub2-cert")
	trans := newBYOTransporter(t, byoMGH(ns), hub1, hub2)

	user, err := trans.EnsureUser("hub1")
	if err != nil {
		t.Fatalf("EnsureUser(hub1) error = %v", err)
	}
	if user != "hub1-kafka-user" {
		t.Fatalf("EnsureUser() = %q, want hub1-kafka-user", user)
	}
}

func TestEnsureUserSucceedsWithoutSecret(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	trans := newBYOTransporter(t, byoMGH(ns))

	user, err := trans.EnsureUser("hub1")
	if err != nil {
		t.Fatalf("EnsureUser(hub1) without secret error = %v", err)
	}
	if user != "hub1-kafka-user" {
		t.Fatalf("EnsureUser() = %q, want hub1-kafka-user", user)
	}
}

func TestEnsureUserReturnsAPIError(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	base := fake.NewClientBuilder().WithScheme(config.GetRuntimeScheme()).
		WithObjects(byoMGH(ns)).Build()
	forbidden := apierrors.NewForbidden(
		schema.GroupResource{Group: "", Resource: "secrets"},
		constants.GHTransportSecretNameForCluster("hub1"),
		nil,
	)
	trans := NewBYOTransporter(context.Background(), types.NamespacedName{
		Name:      constants.GHTransportSecretName,
		Namespace: ns,
	}, getErrorClient{Client: base, err: forbidden})

	_, err := trans.EnsureUser("hub1")
	if err == nil {
		t.Fatal("EnsureUser(hub1) expected API error")
	}
	if !apierrors.IsForbidden(err) {
		t.Fatalf("EnsureUser() error = %v, want forbidden", err)
	}
}

func TestGetConnCredentialMissingSecret(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	trans := newBYOTransporter(t, byoMGH(ns))

	_, err := trans.GetConnCredential("hub1")
	if err == nil {
		t.Fatal("GetConnCredential(hub1) expected missing secret error")
	}
}
