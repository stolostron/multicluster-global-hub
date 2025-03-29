package agent

import (
	"context"
	"testing"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPruneAgentResources(t *testing.T) {
	// Define test namespace and transport secret name
	namespace := "test-namespace"

	// Create a fake client with initial objects
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	_ = kafkav1beta2.AddToScheme(scheme)

	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      transportSecretName,
			Namespace: namespace,
			Labels: map[string]string{
				"component": "multicluster-global-hub-agent",
			},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingSecret).Build()

	// Call pruneAgentResources
	err := pruneAgentResources(context.TODO(), fakeClient, namespace)
	// Assert no error occurred
	assert.NoError(t, err)

	// Assert that the transport secret was deleted
	secret := &corev1.Secret{}
	err = fakeClient.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: transportSecretName}, secret)
	assert.True(t, errors.IsNotFound(err))
}
