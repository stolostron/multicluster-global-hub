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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// byoSecret builds a BYO Kafka transport secret for unit tests.
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

// byoMGH returns a MulticlusterGlobalHub with a Kafka consumer-group prefix.
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

// byoManagedCluster builds a ManagedCluster used to identify per-hub BYO secrets.
func byoManagedCluster(name string) *clusterv1.ManagedCluster {
	return &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
}

// newBYOTransporter builds a BYOTransporter backed by a fake client.
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

// Get returns the injected error so EnsureUser can surface API failures.
func (c getErrorClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object,
	opts ...client.GetOption,
) error {
	if c.err != nil {
		return c.err
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

// TestNewBYOTransporterReturnsIsolatedInstance checks each call gets its own transporter.
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

// TestGetConnCredentialPrefersPerHubSecret checks per-hub secrets win over the shared secret.
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

// TestGetConnCredentialManagerUsesSharedSecret checks the manager always uses the shared secret.
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

// TestGetConnCredentialManagerIgnoresGlobalHubNamedSecret ignores a secret named for the manager hub.
func TestGetConnCredentialManagerIgnoresGlobalHubNamedSecret(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	shared := byoSecret(constants.GHTransportSecretName, ns, "manager-cert")
	spoof := byoSecret(constants.GHTransportSecretNameForCluster(
		constants.CloudEventGlobalHubClusterName,
	), ns, "spoof-cert")
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

// TestGetConnCredentialFallsBackToSharedSecret uses the shared secret when no per-hub secret exists.
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

// TestGetConnCredentialPerHubLeavesOtherHubsOnSharedSecret checks that a per-hub
// BYO secret for one hub does not change credentials for other hubs or the manager.
func TestGetConnCredentialPerHubLeavesOtherHubsOnSharedSecret(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	shared := byoSecret(constants.GHTransportSecretName, ns, "shared-cert")
	hub1 := byoSecret(constants.GHTransportSecretNameForCluster("hub1"), ns, "hub1-cert")
	trans := newBYOTransporter(t, byoMGH(ns), shared, hub1)

	hub1Conn, err := trans.GetConnCredential("hub1")
	if err != nil {
		t.Fatalf("GetConnCredential(hub1) error = %v", err)
	}
	hub1Cert, err := base64.StdEncoding.DecodeString(hub1Conn.ClientCert)
	if err != nil {
		t.Fatalf("decode hub1 client cert: %v", err)
	}
	if string(hub1Cert) != "hub1-cert" {
		t.Fatalf("hub1 client cert = %q, want hub1-cert", hub1Cert)
	}

	hub2Conn, err := trans.GetConnCredential("hub2")
	if err != nil {
		t.Fatalf("GetConnCredential(hub2) error = %v", err)
	}
	hub2Cert, err := base64.StdEncoding.DecodeString(hub2Conn.ClientCert)
	if err != nil {
		t.Fatalf("decode hub2 client cert: %v", err)
	}
	if string(hub2Cert) != "shared-cert" {
		t.Fatalf("hub2 client cert = %q, want shared-cert", hub2Cert)
	}

	mgrConn, err := trans.GetConnCredential(constants.CloudEventGlobalHubClusterName)
	if err != nil {
		t.Fatalf("GetConnCredential(global-hub) error = %v", err)
	}
	mgrCert, err := base64.StdEncoding.DecodeString(mgrConn.ClientCert)
	if err != nil {
		t.Fatalf("decode manager client cert: %v", err)
	}
	if string(mgrCert) != "shared-cert" {
		t.Fatalf("manager client cert = %q, want shared-cert", mgrCert)
	}
}

// TestEnsureUserRejectsIdenticalPerHubCerts rejects two hubs sharing the same client certificate.
func TestEnsureUserRejectsIdenticalPerHubCerts(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	same := "duplicate-cert"
	hub1 := byoSecret(constants.GHTransportSecretNameForCluster("hub1"), ns, same)
	hub2 := byoSecret(constants.GHTransportSecretNameForCluster("hub2"), ns, same)
	trans := newBYOTransporter(t, byoMGH(ns), hub1, hub2, byoManagedCluster("hub1"), byoManagedCluster("hub2"))

	_, err := trans.EnsureUser("hub1")
	if err == nil {
		t.Fatal("EnsureUser(hub1) expected identical-cert error")
	}
	if !strings.Contains(err.Error(), "identical") {
		t.Fatalf("EnsureUser() error = %v, want identical cert message", err)
	}
}

// TestEnsureUserRejectsEquivalentLeafCerts rejects the same leaf certificate
// encoded with extra PEM wrapping or chain blocks.
func TestEnsureUserRejectsEquivalentLeafCerts(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	leaf := mustBYOCertPEM(t, "shared-leaf")
	chain := append(append([]byte("\n"), leaf...), mustBYOCertPEM(t, "intermediate")...)
	hub1 := byoSecret(constants.GHTransportSecretNameForCluster("hub1"), ns, string(leaf))
	hub2 := byoSecret(constants.GHTransportSecretNameForCluster("hub2"), ns, string(chain))
	trans := newBYOTransporter(t, byoMGH(ns), hub1, hub2, byoManagedCluster("hub1"), byoManagedCluster("hub2"))

	_, err := trans.EnsureUser("hub1")
	if err == nil {
		t.Fatal("EnsureUser(hub1) expected identical leaf certificate error")
	}
	if !strings.Contains(err.Error(), "identical") {
		t.Fatalf("EnsureUser() error = %v, want identical cert message", err)
	}
}

// TestEnsureUserAllowsDistinctPerHubCerts allows hubs with different client certificates.
func TestEnsureUserAllowsDistinctPerHubCerts(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	hub1 := byoSecret(constants.GHTransportSecretNameForCluster("hub1"), ns, "hub1-cert")
	hub2 := byoSecret(constants.GHTransportSecretNameForCluster("hub2"), ns, "hub2-cert")
	trans := newBYOTransporter(t, byoMGH(ns), hub1, hub2, byoManagedCluster("hub1"), byoManagedCluster("hub2"))

	user, err := trans.EnsureUser("hub1")
	if err != nil {
		t.Fatalf("EnsureUser(hub1) error = %v", err)
	}
	if user != "hub1-kafka-user" {
		t.Fatalf("EnsureUser() = %q, want hub1-kafka-user", user)
	}
}

// TestEnsureUserReportsMissingClientCert rejects a per-hub secret that omits client.crt.
func TestEnsureUserReportsMissingClientCert(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	hub1 := byoSecret(constants.GHTransportSecretNameForCluster("hub1"), ns, "")
	trans := newBYOTransporter(t, byoMGH(ns), hub1)

	_, err := trans.EnsureUser("hub1")
	if err == nil {
		t.Fatal("EnsureUser(hub1) expected missing client.crt error")
	}
	if !strings.Contains(err.Error(), "missing client.crt") {
		t.Fatalf("EnsureUser() error = %v, want missing client.crt", err)
	}
}

// TestEnsureUserRejectsIdenticalUnrelatedPrefixedSecret rejects identical certs on any per-hub secret.
func TestEnsureUserRejectsIdenticalUnrelatedPrefixedSecret(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	same := "duplicate-cert"
	hub1 := byoSecret(constants.GHTransportSecretNameForCluster("hub1"), ns, same)
	unrelated := byoSecret(constants.GHTransportSecretName+"-not-a-managed-hub", ns, same)
	trans := newBYOTransporter(t, byoMGH(ns), hub1, unrelated)

	_, err := trans.EnsureUser("hub1")
	if err == nil {
		t.Fatal("EnsureUser(hub1) expected identical-cert error")
	}
	if !strings.Contains(err.Error(), "identical") {
		t.Fatalf("EnsureUser() error = %v, want identical cert message", err)
	}
}

// TestEnsureUserSucceedsWithoutSecret allows addon install before the BYO secret exists.
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

// TestEnsureUserReturnsAPIError surfaces non-NotFound secret lookup failures.
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

// TestGetConnCredentialMissingSecret errors when neither per-hub nor shared secret exists.
func TestGetConnCredentialMissingSecret(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	trans := newBYOTransporter(t, byoMGH(ns))

	_, err := trans.GetConnCredential("hub1")
	if err == nil {
		t.Fatal("GetConnCredential(hub1) expected missing secret error")
	}
	if !strings.Contains(err.Error(), "failed to get BYO Kafka transport secret") {
		t.Fatalf("GetConnCredential() error = %v, want wrapped lookup error", err)
	}
}

// TestGetConnCredentialRejectsIdenticalPerHubCerts rejects duplicate per-hub client certificates.
func TestGetConnCredentialRejectsIdenticalPerHubCerts(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	same := "duplicate-cert"
	hub1 := byoSecret(constants.GHTransportSecretNameForCluster("hub1"), ns, same)
	hub2 := byoSecret(constants.GHTransportSecretNameForCluster("hub2"), ns, same)
	trans := newBYOTransporter(t, byoMGH(ns), hub1, hub2, byoManagedCluster("hub1"), byoManagedCluster("hub2"))

	_, err := trans.GetConnCredential("hub1")
	if err == nil {
		t.Fatal("GetConnCredential(hub1) expected identical-cert error")
	}
	if !strings.Contains(err.Error(), "identical") {
		t.Fatalf("GetConnCredential() error = %v, want identical cert message", err)
	}
}

// TestEnsureUserAllowsSharedFallbackMatchingPerHubCert allows shared-secret fallback with a matching cert.
func TestEnsureUserAllowsSharedFallbackMatchingPerHubCert(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	same := "shared-cert"
	shared := byoSecret(constants.GHTransportSecretName, ns, same)
	hub1 := byoSecret(constants.GHTransportSecretNameForCluster("hub1"), ns, same)
	trans := newBYOTransporter(t, byoMGH(ns), shared, hub1)

	if _, err := trans.EnsureUser("hub2"); err != nil {
		t.Fatalf("EnsureUser(hub2) shared fallback error = %v", err)
	}
	if _, err := trans.GetConnCredential("hub2"); err != nil {
		t.Fatalf("GetConnCredential(hub2) shared fallback error = %v", err)
	}
}

// TestEnsureUserIgnoresManagerNamedSecretDuplicates ignores manager-named secrets in uniqueness checks.
func TestEnsureUserIgnoresManagerNamedSecretDuplicates(t *testing.T) {
	ns := utils.GetDefaultNamespace()
	hub1 := byoSecret(constants.GHTransportSecretNameForCluster("hub1"), ns, "hub1-cert")
	managerNamed := byoSecret(constants.GHTransportSecretNameForCluster(
		constants.CloudEventGlobalHubClusterName,
	), ns, "hub1-cert")
	trans := newBYOTransporter(t, byoMGH(ns), hub1, managerNamed)

	if _, err := trans.EnsureUser("hub1"); err != nil {
		t.Fatalf("EnsureUser(hub1) error = %v", err)
	}
}

// TestBYOTransporterEnsureTopicIncludesMigrationTopic includes the migration topic in BYO cluster topics.
func TestBYOTransporterEnsureTopicIncludesMigrationTopic(t *testing.T) {
	trans := NewBYOTransporter(context.Background(), types.NamespacedName{
		Name: constants.GHTransportSecretName, Namespace: "test-ns",
	}, fake.NewClientBuilder().Build())
	topic, err := trans.EnsureTopic("hub1")
	if err != nil {
		t.Fatalf("EnsureTopic() error = %v", err)
	}
	if topic.MigrationTopic != config.GetMigrationTopic() {
		t.Fatalf("MigrationTopic = %q, want %q", topic.MigrationTopic, config.GetMigrationTopic())
	}
}

// mustBYOCertPEM builds a PEM-encoded test certificate with the given CommonName.
func mustBYOCertPEM(t *testing.T, commonName string) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
