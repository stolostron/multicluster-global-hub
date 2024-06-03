package config

import (
	"context"
	"encoding/base64"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	transporterProtocol transport.TransportProtocol
	transporter         transport.Transporter
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
	transporter = p
}

func GetTransporter() transport.Transporter {
	return transporter
}

func GetKafkaResourceReady() bool {
	return kafkaResourceReady
}

func SetKafkaResourceReady(ready bool) {
	kafkaResourceReady = ready
}

func GetKafkaStorageSize(mgh *globalhubv1alpha4.MulticlusterGlobalHub) string {
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

func GetConnFromGHTransportSecret(ctx context.Context, c client.Client) (*transport.ConnCredential, error) {
	kafkaSecret := &corev1.Secret{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      constants.GHTransportSecretName,
		Namespace: utils.GetDefaultNamespace(),
	}, kafkaSecret)
	if err != nil {
		return nil, err
	}
	return &transport.ConnCredential{
		Identity:        string(kafkaSecret.Data[filepath.Join("bootstrap_server")]),
		BootstrapServer: string(kafkaSecret.Data[filepath.Join("bootstrap_server")]),
		CACert:          base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("ca.crt")]),
		ClientCert:      base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("client.crt")]),
		ClientKey:       base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("client.key")]),
	}, nil
}
