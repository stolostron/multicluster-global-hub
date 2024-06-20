package config

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var (
	transporterProtocol transport.TransportProtocol
	transporterInstance transport.Transporter
	transporterConn     *transport.ConnCredential
	isBYOKafka          = false
	kafkaResourceReady  = false
)

func SetTransporterConn(conn *transport.ConnCredential) {
	transporterConn = conn
}

func GetTransporterConn() *transport.ConnCredential {
	return transporterConn
}

func SetTransporter(p transport.Transporter) {
	transporterInstance = p
}

func GetTransporter() transport.Transporter {
	return transporterInstance
}

func GetKafkaResourceReady() bool {
	return kafkaResourceReady
}

func SetKafkaResourceReady(ready bool) {
	kafkaResourceReady = ready
}

func GetKafkaStorageSize(mgh *v1alpha4.MulticlusterGlobalHub) string {
	defaultKafkaStorageSize := "10Gi"
	if mgh.Spec.DataLayer.Kafka.StorageSize != "" {
		return mgh.Spec.DataLayer.Kafka.StorageSize
	}
	return defaultKafkaStorageSize
}

func SetBYOKafka(ctx context.Context, runtimeClient client.Client, namespace string) error {
	kafkaSecret := &corev1.Secret{}
	err := runtimeClient.Get(ctx, types.NamespacedName{
		Name:      constants.GHTransportSecretName,
		Namespace: namespace,
	}, kafkaSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			transporterProtocol = transport.StrimziTransporter
			isBYOKafka = false
			return nil
		}
		return err
	}
	transporterProtocol = transport.SecretTransporter
	isBYOKafka = true
	return nil
}

func IsBYOKafka() bool {
	return isBYOKafka
}

func TransporterProtocol() transport.TransportProtocol {
	return transporterProtocol
}
