package config

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	DEFAULT_SPEC_TOPIC          = "gh-spec"
	DEFAULT_STATUS_TOPIC        = "gh-status.*"
	DEFAULT_SHARED_STATUS_TOPIC = "gh-status"
)

var (
	transporterProtocol   transport.TransportProtocol
	transporterInstance   transport.Transporter
	transporterConn       *transport.KafkaConfig
	enableInventory       = false
	isBYOKafka            = false
	specTopic             = ""
	statusTopic           = ""
	migrationTopic        = ""
	kafkaResourceReady    = false
	acmResourceReady      = false
	kafkaClientCAKey      []byte
	kafkaClientCACert     []byte
	inventoryClientCAKey  []byte
	inventoryClientCACert []byte
	inventoryConn         *transport.RestfulConfig
)

func SetTransporterConn(conn *transport.KafkaConfig) bool {
	log.Debug("set Transporter Conn")
	if conn == nil {
		transporterConn = nil
		return true
	}
	if !reflect.DeepEqual(conn, transporterConn) {
		transporterConn = conn
		log.Debug("update Transporter Conn")
		return true
	}
	return false
}

func GetTransporterConn() *transport.KafkaConfig {
	log.Debugf("Get Transporter Conn: %v", transporterConn != nil)
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
	log.Debugf("acmResourceReady: %v", acmResourceReady)
	return acmResourceReady
}

func SetACMResourceReady(ready bool) {
	acmResourceReady = ready
}

func GetKafkaStorageSize(mgh *v1alpha4.MulticlusterGlobalHub) string {
	defaultKafkaStorageSize := "10Gi"
	if mgh.Spec.DataLayerSpec.Kafka.StorageSize != "" {
		return mgh.Spec.DataLayerSpec.Kafka.StorageSize
	}
	return defaultKafkaStorageSize
}

// SetTransportConfig sets the kafka type, protocol and topics
func SetTransportConfig(ctx context.Context, runtimeClient client.Client, mgh *v1alpha4.MulticlusterGlobalHub) error {
	// set the transport type
	if err := SetKafkaType(ctx, runtimeClient, mgh.Namespace); err != nil {
		return err
	}

	// set the inventory
	enableInventory = WithInventory(mgh)

	// set the topic
	specTopic = mgh.Spec.DataLayerSpec.Kafka.KafkaTopics.SpecTopic
	statusTopic = mgh.Spec.DataLayerSpec.Kafka.KafkaTopics.StatusTopic
	migrationTopic = mgh.Spec.DataLayerSpec.Kafka.KafkaTopics.MigrationTopic
	if !isValidKafkaTopicName(specTopic) {
		return fmt.Errorf("the specTopic is invalid: %s", specTopic)
	}
	if !isValidKafkaTopicName(statusTopic) {
		return fmt.Errorf("the specTopic is invalid: %s", statusTopic)
	}
	if !isValidKafkaTopicName(migrationTopic) {
		return fmt.Errorf("the migrationTopic is invalid: %s", migrationTopic)
	}

	// BYO Case:
	// 1. change the default status topic from 'gh-status.*' to 'gh-status'
	// 2. ensure the status topic must not contain '*'
	if isBYOKafka {
		if statusTopic == DEFAULT_STATUS_TOPIC {
			mgh.Spec.DataLayerSpec.Kafka.KafkaTopics.StatusTopic = DEFAULT_SHARED_STATUS_TOPIC
			statusTopic = DEFAULT_SHARED_STATUS_TOPIC

			if err := runtimeClient.Update(ctx, mgh); err != nil {
				return fmt.Errorf("failed to update the topic from %s to %s, err:%v",
					DEFAULT_STATUS_TOPIC, DEFAULT_SHARED_STATUS_TOPIC, err)
			}
		}

		if strings.Contains(statusTopic, "*") {
			return fmt.Errorf("status topic(%s) must not contain '*'", statusTopic)
		}
	}
	return nil
}

// isValidKafkaTopicName validates the Kafka topic name based on common rules.
// ref: https://github.com/apache/kafka/blob/3.9/clients/src/main/java/org/apache/kafka/common/internals/Topic.java
func isValidKafkaTopicName(name string) bool {
	// Kafka topic name must be between 1 and 255 characters.
	if len(name) < 1 || len(name) >= 255 {
		return false
	}

	if name == "." || name == ".." {
		return false
	}

	// Kafka topic names can contain letters, numbers, dots (.), underscores (_), and dashes (-).
	// The rule is only for the GlobalHub: An asterisk (*) can be appended to the suffix.
	re := regexp.MustCompile(`^[a-zA-Z0-9._-]+(\*)?$`)

	return re.MatchString(name)
}

func GetSpecTopic() string {
	return specTopic
}

func GetMigrationTopic() string {
	return migrationTopic
}

// GetStatusTopic return the status topic with clusterName, like 'gh-status.<clusterName>'
func GetStatusTopic(clusterName string) string {
	return strings.Replace(statusTopic, "*", clusterName, -1)
}

// GetRawStatusTopic return the validated statusTopic from mgh CR
func GetRawStatusTopic() string {
	return statusTopic
}

// ManagerStatusTopic return the regex topic with fuzzy matching, like '^gh-status.*'
func ManagerStatusTopic() string {
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

func SetBYOKafka(byoKafka bool) {
	isBYOKafka = byoKafka
}

func IsBYOKafka() bool {
	return isBYOKafka
}

func TransporterProtocol() transport.TransportProtocol {
	return transporterProtocol
}

// GetKafkaClientCA the raw([]byte) of client ca key and ca cert
func GetKafkaClientCA() ([]byte, []byte) {
	return kafkaClientCAKey, kafkaClientCACert
}

func GetInventoryClientCA() ([]byte, []byte) {
	return inventoryClientCAKey, inventoryClientCACert
}

func SetInventoryClientCA(ctx context.Context, namespace, name string, c client.Client) error {
	clientCASecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	err := c.Get(ctx, client.ObjectKeyFromObject(clientCASecret), clientCASecret)
	if err != nil {
		return err
	}

	if inventoryClientCAKey == nil || !bytes.Equal(clientCASecret.Data["tls.key"], inventoryClientCAKey) {
		log.Infof("set the inventory clientCA - key: %s", clientCASecret.Name)
		inventoryClientCAKey = clientCASecret.Data["tls.key"]
	}
	if inventoryClientCACert == nil || !bytes.Equal(clientCASecret.Data["tls.crt"], inventoryClientCACert) {
		log.Infof("set the inventory clientCA - cert: %s", clientCASecret.Name)
		inventoryClientCACert = clientCASecret.Data["tls.crt"]
	}
	return nil
}

func SetKafkaClientCA(ctx context.Context, namespace, name string, c client.Client) error {
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
	if kafkaClientCAKey == nil || !bytes.Equal(clientCAKeySecret.Data["ca.key"], kafkaClientCAKey) {
		log.Infof("set the ca - client key: %s", clientCAKeySecret.Name)
		kafkaClientCAKey = clientCAKeySecret.Data["ca.key"]
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

	if kafkaClientCACert == nil || !bytes.Equal(clientCACertSecret.Data["ca.crt"], kafkaClientCACert) {
		log.Infof("set the ca - client cert: %s", clientCACertSecret.Name)
		kafkaClientCACert = clientCACertSecret.Data["ca.crt"]
	}
	return nil
}

func EnableInventory() bool {
	return enableInventory
}

// GetTransportConfigClientName gives the client name based on the cluster name, it could be kafkauser or inventory name
func GetTransportConfigClientName(clusterName string) string {
	if TransporterProtocol() == transport.StrimziTransporter {
		return GetKafkaUserName(clusterName)
	}
	return ""
}

// GetKafkaUserName gives a kafkaUser name based on the cluster name, it's also the CN of the certificate
func GetKafkaUserName(clusterName string) string {
	return fmt.Sprintf("%s-kafka-user", clusterName)
}
