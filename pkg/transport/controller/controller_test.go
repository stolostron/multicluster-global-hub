package controller

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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
		transportConfig: &transport.TransportConfig{
			TransportType:   string(transport.Chan),
			ConsumerGroupId: "test",
		},
		callback: func(p transport.Producer, c transport.Consumer) error {
			callbackInvoked = true
			return nil
		},
		runtimeClient: fakeClient,
	}

	ctx := context.TODO()

	kafkaConn := &transport.KafkaConnCredential{
		BootstrapServer: "localhost:3031",
		StatusTopic:     "event",
		SpecTopic:       "spec",
		ClusterID:       "123",
		// the following fields are only for the manager, and the agent of byo/standalone kafka
		CACert:     base64.StdEncoding.EncodeToString([]byte("11")),
		ClientCert: base64.StdEncoding.EncodeToString([]byte("12")),
		ClientKey:  base64.StdEncoding.EncodeToString([]byte("13")),
	}

	kafkaConnYaml, err := kafkaConn.YamlMarshal(false)
	assert.NoError(t, err)

	inventoryConn := &transport.RestfulConnCredentail{
		Host: "localhost:123",
		// the following fields are only for the manager, and the agent of byo/standalone kafka
		CACert:     base64.StdEncoding.EncodeToString([]byte("11")),
		ClientCert: base64.StdEncoding.EncodeToString([]byte("12")),
		ClientKey:  base64.StdEncoding.EncodeToString([]byte("13")),
	}

	inventoryConnYaml, err := inventoryConn.YamlMarshal()
	assert.NoError(t, err)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-secret",
		},
		Data: map[string][]byte{
			"kafka.yaml":     kafkaConnYaml,
			"inventory.yaml": inventoryConnYaml,
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
	assert.NotNil(t, secretController.producer)
	assert.NotNil(t, secretController.consumer)
	assert.True(t, callbackInvoked)

	// Test when transport config changes
	result, err = secretController.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.False(t, result.Requeue)
	utils.PrettyPrint(secretController.transportConfig.RestfulCredentail)
}
