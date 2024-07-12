package config

import (
	"bytes"
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
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
	clientCAKey         []byte
	clientCACert        []byte
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

// GetClientCA the raw([]byte) of client ca key and ca cert
func GetClientCA() ([]byte, []byte) {
	return clientCAKey, clientCACert
}

func SetClientCA(namespace, name string, c client.Client) error {
	clientCAKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-clients-ca", name),
			Namespace: namespace,
		},
	}
	err := c.Get(context.TODO(), client.ObjectKeyFromObject(clientCAKeySecret), clientCAKeySecret)
	if err != nil {
		return err
	}
	if clientCAKey == nil || !bytes.Equal(clientCAKeySecret.Data["ca.key"], clientCAKey) {
		klog.Infof("set the ca - client key: %s", clientCAKeySecret.Name)
		clientCAKey = clientCAKeySecret.Data["ca.key"]
	}

	clientCACertSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-clients-ca-cert", name),
			Namespace: namespace,
		},
	}
	err = c.Get(context.TODO(), client.ObjectKeyFromObject(clientCACertSecret), clientCACertSecret)
	if err != nil {
		return err
	}

	if clientCACert == nil || !bytes.Equal(clientCACertSecret.Data["ca.crt"], clientCACert) {
		klog.Infof("set the ca - client cert: %s", clientCACertSecret.Name)
		clientCACert = clientCACertSecret.Data["ca.crt"]
	}

	return nil
}

// GetKafkaUserName gives a kafkaUser name based on the cluster name, it's also the CN of the certificate
func GetKafkaUserName(clusterName string) string {
	return fmt.Sprintf("%s-kafka-user", clusterName)
}
