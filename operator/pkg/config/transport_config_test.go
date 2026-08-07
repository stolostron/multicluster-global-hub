package config

import (
	"context"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func TestSetKafkaClientCAConcurrentAccess(t *testing.T) {
	originalKey := kafkaClientCAKey
	originalCert := kafkaClientCACert
	t.Cleanup(func() {
		kafkaClientCAKey = originalKey
		kafkaClientCACert = originalCert
	})

	ctx := context.Background()
	testScheme := runtime.NewScheme()
	_ = corev1.AddToScheme(testScheme)
	ns := "test-ns"
	c := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "kafka-clients-ca", Namespace: ns},
			Data:       map[string][]byte{"ca.key": []byte("key-v1")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "kafka-clients-ca-cert", Namespace: ns},
			Data:       map[string][]byte{"ca.crt": []byte("cert-v1")},
		},
	).Build()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := SetKafkaClientCA(ctx, ns, "kafka", c); err != nil {
				t.Errorf("SetKafkaClientCA() error = %v", err)
			}
			key, cert := GetKafkaClientCA()
			if len(key) == 0 || len(cert) == 0 {
				t.Error("GetKafkaClientCA() returned empty snapshot")
			}
		}()
	}
	wg.Wait()

	key, cert := GetKafkaClientCA()
	if string(key) != "key-v1" || string(cert) != "cert-v1" {
		t.Fatalf("GetKafkaClientCA() = (%q, %q), want (key-v1, cert-v1)", key, cert)
	}
}
