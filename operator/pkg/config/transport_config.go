package config

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
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
	transporterProtocol transport.TransportProtocol
	transporterInstance transport.Transporter
	transporterConn     *transport.KafkaConnCredential
	isBYOKafka          = false
	specTopic           = ""
	statusTopic         = ""
	kafkaResourceReady  = false
	acmResourceReady    = false
	clientCAKey         []byte
	clientCACert        []byte
	transportCond       = &metav1.Condition{
		Type:   CONDITION_TRANSPORT_TYPE,
		Status: metav1.ConditionTrue,
	}
)

func SetTransporterConn(conn *transport.KafkaConnCredential) {
	transporterConn = conn
}

func GetTransporterConn() *transport.KafkaConnCredential {
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

func UpdateTransportCondition(reason string, err error) {
	if err != nil {
		if transportCond.Status == metav1.ConditionTrue {
			transportCond.Reason = reason
			transportCond.Status = metav1.ConditionFalse
			transportCond.Message = err.Error()
		}
	} else {
		if transportCond.Status == metav1.ConditionFalse && transportCond.Reason == reason {
			transportCond.Status = metav1.ConditionTrue
			transportCond.Message = fmt.Sprintf("The topics is parsed: spec(%s), status(%s)", GetSpecTopic(),
				ManagerStatusTopic())
		}
	}
}

func GetTransportCondition() *metav1.Condition {
	return transportCond
}

// SetTransportConfig sets the kafka type, protocol and topics
func SetTransportConfig(ctx context.Context, runtimeClient client.Client, mgh *v1alpha4.MulticlusterGlobalHub) error {
	var err error

	defer func() {
		UpdateTransportCondition(CONDITION_TRANSPORT_REASON_TOPIC, err)
	}()

	if err = SetKafkaType(ctx, runtimeClient, mgh.Namespace); err != nil {
		return err
	}

	// set the topic
	specTopic = mgh.Spec.DataLayer.Kafka.KafkaTopics.SpecTopic
	statusTopic = mgh.Spec.DataLayer.Kafka.KafkaTopics.StatusTopic

	if !isValidKafkaTopicName(specTopic) {
		return fmt.Errorf("the specTopic is invalid: %s", specTopic)
	}
	if !isValidKafkaTopicName(statusTopic) {
		return fmt.Errorf("the statusTopic is invalid: %s", statusTopic)
	}

	// BYO Case:
	// 1. change the default status topic from 'gh-status.*' to 'gh-status'
	// 2. ensure the status topic must not contain '*'
	if isBYOKafka {
		if statusTopic == DEFAULT_STATUS_TOPIC {
			mgh.Spec.DataLayer.Kafka.KafkaTopics.StatusTopic = DEFAULT_SHARED_STATUS_TOPIC
			statusTopic = DEFAULT_SHARED_STATUS_TOPIC

			if err = runtimeClient.Update(ctx, mgh); err != nil {
				return fmt.Errorf("failed to update the topic from %s to %s", DEFAULT_STATUS_TOPIC, DEFAULT_SHARED_STATUS_TOPIC)
			}
		}

		if strings.Contains(statusTopic, "*") {
			err = fmt.Errorf("status topic(%s) must not contain '*'", statusTopic)
			return err
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
