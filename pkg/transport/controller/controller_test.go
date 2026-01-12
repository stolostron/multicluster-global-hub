package controller

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	inventoryclient "github.com/project-kessel/inventory-client-go/v1beta1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestSecretCtrlReconcile(t *testing.T) {
	// Set up a fake Kubernetes client
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := context.TODO()

	// Create mock manager
	mockMgr := &mockManager{
		client: fakeClient,
		ctx:    ctx,
	}

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
		transportCallback: func(transportClient transport.TransportClient) error {
			callbackInvoked = true
			return nil
		},
		transportClient:     &TransportClient{},
		runtimeClient:       fakeClient,
		manager:             mockMgr,
		producerTopic:       "event",
		transportConfigChan: make(chan *transport.TransportInternalConfig, 1),
	}

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
type mockConsumer struct{}

func (mc *mockConsumer) Start(ctx context.Context) error {
	return nil
}

func (mc *mockConsumer) EventChan() chan *cloudevents.Event {
	return make(chan *cloudevents.Event)
}

// mockManager implements a minimal manager.Manager for testing
type mockManager struct {
	client client.Client
	ctx    context.Context
}

func (m *mockManager) Add(runnable manager.Runnable) error {
	// Start the runnable in a goroutine like the real manager does
	go func() {
		_ = runnable.Start(m.ctx)
	}()
	return nil
}

func (m *mockManager) GetClient() client.Client                                 { return m.client }
func (m *mockManager) GetScheme() *runtime.Scheme                               { return nil }
func (m *mockManager) GetFieldIndexer() client.FieldIndexer                     { return nil }
func (m *mockManager) GetCache() cache.Cache                                    { return nil }
func (m *mockManager) GetEventRecorderFor(name string) record.EventRecorder     { return nil }
func (m *mockManager) GetRESTMapper() meta.RESTMapper                           { return nil }
func (m *mockManager) GetAPIReader() client.Reader                              { return nil }
func (m *mockManager) Start(ctx context.Context) error                          { return nil }
func (m *mockManager) GetWebhookServer() webhook.Server                         { return nil }
func (m *mockManager) GetLogger() logr.Logger                                   { return logr.Discard() }
func (m *mockManager) GetControllerOptions() config.Controller                  { return config.Controller{} }
func (m *mockManager) Elected() <-chan struct{}                                 { return nil }
func (m *mockManager) AddHealthzCheck(name string, check healthz.Checker) error { return nil }
func (m *mockManager) AddReadyzCheck(name string, check healthz.Checker) error  { return nil }
func (m *mockManager) GetHTTPClient() *http.Client                              { return nil }
func (m *mockManager) AddMetricsServerExtraHandler(path string, handler http.Handler) error {
	return nil
}
func (m *mockManager) GetConfig() *rest.Config { return nil }

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
			transportClient:     &TransportClient{},
			transportConfigChan: make(chan *transport.TransportInternalConfig, 1),
			inManager:           false,
		}

		err := ctrl.ReconcileConsumer(ctx)
		assert.NoError(t, err)
		assert.Nil(t, ctrl.transportClient.consumer)
	})

	// Test case 2: Consumer already exists, should send config to channel
	t.Run("Consumer exists - send config to channel", func(t *testing.T) {
		mock := &mockConsumer{}
		transportConfigChan := make(chan *transport.TransportInternalConfig, 1) // buffered to avoid blocking
		transportConfig := &transport.TransportInternalConfig{
			KafkaCredential: &transport.KafkaConfig{
				ConsumerGroupID: "test-group",
				SpecTopic:       "spec-topic",
				StatusTopic:     "status-topic",
				BootstrapServer: "localhost:9092",
			},
		}
		ctrl := &TransportCtrl{
			transportConfig: transportConfig,
			transportClient: &TransportClient{
				consumer: mock, // Existing consumer
			},
			transportConfigChan: transportConfigChan,
			inManager:           true,
		}

		err := ctrl.ReconcileConsumer(ctx)
		assert.NoError(t, err)

		// Verify config was sent to channel
		select {
		case receivedConfig := <-transportConfigChan:
			assert.Equal(t, transportConfig, receivedConfig)
		default:
			t.Error("Expected config to be sent to channel")
		}
	})

	// Test case 3: Consumer disabled, should skip
	t.Run("Skip when consumer is disabled", func(t *testing.T) {
		ctrl := &TransportCtrl{
			transportConfig: &transport.TransportInternalConfig{
				KafkaCredential: &transport.KafkaConfig{
					ConsumerGroupID: "test-group",
					SpecTopic:       "spec-topic",
					StatusTopic:     "status-topic",
				},
			},
			transportClient:     &TransportClient{},
			transportConfigChan: make(chan *transport.TransportInternalConfig, 1),
			disableConsumer:     true,
		}

		err := ctrl.ReconcileConsumer(ctx)
		assert.NoError(t, err)
		assert.Nil(t, ctrl.transportClient.consumer)
	})
}

func TestNewTransportCtrl(t *testing.T) {
	callback := func(tc transport.TransportClient) error {
		return nil
	}
	config := &transport.TransportInternalConfig{
		TransportType: string(transport.Chan),
	}

	ctrl := NewTransportCtrl("test-ns", "test-secret", callback, config, true)

	assert.NotNil(t, ctrl)
	assert.Equal(t, "test-ns", ctrl.secretNamespace)
	assert.Equal(t, "test-secret", ctrl.secretName)
	assert.NotNil(t, ctrl.transportCallback)
	assert.NotNil(t, ctrl.transportClient)
	assert.Equal(t, config, ctrl.transportConfig)
	assert.NotNil(t, ctrl.transportConfigChan)
	assert.Equal(t, true, ctrl.inManager)
	assert.Equal(t, false, ctrl.disableConsumer)
}

func TestDisableConsumer(t *testing.T) {
	ctrl := NewTransportCtrl("test-ns", "test-secret", nil, &transport.TransportInternalConfig{}, false)

	assert.False(t, ctrl.disableConsumer)

	ctrl.DisableConsumer()

	assert.True(t, ctrl.disableConsumer)
}

func TestCredentialSecret(t *testing.T) {
	ctrl := &TransportCtrl{
		secretName:       "main-secret",
		extraSecretNames: []string{"extra-secret-1", "extra-secret-2"},
	}

	t.Run("returns true for main secret", func(t *testing.T) {
		assert.True(t, ctrl.credentialSecret("main-secret"))
	})

	t.Run("returns true for extra secrets", func(t *testing.T) {
		assert.True(t, ctrl.credentialSecret("extra-secret-1"))
		assert.True(t, ctrl.credentialSecret("extra-secret-2"))
	})

	t.Run("returns false for unknown secret", func(t *testing.T) {
		assert.False(t, ctrl.credentialSecret("unknown-secret"))
	})

	t.Run("returns false when extra secrets is empty", func(t *testing.T) {
		ctrlNoExtra := &TransportCtrl{
			secretName:       "main-secret",
			extraSecretNames: []string{},
		}
		assert.False(t, ctrlNoExtra.credentialSecret("unknown-secret"))
	})
}

func TestTransportClientGettersSetters(t *testing.T) {
	tc := &TransportClient{}

	t.Run("producer getter/setter", func(t *testing.T) {
		assert.Nil(t, tc.GetProducer())

		mockProducer := &mockProducerImpl{}
		tc.SetProducer(mockProducer)

		assert.Equal(t, mockProducer, tc.GetProducer())
	})

	t.Run("consumer getter/setter", func(t *testing.T) {
		assert.Nil(t, tc.GetConsumer())

		mockCons := &mockConsumer{}
		tc.SetConsumer(mockCons)

		assert.Equal(t, mockCons, tc.GetConsumer())
	})

	t.Run("requester getter/setter", func(t *testing.T) {
		assert.Nil(t, tc.GetRequester())

		mockReq := &mockRequester{}
		tc.SetRequester(mockReq)

		assert.Equal(t, mockReq, tc.GetRequester())
	})
}

// Mock producer for testing
type mockProducerImpl struct{}

func (mp *mockProducerImpl) SendEvent(ctx context.Context, evt cloudevents.Event) error {
	return nil
}

func (mp *mockProducerImpl) Reconnect(config *transport.TransportInternalConfig, topic string) error {
	return nil
}

// Mock requester for testing
type mockRequester struct{}

func (mr *mockRequester) RefreshClient(ctx context.Context, restfulConn *transport.RestfulConfig) error {
	return nil
}

func (mr *mockRequester) GetHttpClient() *inventoryclient.InventoryHttpClient {
	return nil
}

func TestReconcileProducer(t *testing.T) {
	t.Run("create producer for manager", func(t *testing.T) {
		ctrl := &TransportCtrl{
			transportConfig: &transport.TransportInternalConfig{
				TransportType: string(transport.Chan),
				KafkaCredential: &transport.KafkaConfig{
					SpecTopic:   "spec-topic",
					StatusTopic: "status-topic",
				},
				Extends: make(map[string]any),
			},
			transportClient: &TransportClient{},
			inManager:       true,
		}

		err := ctrl.ReconcileProducer()
		assert.NoError(t, err)
		assert.NotNil(t, ctrl.transportClient.producer)
		assert.Equal(t, "spec-topic", ctrl.producerTopic)
	})

	t.Run("create producer for agent", func(t *testing.T) {
		ctrl := &TransportCtrl{
			transportConfig: &transport.TransportInternalConfig{
				TransportType: string(transport.Chan),
				KafkaCredential: &transport.KafkaConfig{
					SpecTopic:   "spec-topic",
					StatusTopic: "status-topic",
				},
				Extends: make(map[string]any),
			},
			transportClient: &TransportClient{},
			inManager:       false,
		}

		err := ctrl.ReconcileProducer()
		assert.NoError(t, err)
		assert.NotNil(t, ctrl.transportClient.producer)
		assert.Equal(t, "status-topic", ctrl.producerTopic)
	})

	t.Run("reconnect existing producer", func(t *testing.T) {
		mockProd := &mockProducerImpl{}
		ctrl := &TransportCtrl{
			transportConfig: &transport.TransportInternalConfig{
				TransportType: string(transport.Chan),
				KafkaCredential: &transport.KafkaConfig{
					SpecTopic:   "spec-topic",
					StatusTopic: "status-topic",
				},
				Extends: make(map[string]any),
			},
			transportClient: &TransportClient{
				producer: mockProd,
			},
			inManager: true,
		}

		err := ctrl.ReconcileProducer()
		assert.NoError(t, err)
		assert.Equal(t, "spec-topic", ctrl.producerTopic)
	})
}
