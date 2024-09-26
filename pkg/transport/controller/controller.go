package controller

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

type TransportCallback func(producer transport.Producer, consumer transport.Consumer) error

type TransportCtrl struct {
	secretNamespace string
	secretName      string
	transportConfig *transport.TransportConfig

	// the use the producer and consumer to activate the call back funciton, once it executed successful, then clear it.
	callback TransportCallback

	runtimeClient    client.Client
	consumer         transport.Consumer
	producer         transport.Producer
	extraSecretNames []string

	mutex sync.Mutex
}

func NewTransportCtrl(namespace, name string, callback TransportCallback,
	transportConfig *transport.TransportConfig,
) *TransportCtrl {
	return &TransportCtrl{
		secretNamespace:  namespace,
		secretName:       name,
		callback:         callback,
		transportConfig:  transportConfig,
		extraSecretNames: make([]string, 2),
	}
}

func (c *TransportCtrl) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: c.secretNamespace,
			Name:      c.secretName,
		},
	}
	if err := c.runtimeClient.Get(ctx, client.ObjectKeyFromObject(secret), secret); err != nil {
		return ctrl.Result{}, err
	}

	_, isKafka := secret.Data["kafka.yaml"]
	if isKafka {
		c.transportConfig.TransportType = string(transport.Kafka)
	}

	_, isRestful := secret.Data["rest.yaml"]
	if isRestful {
		c.transportConfig.TransportType = string(transport.Rest)
	}

	var updated bool
	var err error
	switch c.transportConfig.TransportType {
	case string(transport.Kafka):
		updated, err = c.ReconcileKafkaCredential(ctx, secret)
		if err != nil {
			return ctrl.Result{}, err
		}
	case string(transport.Rest):
		updated, err = c.ReconcileRestfulCredential(ctx, secret)
		if err != nil {
			return ctrl.Result{}, err
		}
	default:
		return ctrl.Result{}, fmt.Errorf("unsupported transport type: %s", c.transportConfig.TransportType)
	}

	if !updated {
		return ctrl.Result{}, nil
	}

	// secret is changed, then create/update the producer
	err = c.ReconcileProducer()
	if err != nil {
		return ctrl.Result{}, err
	}
	klog.Info("the transport producer is created/updated")

	if c.producer != nil && c.callback != nil {
		if err := c.callback(c.producer, c.consumer); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to invoke the callback function: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

// ReconcileProducer, transport config is changed, then create/update the producer
func (c *TransportCtrl) ReconcileProducer() error {
	if c.producer == nil {
		sender, err := producer.NewGenericProducer(c.transportConfig)
		if err != nil {
			return fmt.Errorf("failed to create/update the producer: %w", err)
		}
		c.producer = sender
	} else {
		if err := c.producer.Reconnect(c.transportConfig); err != nil {
			return fmt.Errorf("failed to reconnect the producer: %w", err)
		}
	}
	return nil
}

// ReconcileKafkaCredential update the kafka connection credentail based on the secret, return true if the kafka
// credentail is updated, It also create/update the consumer if not in the standalone mode
func (c *TransportCtrl) ReconcileKafkaCredential(ctx context.Context, secret *corev1.Secret) (updated bool, err error) {
	// load the kafka connection credentail based on the transport type. kafka, multiple
	kafkaConn, err := config.GetKafkaCredentailBySecret(secret, c.runtimeClient)
	if err != nil {
		return updated, err
	}

	// update the wathing secret lits
	if kafkaConn.CASecretName != "" || !utils.ContainsString(c.extraSecretNames, kafkaConn.CASecretName) {
		c.extraSecretNames = append(c.extraSecretNames, kafkaConn.CASecretName)
	}
	if kafkaConn.ClientSecretName != "" || utils.ContainsString(c.extraSecretNames, kafkaConn.ClientSecretName) {
		c.extraSecretNames = append(c.extraSecretNames, kafkaConn.ClientSecretName)
	}

	// if credentials aren't updated, then return
	if reflect.DeepEqual(c.transportConfig.KafkaCredential, kafkaConn) {
		return
	}
	c.transportConfig.KafkaCredential = kafkaConn
	updated = true

	// if the consumer groupId is empty, then it's means the agent is in the standalone mode, don't create the consumer
	if c.transportConfig.ConsumerGroupId == "" {
		return
	}

	// create/update the consumer with the kafka transport
	if c.consumer == nil {
		receiver, err := consumer.NewGenericConsumer(c.transportConfig)
		if err != nil {
			return updated, fmt.Errorf("failed to create the consumer: %w", err)
		}
		c.consumer = receiver
		go func() {
			if err = c.consumer.Start(ctx); err != nil {
				klog.Errorf("failed to start the consumser: %v", err)
			}
		}()
	} else {
		if err := c.consumer.Reconnect(ctx, c.transportConfig); err != nil {
			return updated, fmt.Errorf("failed to reconnect the consumer: %w", err)
		}
	}
	klog.Info("the transport(kafka) onsumer is created/updated")

	return updated, nil
}

func (c *TransportCtrl) ReconcileRestfulCredential(ctx context.Context, secret *corev1.Secret) (
	updated bool, err error,
) {
	restfulConn, err := config.GetRestfulConnBySecret(secret, c.runtimeClient)
	if err != nil {
		return updated, err
	}

	// update the wathing secret lits
	if restfulConn.CASecretName != "" || !utils.ContainsString(c.extraSecretNames, restfulConn.CASecretName) {
		c.extraSecretNames = append(c.extraSecretNames, restfulConn.CASecretName)
	}
	if restfulConn.ClientSecretName != "" || utils.ContainsString(c.extraSecretNames, restfulConn.ClientSecretName) {
		c.extraSecretNames = append(c.extraSecretNames, restfulConn.ClientSecretName)
	}

	if reflect.DeepEqual(c.transportConfig.RestfulCredential, restfulConn) {
		return
	}
	updated = true
	c.transportConfig.RestfulCredential = restfulConn
	return
}

// SetupWithManager sets up the controller with the Manager.
func (c *TransportCtrl) SetupWithManager(mgr ctrl.Manager) error {
	c.runtimeClient = mgr.GetClient()
	secretPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return c.credentialSecret(e.Object.GetName())
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if !c.credentialSecret(e.ObjectNew.GetName()) {
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

func (c *TransportCtrl) credentialSecret(name string) bool {
	if c.secretName == name {
		return true
	}
	if c.extraSecretNames == nil || len(c.extraSecretNames) == 0 {
		return false
	}
	for _, secretName := range c.extraSecretNames {
		if name == secretName {
			return true
		}
	}
	return false
}
