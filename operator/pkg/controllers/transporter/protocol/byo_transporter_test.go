// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package protocol

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	operatorv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	operatorconfig "github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestBYOTransporterEnsureTopicIncludesMigrationTopic(t *testing.T) {
	restoreBYOTransporterState(t)

	trans := NewBYOTransporter(context.Background(), types.NamespacedName{
		Name:      constants.GHTransportSecretName,
		Namespace: "test-ns",
	}, fake.NewClientBuilder().Build())

	topic, err := trans.EnsureTopic("hub1")
	if err != nil {
		t.Fatalf("EnsureTopic() error = %v", err)
	}
	if topic.MigrationTopic != operatorconfig.GetMigrationTopic() {
		t.Fatalf("MigrationTopic = %q, want %q", topic.MigrationTopic, operatorconfig.GetMigrationTopic())
	}
	if topic.SpecTopic != operatorconfig.GetSpecTopic() {
		t.Fatalf("SpecTopic = %q, want %q", topic.SpecTopic, operatorconfig.GetSpecTopic())
	}
}

func TestBYOTransporterEnsureUserAndPruneAreNoOps(t *testing.T) {
	restoreBYOTransporterState(t)

	trans := NewBYOTransporter(context.Background(), types.NamespacedName{
		Name: constants.GHTransportSecretName, Namespace: "test-ns",
	}, fake.NewClientBuilder().Build())

	if _, err := trans.EnsureUser("hub1"); err != nil {
		t.Fatalf("EnsureUser() error = %v", err)
	}
	if ready, err := trans.EnsureKafka(); err != nil || ready {
		t.Fatalf("EnsureKafka() = (%v, %v), want (false, nil)", ready, err)
	}
	if err := trans.Prun("hub1"); err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
}

func TestBYOTransporterGetConnCredential(t *testing.T) {
	restoreBYOTransporterState(t)

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	if err := operatorv1alpha4.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(MGH): %v", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.GHTransportSecretName,
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"bootstrap_server": []byte("kafka:9093"),
			"ca.crt":           []byte("ca-bytes"),
			"client.crt":       []byte("cert-bytes"),
			"client.key":       []byte("key-bytes"),
		},
	}
	mgh := &operatorv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "mgh", Namespace: "test-ns"},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, mgh).
		Build()

	trans := NewBYOTransporter(context.Background(), types.NamespacedName{
		Name: constants.GHTransportSecretName, Namespace: "test-ns",
	}, fakeClient)

	cfg, err := trans.GetConnCredential("hub1")
	if err != nil {
		t.Fatalf("GetConnCredential() error = %v", err)
	}
	if cfg.BootstrapServer != "kafka:9093" {
		t.Fatalf("BootstrapServer = %q, want kafka:9093", cfg.BootstrapServer)
	}
	if cfg.MigrationTopic != operatorconfig.GetMigrationTopic() {
		t.Fatalf("MigrationTopic = %q, want %q", cfg.MigrationTopic, operatorconfig.GetMigrationTopic())
	}
	if cfg.GetCACert() == "" || cfg.GetClientCert() == "" || cfg.GetClientKey() == "" {
		t.Fatal("expected encoded cert material in KafkaConfig")
	}
}

func TestBYOTransporterGetConnCredentialErrors(t *testing.T) {
	restoreBYOTransporterState(t)

	trans := NewBYOTransporter(context.Background(), types.NamespacedName{
		Name: "missing-secret", Namespace: "test-ns",
	}, fake.NewClientBuilder().Build())

	if _, err := trans.GetConnCredential("hub1"); err == nil {
		t.Fatal("GetConnCredential() expected error for missing secret")
	}
}

func restoreBYOTransporterState(t *testing.T) {
	t.Helper()

	originalTransporter := operatorconfig.GetTransporter()

	t.Cleanup(func() {
		byoTransporter = nil
		operatorconfig.SetTransporter(originalTransporter)
	})

	byoTransporter = nil
	operatorconfig.SetTransporter(nil)

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	mgh := &operatorv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "mgh", Namespace: "test-ns"},
		Spec: operatorv1alpha4.MulticlusterGlobalHubSpec{
			DataLayerSpec: operatorv1alpha4.DataLayerSpec{Kafka: operatorv1alpha4.KafkaSpec{}},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	if err := operatorconfig.SetTransportConfig(context.Background(), fakeClient, mgh); err != nil {
		t.Fatalf("SetTransportConfig() error = %v", err)
	}
}
