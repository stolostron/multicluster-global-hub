package config

import (
	"bytes"
	"context"
	"fmt"
	"strings"

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

const (
	DEFAULT_SPEC_TOPIC          = "gh-spec"
	DEFAULT_STATUS_TOPIC        = "gh-event.*"
	DEFAULT_SHARED_STATUS_TOPIC = "gh-event"
)

var (
	transporterProtocol transport.TransportProtocol
	transporterInstance transport.Transporter
	transporterConn     *transport.ConnCredential
	isBYOKafka          = false
	specTopic           = ""
	statusTopic         = ""
	kafkaResourceReady  = false
	acmResourceReady    = false
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

func IsACMResourceReady() bool {
	return acmResourceReady
}

func SetACMResourceReady(ready bool) {
	acmResourceReady = ready
}

func GetKafkaStorageSize(mgh *v1alpha4.MulticlusterGlobalHub) string {
	defaultKafkaStorageSize := "10Gi"
	if mgh.Spec.DataLayer.Kafka.StorageSize != "" {
		return mgh.Spec.DataLayer.Kafka.StorageSize
	}
	return defaultKafkaStorageSize
}

// SetTransportConfig sets the kafka type, protocol and topics
func SetTransportConfig(ctx context.Context, runtimeClient client.Client, mgh *v1alpha4.MulticlusterGlobalHub) error {
	if err := SetKafkaType(ctx, runtimeClient, mgh.Namespace); err != nil {
		return err
	}

	// set the topic
	specTopic = mgh.Spec.DataLayer.Kafka.KafkaTopics.SpecTopic
	statusTopic = mgh.Spec.DataLayer.Kafka.KafkaTopics.StatusTopic

	if specTopic == "" || statusTopic == "" {
		return fmt.Errorf("specTopic (%s) and statusTopic (%s) must not be empty", specTopic, statusTopic)
	}

	// BYO Case:
	// 1. change the default status topic from 'gh-event.*' to 'gh-event'
	// 2. ensure the status topic must not contain '*'
	if isBYOKafka {
		if statusTopic == DEFAULT_STATUS_TOPIC {
			mgh.Spec.DataLayer.Kafka.KafkaTopics.StatusTopic = DEFAULT_SHARED_STATUS_TOPIC
			statusTopic = DEFAULT_SHARED_STATUS_TOPIC

			if err := runtimeClient.Update(ctx, mgh); err != nil {
				return fmt.Errorf("failed to update the topic from %s to %s", DEFAULT_STATUS_TOPIC, DEFAULT_SHARED_STATUS_TOPIC)
			}
		}

		if strings.Contains(statusTopic, "*") {
			return fmt.Errorf("status topic(%s) must not contain '*'", statusTopic)
		}
	}

	// kafka use the prefix topic for the authz
	if strings.Contains(statusTopic, "*") && !strings.HasSuffix(statusTopic, "*") {
		return fmt.Errorf("the status topic (%s) contains '*', it must be at the end", statusTopic)
	}
	return nil
}

func GetSpecTopic() string {
	return specTopic
}

// GetStatusTopic return the status topic with clusterName, like 'gh-event.<clusterName>'
func GetStatusTopic(clusterName string) string {
	return strings.Replace(statusTopic, "*", clusterName, -1)
}

// FuzzyStatusTopic return the regex topic with fuzzy matching, like '^gh-event.*'
func FuzzyStatusTopic() string {
	if strings.Contains(statusTopic, "*") {
		return fmt.Sprintf("^%s", statusTopic)
	}
	return statusTopic
}

// SetKafkaType will assert whether it's a BYO case and also set the related transport protocol
func SetKafkaType(ctx context.Context, runtimeClient client.Client, namespace string) error {
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

func SetClientCA(ctx context.Context, namespace, name string, c client.Client) error {
	clientCAKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-clients-ca", name),
			Namespace: namespace,
		},
	}
	err := c.Get(ctx, client.ObjectKeyFromObject(clientCAKeySecret), clientCAKeySecret)
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
	err = c.Get(ctx, client.ObjectKeyFromObject(clientCACertSecret), clientCACertSecret)
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
