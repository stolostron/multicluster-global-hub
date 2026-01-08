package controller

import (
	"context"
	"encoding/base64"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestSecretCtrlReconcile(t *testing.T) {
	// Set up a fake Kubernetes client
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	callbackInvoked := false

	secretController := &TransportCtrl{
		secretNamespace: "default",
		secretName:      "test-secret",
		transportConfig: &transport.TransportInternalConfig{
			TransportType: string(transport.Chan),
			KafkaCredential: &transport.KafkaConfig{
				SpecTopic:       "spec",
				StatusTopic:     "event",
				ConsumerGroupID: "test",
			},
			FailureThreshold: 100,
		},
		workqueue: workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request](), workqueue.TypedRateLimitingQueueConfig[ctrl.Request]{
			Name: "controllerName",
		}),
		transportCallback: func(transportClient transport.TransportClient) error {
			callbackInvoked = true
			return nil
		},
		transportClient: &TransportClient{},
		runtimeClient:   fakeClient,
		producerTopic:   "event",
		consumerTopics:  []string{"spec"},
	}

	ctx := context.TODO()

	kafkaConn := &transport.KafkaConfig{
		BootstrapServer: "localhost:3031",
		StatusTopic:     "event",
		SpecTopic:       "spec",
		ConsumerGroupID: "test",
		ClusterID:       "123",
		CACert:          base64.StdEncoding.EncodeToString([]byte("11")),
		ClientCert:      base64.StdEncoding.EncodeToString([]byte("12")),
		ClientKey:       base64.StdEncoding.EncodeToString([]byte("13")),
	}

	kafkaConnYaml, err := kafkaConn.YamlMarshal(false)
	assert.NoError(t, err)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-secret",
		},
		Data: map[string][]byte{
			"kafka.yaml": kafkaConnYaml,
		},
	}
	_ = fakeClient.Create(ctx, secret)

	// Reconcile
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-secret",
		},
	}
	result, err := secretController.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.False(t, result.Requeue)
	assert.NotNil(t, secretController.transportClient.producer)
	// cannot assert consumer, because the consumer can be closed since we do not have a real kafka
	// assert.NotNil(t, secretController.transportClient.consumer)
	assert.True(t, callbackInvoked)

	// Test when transport config changes
	result, err = secretController.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.False(t, result.Requeue)
	utils.PrettyPrint(secretController.transportConfig.RestfulCredential)
}

func TestInventorySecretCtrlReconcile(t *testing.T) {
	// Set up a fake Kubernetes client
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	callbackInvoked := false

	secretController := &TransportCtrl{
		secretNamespace: "default",
		secretName:      "test-secret",
		transportConfig: &transport.TransportInternalConfig{
			FailureThreshold: 100,
		},
		transportCallback: func(transport.TransportClient) error {
			callbackInvoked = true
			return nil
		},
		transportClient: &TransportClient{},
		runtimeClient:   fakeClient,
		workqueue: workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request](), workqueue.TypedRateLimitingQueueConfig[ctrl.Request]{
			Name: "controllerName",
		}),
		disableConsumer: true,
	}

	ctx := context.TODO()

	restfulConn := &transport.RestfulConfig{
		Host:       "localhost:123",
		CACert:     base64.StdEncoding.EncodeToString(rootPEM),
		ClientCert: base64.StdEncoding.EncodeToString(certPem),
		ClientKey:  base64.StdEncoding.EncodeToString(keyPem),
	}

	restfulConnYaml, err := restfulConn.YamlMarshal(true)
	assert.NoError(t, err)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-secret",
		},
		Data: map[string][]byte{
			"rest.yaml": restfulConnYaml,
		},
	}
	_ = fakeClient.Create(ctx, secret)

	// Reconcile
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-secret",
		},
	}
	result, err := secretController.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.False(t, result.Requeue)
	assert.Nil(t, secretController.transportClient.producer)
	assert.Nil(t, secretController.transportClient.consumer)
	assert.NotNil(t, secretController.transportClient.requester)
	assert.True(t, callbackInvoked)

	// Test when transport config changes
	result, err = secretController.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.False(t, result.Requeue)
}

var rootPEM = []byte(`
-- GlobalSign Root R2, valid until Dec 15, 2021
-----BEGIN CERTIFICATE-----
MIIDujCCAqKgAwIBAgILBAAAAAABD4Ym5g0wDQYJKoZIhvcNAQEFBQAwTDEgMB4G
A1UECxMXR2xvYmFsU2lnbiBSb290IENBIC0gUjIxEzARBgNVBAoTCkdsb2JhbFNp
Z24xEzARBgNVBAMTCkdsb2JhbFNpZ24wHhcNMDYxMjE1MDgwMDAwWhcNMjExMjE1
MDgwMDAwWjBMMSAwHgYDVQQLExdHbG9iYWxTaWduIFJvb3QgQ0EgLSBSMjETMBEG
A1UEChMKR2xvYmFsU2lnbjETMBEGA1UEAxMKR2xvYmFsU2lnbjCCASIwDQYJKoZI
hvcNAQEBBQADggEPADCCAQoCggEBAKbPJA6+Lm8omUVCxKs+IVSbC9N/hHD6ErPL
v4dfxn+G07IwXNb9rfF73OX4YJYJkhD10FPe+3t+c4isUoh7SqbKSaZeqKeMWhG8
eoLrvozps6yWJQeXSpkqBy+0Hne/ig+1AnwblrjFuTosvNYSuetZfeLQBoZfXklq
tTleiDTsvHgMCJiEbKjNS7SgfQx5TfC4LcshytVsW33hoCmEofnTlEnLJGKRILzd
C9XZzPnqJworc5HGnRusyMvo4KD0L5CLTfuwNhv2GXqF4G3yYROIXJ/gkwpRl4pa
zq+r1feqCapgvdzZX99yqWATXgAByUr6P6TqBwMhAo6CygPCm48CAwEAAaOBnDCB
mTAOBgNVHQ8BAf8EBAMCAQYwDwYDVR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQUm+IH
V2ccHsBqBt5ZtJot39wZhi4wNgYDVR0fBC8wLTAroCmgJ4YlaHR0cDovL2NybC5n
bG9iYWxzaWduLm5ldC9yb290LXIyLmNybDAfBgNVHSMEGDAWgBSb4gdXZxwewGoG
3lm0mi3f3BmGLjANBgkqhkiG9w0BAQUFAAOCAQEAmYFThxxol4aR7OBKuEQLq4Gs
J0/WwbgcQ3izDJr86iw8bmEbTUsp9Z8FHSbBuOmDAGJFtqkIk7mpM0sYmsL4h4hO
291xNBrBVNpGP+DTKqttVCL1OmLNIG+6KYnX3ZHu01yiPqFbQfXf5WRDLenVOavS
ot+3i9DAgBkcRcAtjOj4LaR0VknFBbVPFd5uRHg5h6h+u/N5GJG79G+dwfCMNYxd
AfvDbbnvRG15RjF+Cv6pgsH/76tuIMRQyV+dTZsXjAzlAcmgQWpzU/qlULRuJQ/7
TBj0/VLZjmmx6BEP3ojY+x1J96relc8geMJgEtslQIxq/H5COEBkEveegeGTLg==
-----END CERTIFICATE-----`)

var certPem = []byte(`-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIQIRi6zePL6mKjOipn+dNuaTAKBggqhkjOPQQDAjASMRAw
DgYDVQQKEwdBY21lIENvMB4XDTE3MTAyMDE5NDMwNloXDTE4MTAyMDE5NDMwNlow
EjEQMA4GA1UEChMHQWNtZSBDbzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABD0d
7VNhbWvZLWPuj/RtHFjvtJBEwOkhbN/BnnE8rnZR8+sbwnc/KhCk3FhnpHZnQz7B
5aETbbIgmuvewdjvSBSjYzBhMA4GA1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggr
BgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MCkGA1UdEQQiMCCCDmxvY2FsaG9zdDo1
NDUzgg4xMjcuMC4wLjE6NTQ1MzAKBggqhkjOPQQDAgNIADBFAiEA2zpJEPQyz6/l
Wf86aX6PepsntZv2GYlA5UpabfT2EZICICpJ5h/iI+i341gBmLiAFQOyTDT+/wQc
6MF9+Yw1Yy0t
-----END CERTIFICATE-----`)

var keyPem = []byte("-----BEGIN EC PRIVATE KEY-----\nMHcCAQEEIIrYSSNQFaA2Hwf1duRSxKtLYX5CB04fSeQ6tF1aY/PuoAoGCCqGSM49\nAwEHoUQDQgAEPR3tU2Fta9ktY+6P9G0cWO+0kETA6SFs38GecTyudlHz6xvCdz8q\nEKTcWGekdmdDPsHloRNtsiCa697B2O9IFA==\n-----END EC PRIVATE KEY-----") // notsecret

func TestTransportCtrl_ResyncKafkaClientSecret(t *testing.T) {
	tests := []struct {
		name        string
		kafkaConn   *transport.KafkaConfig
		secret      *corev1.Secret
		initObjects []runtime.Object
	}{
		{
			name: "default install",
			kafkaConn: &transport.KafkaConfig{
				ClusterID: "0001",
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "transport-config",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.secret != nil {
				tt.initObjects = append(tt.initObjects, tt.secret)
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.initObjects...).Build()

			c := &TransportCtrl{
				runtimeClient: fakeClient,
			}
			if err := c.ResyncKafkaClientSecret(context.Background(), tt.kafkaConn, tt.secret); err != nil {
				t.Errorf("TransportCtrl.ResyncKafkaClientSecret() error = %v", err)
			}
			secret := &corev1.Secret{}
			if err := c.runtimeClient.Get(context.Background(), client.ObjectKeyFromObject(tt.secret), secret); err != nil {
				if secret.Annotations[constants.KafkaClusterIdAnnotation] != tt.kafkaConn.ClusterID {
					t.Errorf("secret.Annotations[constants.KafkaClusterIdAnnotation]:%v,tt.kafkaConn.ClusterID:%v",
						secret.Annotations[constants.KafkaClusterIdAnnotation], tt.kafkaConn.ClusterID)
				}
			}
		})
	}
}

// Mock consumer for testing
type mockConsumer struct {
	reconnectCalled bool
	reconnectErr    error
}

func (mc *mockConsumer) Start(ctx context.Context) error {
	return nil
}

func (mc *mockConsumer) Reconnect(ctx context.Context, cfg *transport.TransportInternalConfig, topics []string) error {
	mc.reconnectCalled = true
	return mc.reconnectErr
}

func (mc *mockConsumer) EventChan() chan *cloudevents.Event {
	return make(chan *cloudevents.Event)
}

func TestTransportCtrl_ReconcileConsumer(t *testing.T) {
	ctx := context.TODO()

	// Test case 1: ConsumerGroupID is empty, should skip initializing consumer
	t.Run("Skip when ConsumerGroupID is empty", func(t *testing.T) {
		ctrl := &TransportCtrl{
			transportConfig: &transport.TransportInternalConfig{
				KafkaCredential: &transport.KafkaConfig{
					ConsumerGroupID: "", // empty means standalone mode
					SpecTopic:       "spec-topic",
					StatusTopic:     "status-topic",
				},
			},
			transportClient: &TransportClient{},
			inManager:       false,
		}

		err := ctrl.ReconcileConsumer(ctx)
		assert.NoError(t, err)
		assert.Nil(t, ctrl.transportClient.consumer)
	})

	// Test case 2: Valid consumer group ID but no valid kafka config (should fail gracefully)
	t.Run("Handle invalid kafka config", func(t *testing.T) {
		ctrl := &TransportCtrl{
			transportConfig: &transport.TransportInternalConfig{
				KafkaCredential: &transport.KafkaConfig{
					ConsumerGroupID: "test-group",
					SpecTopic:       "spec-topic",
					StatusTopic:     "status-topic",
					// Missing bootstrap server and other required fields
				},
			},
			transportClient: &TransportClient{},
			inManager:       false,
		}

		err := ctrl.ReconcileConsumer(ctx)
		// Should return an error because the kafka config is invalid
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create the consumer")
	})

	// Test case 3: Consumer already exists, should call Reconnect and set consumerRunning
	t.Run("Reconnect existing consumer and set consumerRunning", func(t *testing.T) {
		mock := &mockConsumer{}
		ctrl := &TransportCtrl{
			transportConfig: &transport.TransportInternalConfig{
				KafkaCredential: &transport.KafkaConfig{
					ConsumerGroupID: "test-group",
					SpecTopic:       "spec-topic",
					StatusTopic:     "status-topic",
					BootstrapServer: "localhost:9092", // Add required field
				},
			},
			transportClient: &TransportClient{
				consumer: mock, // Existing consumer
			},
			inManager:       true,
			consumerRunning: false, // Initially not started
		}

		err := ctrl.ReconcileConsumer(ctx)
		assert.NoError(t, err)
		assert.True(t, mock.reconnectCalled)
		assert.True(t, ctrl.consumerRunning, "consumerRunning should be true after successful ReconcileConsumer")
	})
}

func TestTransportCtrl_ConsumerStartedFlag(t *testing.T) {
	// Test that consumerRunning flag is properly initialized
	t.Run("NewTransportCtrl initializes consumerRunning to false", func(t *testing.T) {
		ctrl := NewTransportCtrl("default", "test-secret", nil,
			&transport.TransportInternalConfig{
				KafkaCredential: &transport.KafkaConfig{},
			}, true)

		assert.False(t, ctrl.consumerRunning, "consumerRunning should be false initially")
	})

	// Test that consumerRunning flag triggers reconciliation
	t.Run("Reconcile triggers consumer reconciliation when consumerRunning is false", func(t *testing.T) {
		// Set up a fake Kubernetes client
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		mock := &mockConsumer{}
		kafkaConn := &transport.KafkaConfig{
			BootstrapServer: "localhost:3031",
			StatusTopic:     "event",
			SpecTopic:       "spec",
			ConsumerGroupID: "test",
			ClusterID:       "123",
			CACert:          base64.StdEncoding.EncodeToString([]byte("11")),
			ClientCert:      base64.StdEncoding.EncodeToString([]byte("12")),
			ClientKey:       base64.StdEncoding.EncodeToString([]byte("13")),
		}
		kafkaConnYaml, err := kafkaConn.YamlMarshal(false)
		assert.NoError(t, err)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test-secret",
			},
			Data: map[string][]byte{
				"kafka.yaml": kafkaConnYaml,
			},
		}
		ctx := context.TODO()
		_ = fakeClient.Create(ctx, secret)

		ctrl := &TransportCtrl{
			secretNamespace: "default",
			secretName:      "test-secret",
			transportConfig: &transport.TransportInternalConfig{
				TransportType: string(transport.Chan),
				KafkaCredential: &transport.KafkaConfig{
					BootstrapServer: "localhost:3031",
					SpecTopic:       "spec",
					StatusTopic:     "event",
					ConsumerGroupID: "test",
					ClusterID:       "123",
				},
				FailureThreshold: 100,
			},
			workqueue: workqueue.NewTypedRateLimitingQueueWithConfig(
				workqueue.DefaultTypedControllerRateLimiter[reconcile.Request](),
				workqueue.TypedRateLimitingQueueConfig[ctrl.Request]{
					Name: "controllerName",
				}),
			transportClient: &TransportClient{
				consumer: mock, // Consumer exists but not started
			},
			runtimeClient:   fakeClient,
			producerTopic:   "event",
			consumerTopics:  []string{"spec"},
			inManager:       false,
			consumerRunning: false, // Consumer stopped, should trigger reconnect
		}

		// ReconcileConsumer should be called because consumerRunning is false
		err = ctrl.ReconcileConsumer(ctx)
		assert.NoError(t, err)
		assert.True(t, mock.reconnectCalled, "Reconnect should be called when consumerRunning is false")
		assert.True(t, ctrl.consumerRunning, "consumerRunning should be true after reconciliation")
	})

	// Test that consumer already started doesn't trigger unnecessary reconciliation
	t.Run("Consumer already exists and started - Reconnect still called on ReconcileConsumer", func(t *testing.T) {
		mock := &mockConsumer{}
		ctrl := &TransportCtrl{
			transportConfig: &transport.TransportInternalConfig{
				KafkaCredential: &transport.KafkaConfig{
					ConsumerGroupID: "test-group",
					SpecTopic:       "spec-topic",
					StatusTopic:     "status-topic",
					BootstrapServer: "localhost:9092",
				},
			},
			transportClient: &TransportClient{
				consumer: mock,
			},
			inManager:       true,
			consumerRunning: true, // Already started
		}

		// ReconcileConsumer should still call Reconnect (it always does when consumer exists)
		err := ctrl.ReconcileConsumer(context.TODO())
		assert.NoError(t, err)
		assert.True(t, mock.reconnectCalled)
		assert.True(t, ctrl.consumerRunning)
	})
}
