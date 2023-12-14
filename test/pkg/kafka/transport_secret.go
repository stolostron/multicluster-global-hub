package kafka

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	KafkaBootstrapServer = "https://test-kafka.example.com"
	KafkaCA              = "foobar"
	KafkaClientCert      = ""
	KafkaClientKey       = ""
)

func CreateTestTransportSecret(c client.Client, namespace string) error {
	// if namespace not exist, then create the namespace
	err := c.Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	err = c.Get(context.TODO(), types.NamespacedName{
		Namespace: namespace,
		Name:      constants.GHTransportSecretName,
	}, &corev1.Secret{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	} else if errors.IsNotFound(err) {
		return c.Create(context.TODO(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.GHTransportSecretName,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"bootstrap_server": []byte(KafkaBootstrapServer),
				"ca.crt":           []byte(KafkaCA),
				"client.crt":       []byte(KafkaClientCert),
				"client.key":       []byte(KafkaClientKey),
			},
		})
	}
	return nil
}
