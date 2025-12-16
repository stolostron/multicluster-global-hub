package utils

import (
	"encoding/base64"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetKafkaCredentialBySecret(t *testing.T) {
	namespace := "default"

	// Create test certificates
	caCert := "test-ca-cert"
	clientCert := "test-client-cert"
	clientKey := "test-client-key"

	// Encode certificates to base64
	caCertEncoded := base64.StdEncoding.EncodeToString([]byte(caCert))
	clientCertEncoded := base64.StdEncoding.EncodeToString([]byte(clientCert))
	clientKeyEncoded := base64.StdEncoding.EncodeToString([]byte(clientKey))

	tests := []struct {
		name        string
		secret      *corev1.Secret
		wantErr     bool
		errContains string
	}{
		{
			name: "valid kafka config with base64 encoded certs",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "transport-config",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"kafka.yaml": []byte(`bootstrap.server: "kafka-broker:9092"
ca.crt: "` + caCertEncoded + `"
client.crt: "` + clientCertEncoded + `"
client.key: "` + clientKeyEncoded + `"
`),
				},
			},
			wantErr: false,
		},
		{
			name: "missing kafka.yaml",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "transport-config",
					Namespace: namespace,
				},
				Data: map[string][]byte{},
			},
			wantErr:     true,
			errContains: "must set the `kafka.yaml`",
		},
		{
			name: "invalid yaml format",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "transport-config",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"kafka.yaml": []byte("invalid: yaml: content: ["),
				},
			},
			wantErr:     true,
			errContains: "failed to unmarshal",
		},
		{
			name: "invalid base64 in ca cert",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "transport-config",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"kafka.yaml": []byte(`bootstrap.server: "kafka-broker:9092"
ca.crt: "invalid-base64!!!"
`),
				},
			},
			wantErr:     true,
			errContains: "failed to decode credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()

			kafkaConfig, err := GetKafkaCredentialBySecret(tt.secret, fakeClient)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetKafkaCredentialBySecret() expected error but got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("GetKafkaCredentialBySecret() error = %v, want error containing %v", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("GetKafkaCredentialBySecret() unexpected error = %v", err)
				return
			}

			// Verify decoded certificates
			if kafkaConfig.CACert != caCert {
				t.Errorf("CACert = %v, want %v", kafkaConfig.CACert, caCert)
			}
			if kafkaConfig.ClientCert != clientCert {
				t.Errorf("ClientCert = %v, want %v", kafkaConfig.ClientCert, clientCert)
			}
			if kafkaConfig.ClientKey != clientKey {
				t.Errorf("ClientKey = %v, want %v", kafkaConfig.ClientKey, clientKey)
			}
		})
	}
}

func TestDecodeTransportCertificate_FromSecretReference(t *testing.T) {
	namespace := "default"

	// Create CA secret
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kafka-ca-cert",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"ca.crt": []byte("test-ca-cert-from-secret"),
		},
	}

	// Create client secret
	clientSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kafka-client-cert",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"tls.crt": []byte("test-client-cert-from-secret"),
			"tls.key": []byte("test-client-key-from-secret"),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(caSecret, clientSecret).
		Build()

	// Create transport config secret referencing other secrets
	transportSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "transport-config",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"kafka.yaml": []byte(`bootstrap.server: "kafka-broker:9092"
ca.secret: "kafka-ca-cert"
client.secret: "kafka-client-cert"
`),
		},
	}

	kafkaConfig, err := GetKafkaCredentialBySecret(transportSecret, fakeClient)
	if err != nil {
		t.Fatalf("GetKafkaCredentialBySecret() unexpected error = %v", err)
	}

	// Verify certificates loaded from referenced secrets
	if kafkaConfig.CACert != "test-ca-cert-from-secret" {
		t.Errorf("CACert = %v, want %v", kafkaConfig.CACert, "test-ca-cert-from-secret")
	}
	if kafkaConfig.ClientCert != "test-client-cert-from-secret" {
		t.Errorf("ClientCert = %v, want %v", kafkaConfig.ClientCert, "test-client-cert-from-secret")
	}
	if kafkaConfig.ClientKey != "test-client-key-from-secret" {
		t.Errorf("ClientKey = %v, want %v", kafkaConfig.ClientKey, "test-client-key-from-secret")
	}
}

func TestDecodeTransportCertificate_MissingClientSecret(t *testing.T) {
	namespace := "default"

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		Build()

	transportSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "transport-config",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"kafka.yaml": []byte(`bootstrap.server: "kafka-broker:9092"
client.secret: "non-existent-secret"
`),
		},
	}

	_, err := GetKafkaCredentialBySecret(transportSecret, fakeClient)
	if err == nil {
		t.Error("GetKafkaCredentialBySecret() expected error for missing secret but got nil")
	}
	if !contains(err.Error(), "failed to get the client cert") {
		t.Errorf("GetKafkaCredentialBySecret() error = %v, want error containing 'failed to get the client cert'", err)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
