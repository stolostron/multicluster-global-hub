package controller

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

type TransportCallback func(producer transport.Producer, consumer transport.Consumer) error

type TransportCtrl struct {
	secretNamespace string
	secretName      string
	kafkaConfig     *transport.KafkaConfig
	// the use the producer and consumer to activate the call back funciton, once it executed successful, then clear it.
	callback TransportCallback

	runtimeClient client.Client
	consumer      transport.Consumer
	producer      transport.Producer
}

func NewTransportCtrl(secretNamespace, secretName string, kafkaConfig *transport.KafkaConfig,
	callback TransportCallback,
) *TransportCtrl {
	return &TransportCtrl{
		secretNamespace: secretNamespace,
		secretName:      secretName,
		kafkaConfig:     kafkaConfig,
		callback:        callback,
	}
}

func (c *TransportCtrl) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: c.secretNamespace,
			Name:      c.secretName,
		},
	}
	if err := c.runtimeClient.Get(ctx, client.ObjectKeyFromObject(secret), secret); err != nil {
		return ctrl.Result{}, err
	}

	// it the client certs generated on the managed hub clusters
	var clientSecret *corev1.Secret
	clientSecretName := string(secret.Data["client_secret"])
	if clientSecretName != "" {
		clientSecretName = utils.AgentCertificateSecretName()
	}
	clientSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: c.secretNamespace,
			Name:      clientSecretName,
		},
	}
	err := c.runtimeClient.Get(ctx, client.ObjectKeyFromObject(clientSecret), clientSecret)
	if err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if !c.updateKafkaConfig(secret, clientSecret) {
		return ctrl.Result{}, nil
	}

	// create the new consumer/producer and close the previous consumer/producer
	sendTopic := c.kafkaConfig.Topics.SpecTopic
	receiveTopic := c.kafkaConfig.Topics.StatusTopic
	databaseOffset := true
	if c.secretName != constants.GHManagerTransportSecret {
		sendTopic = c.kafkaConfig.Topics.StatusTopic
		receiveTopic = c.kafkaConfig.Topics.SpecTopic
		databaseOffset = false
	}
	transportConfig := &transport.TransportConfig{
		KafkaConfig:   c.kafkaConfig,
		TransportType: string(transport.Kafka),
	}

	if c.producer != nil {
		if err := c.producer.Reconnect(transportConfig); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to reconnect the producer: %w", err)
		}
	} else {
		sender, err := producer.NewGenericProducer(transportConfig, sendTopic)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create/update the producer: %w", err)
		}
		c.producer = sender
	}

	if c.consumer != nil {
		if err := c.consumer.Reconnect(ctx, transportConfig, []string{receiveTopic}); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to reconnect the consumer: %w", err)
		}
	} else {
		receiver, err := consumer.NewGenericConsumer(transportConfig, []string{receiveTopic},
			consumer.EnableDatabaseOffset(databaseOffset))
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create the consumer: %w", err)
		}
		c.consumer = receiver
		go func() {
			if err = c.consumer.Start(ctx); err != nil {
				klog.Errorf("failed to start the consumser: %v", err)
			}
		}()
	}
	klog.Info("the transport secret(producer, consumer) is created/updated")

	if c.callback == nil {
		if err := c.callback(c.producer, c.consumer); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to invoke the callback function: %w", err)
		}
		c.callback = nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (c *TransportCtrl) SetupWithManager(ctx context.Context, mgr ctrl.Manager, secretNames []string) error {
	c.runtimeClient = mgr.GetClient()
	cond := func(obj client.Object) bool {
		for _, name := range secretNames {
			if obj.GetName() == name {
				return true
			}
		}
		return false
	}
	secretPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return cond(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if !cond(e.ObjectNew) {
				return false
			}
			newSecret := e.ObjectNew.(*corev1.Secret)
			oldSecret := e.ObjectOld.(*corev1.Secret)
			// only enqueue the obj when secret data changed
			return !reflect.DeepEqual(newSecret.Data, oldSecret.Data)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}, builder.WithPredicates(secretPred)).
		Complete(c)
}

// updateKafkaConfig update the current config by secret, and validate the configuration
func (c *TransportCtrl) updateKafkaConfig(secret, clientSecret *corev1.Secret) bool {
	update := false
	expectedConn := &transport.ConnCredential{
		Identity:        string(secret.Data["id"]),
		BootstrapServer: string(secret.Data["bootstrap_server"]),
		CACert:          string(secret.Data["ca.crt"]),
		ClientCert:      string(secret.Data["client.crt"]),
		ClientKey:       string(secret.Data["client.key"]),
	}
	// type := string(secret.Data["type"])
	if clientSecret != nil && clientSecret.Data != nil {
		expectedConn.ClientCert = string(clientSecret.Data["tls.crt"])
		expectedConn.ClientKey = string(clientSecret.Data["tls.key"])
	}
	expectedTopics := &transport.ClusterTopic{
		SpecTopic:   string(secret.Data["spec_topic"]),
		StatusTopic: string(secret.Data["status_topic"]),
	}
	if !reflect.DeepEqual(expectedConn, c.kafkaConfig.ConnCredential) {
		c.kafkaConfig.ConnCredential = expectedConn
		update = true
	}
	if !reflect.DeepEqual(expectedTopics, c.kafkaConfig.Topics) {
		c.kafkaConfig.Topics = expectedTopics
		update = true
	}
	return update
}

func LoadDataToSecret(secret *corev1.Secret, conn *transport.ConnCredential, topics *transport.ClusterTopic,
	clientSecret string,
) {
	secret.Data = map[string][]byte{
		"id":               []byte(conn.Identity),
		"bootstrap_server": []byte(conn.BootstrapServer),
		"ca.crt":           []byte(conn.CACert),
		"client.crt":       []byte(conn.ClientCert),
		"client.key":       []byte(conn.ClientKey),
		"type":             []byte(transport.Kafka),
		"status_topic":     []byte(topics.StatusTopic),
		"spec_topic":       []byte(topics.SpecTopic),
		"client_secret":    []byte(clientSecret), // first get the client cert/key from this secret
	}
}
