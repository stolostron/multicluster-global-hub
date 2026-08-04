package config

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestGetMigrationTopic(t *testing.T) {
	original := migrationTopic
	t.Cleanup(func() { migrationTopic = original })

	migrationTopic = "gh-migration"
	if got := GetMigrationTopic(); got != "gh-migration" {
		t.Fatalf("GetMigrationTopic() = %q, want gh-migration", got)
	}
}

func TestGetKafkaUserName(t *testing.T) {
	if got := GetKafkaUserName("hub1"); got != "hub1-kafka-user" {
		t.Fatalf("GetKafkaUserName() = %q, want hub1-kafka-user", got)
	}
}

func TestSetMigrationTopic(t *testing.T) {
	original := migrationTopic
	t.Cleanup(func() { migrationTopic = original })

	SetMigrationTopic("custom-migration")
	if got := GetMigrationTopic(); got != "custom-migration" {
		t.Fatalf("SetMigrationTopic() = %q, want custom-migration", got)
	}
}

func TestGetStatusTopicAndManagerStatusTopic(t *testing.T) {
	original := statusTopic
	t.Cleanup(func() { statusTopic = original })

	statusTopic = "gh-status.*"
	if got := GetStatusTopic("cluster1"); got != "gh-status.cluster1" {
		t.Fatalf("GetStatusTopic() = %q, want gh-status.cluster1", got)
	}
	if got := ManagerStatusTopic(); got != "^gh-status.*" {
		t.Fatalf("ManagerStatusTopic() = %q, want ^gh-status.*", got)
	}

	statusTopic = "gh-status"
	if got := ManagerStatusTopic(); got != "gh-status" {
		t.Fatalf("ManagerStatusTopic() fixed topic = %q, want gh-status", got)
	}
	if got := GetRawStatusTopic(); got != "gh-status" {
		t.Fatalf("GetRawStatusTopic() = %q, want gh-status", got)
	}
}

func TestGetConsumerGroupID(t *testing.T) {
	if got := GetConsumerGroupID("", "hub-1"); got != "hub_1" {
		t.Fatalf("GetConsumerGroupID() = %q, want hub_1", got)
	}
	if got := GetConsumerGroupID("prefix", "hub-1"); got != "prefixhub_1" {
		t.Fatalf("GetConsumerGroupID(prefix) = %q, want prefixhub_1", got)
	}
}

func TestSetTransporterConn(t *testing.T) {
	original := transporterConn
	t.Cleanup(func() { transporterConn = original })

	if changed := SetTransporterConn(nil); !changed {
		t.Fatal("SetTransporterConn(nil) should report change when conn was set")
	}
	conn := &transport.KafkaConfig{SpecTopic: "gh-spec"}
	if !SetTransporterConn(conn) {
		t.Fatal("SetTransporterConn(new) should report change")
	}
	if SetTransporterConn(conn) {
		t.Fatal("SetTransporterConn(same) should not report change")
	}
	if got := GetTransporterConn(); got != conn {
		t.Fatal("GetTransporterConn() returned unexpected value")
	}
}

func TestSetTransportConfigDefaultsAndValidation(t *testing.T) {
	restoreTransportConfigState(t)

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "mgh", Namespace: "test-ns"},
		Spec: v1alpha4.MulticlusterGlobalHubSpec{
			DataLayerSpec: v1alpha4.DataLayerSpec{
				Kafka: v1alpha4.KafkaSpec{},
			},
		},
	}
	if err := SetTransportConfig(context.Background(), fakeClient, mgh); err != nil {
		t.Fatalf("SetTransportConfig() defaults error = %v", err)
	}
	if got := GetSpecTopic(); got != DEFAULT_SPEC_TOPIC {
		t.Fatalf("GetSpecTopic() = %q, want %q", got, DEFAULT_SPEC_TOPIC)
	}
	if got := GetMigrationTopic(); got != DEFAULT_MIGRATION_TOPIC {
		t.Fatalf("GetMigrationTopic() = %q, want %q", got, DEFAULT_MIGRATION_TOPIC)
	}

	mgh.Spec.DataLayerSpec.Kafka.KafkaTopics.SpecTopic = "invalid topic!"
	if err := SetTransportConfig(context.Background(), fakeClient, mgh); err == nil {
		t.Fatal("SetTransportConfig() expected invalid spec topic error")
	}
}

func TestSetTransportConfigBYOStatusTopic(t *testing.T) {
	restoreTransportConfigState(t)

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	if err := v1alpha4.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.GHTransportSecretName,
			Namespace: "test-ns",
		},
	}
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "mgh", Namespace: "test-ns"},
		Spec: v1alpha4.MulticlusterGlobalHubSpec{
			DataLayerSpec: v1alpha4.DataLayerSpec{
				Kafka: v1alpha4.KafkaSpec{
					KafkaTopics: v1alpha4.KafkaTopics{
						StatusTopic: DEFAULT_STATUS_TOPIC,
					},
				},
			},
		},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, mgh).
		Build()

	if err := SetTransportConfig(context.Background(), fakeClient, mgh); err != nil {
		t.Fatalf("SetTransportConfig() BYO error = %v", err)
	}
	if got := GetRawStatusTopic(); got != DEFAULT_SHARED_STATUS_TOPIC {
		t.Fatalf("GetRawStatusTopic() = %q, want %q", got, DEFAULT_SHARED_STATUS_TOPIC)
	}
	if !IsBYOKafka() {
		t.Fatal("expected BYO kafka to be enabled")
	}
}

func TestGetTransportConfigClientName(t *testing.T) {
	restoreTransportConfigState(t)

	transporterProtocol = transport.StrimziTransporter
	if got := GetTransportConfigClientName("hub1"); got != "hub1-kafka-user" {
		t.Fatalf("GetTransportConfigClientName() = %q, want hub1-kafka-user", got)
	}
}

func TestEnableInventory(t *testing.T) {
	restoreTransportConfigState(t)

	if EnableInventory() {
		t.Fatal("EnableInventory() = true, want false by default")
	}
	enableInventory = true
	if !EnableInventory() {
		t.Fatal("EnableInventory() = false, want true")
	}
}

func restoreTransportConfigState(t *testing.T) {
	t.Helper()

	origSpec := specTopic
	origMigration := migrationTopic
	origStatus := statusTopic
	origBYO := isBYOKafka
	origProtocol := transporterProtocol
	origInventory := enableInventory
	origConn := transporterConn

	t.Cleanup(func() {
		specTopic = origSpec
		migrationTopic = origMigration
		statusTopic = origStatus
		isBYOKafka = origBYO
		transporterProtocol = origProtocol
		enableInventory = origInventory
		transporterConn = origConn
	})

	specTopic = ""
	migrationTopic = ""
	statusTopic = ""
	isBYOKafka = false
	transporterProtocol = transport.StrimziTransporter
	enableInventory = false
	transporterConn = nil
}
