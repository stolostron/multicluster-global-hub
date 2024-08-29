package controller

import (
	"context"
	"encoding/base64"
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
	"sigs.k8s.io/kustomize/kyaml/yaml"

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

	// load the kafka connection credentail based on the transport type. kafka, multiple
	kafkaConn, err := config.GetTransportCredentailBySecret(secret, c.runtimeClient)
	if err != nil {
		return ctrl.Result{}, err
	}

	// update the kafka credential secret colletions for the predicates
	if kafkaConn.CASecretName != "" || !utils.ContainsString(c.extraSecretNames, kafkaConn.CASecretName) {
		c.extraSecretNames = append(c.extraSecretNames, kafkaConn.CASecretName)
	}
	if kafkaConn.ClientSecretName != "" || utils.ContainsString(c.extraSecretNames, kafkaConn.ClientSecretName) {
		c.extraSecretNames = append(c.extraSecretNames, kafkaConn.ClientSecretName)
	}

	inventoryConn, err := c.GetInventoryConnBySecret(secret)
	if err != nil {
		return ctrl.Result{}, err
	}

	// if credentials aren't updated, then return
	if reflect.DeepEqual(c.transportConfig.KafkaCredential, kafkaConn) &&
		reflect.DeepEqual(c.transportConfig.InventoryCredentail, inventoryConn) {
		return ctrl.Result{}, nil
	}
	c.transportConfig.KafkaCredential = kafkaConn
	c.transportConfig.InventoryCredentail = inventoryConn

	// transport config is changed, then create/update the consumer/producer
	if c.producer == nil {
		sender, err := producer.NewGenericProducer(c.transportConfig)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create/update the producer: %w", err)
		}
		c.producer = sender
	} else {
		if err := c.producer.Reconnect(c.transportConfig); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to reconnect the producer: %w", err)
		}
	}

	// don't create the consumer when the consumer groupId is empty, that means the agent maybe in the standalone mode
	if c.transportConfig.ConsumerGroupId != "" {
		if c.consumer == nil {
			receiver, err := consumer.NewGenericConsumer(c.transportConfig)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to create the consumer: %w", err)
			}
			c.consumer = receiver
			go func() {
				if err = c.consumer.Start(ctx); err != nil {
					klog.Errorf("failed to start the consumser: %v", err)
				}
			}()
		} else {
			if err := c.consumer.Reconnect(ctx, c.transportConfig); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reconnect the consumer: %w", err)
			}
		}
	}

	klog.Info("the transport secret(producer, consumer) is created/updated")

	if c.callback != nil {
		if err := c.callback(c.producer, c.consumer); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to invoke the callback function: %w", err)
		}
	}

	return ctrl.Result{}, nil
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

func (c *TransportCtrl) GetInventoryConnBySecret(transportConfig *corev1.Secret) (
	*transport.InventoryConnCredentail, error,
) {
	inventoryYaml, ok := transportConfig.Data["inventory.yaml"]
	if !ok {
		return nil, nil
	}
	inventoryConn := &transport.InventoryConnCredentail{}
	if err := yaml.Unmarshal(inventoryYaml, inventoryConn); err != nil {
		return nil, fmt.Errorf("failed to unmarshal kafka config to transport credentail: %w", err)
	}

	// decode the ca and client cert
	if inventoryConn.CACert != "" {
		bytes, err := base64.StdEncoding.DecodeString(inventoryConn.CACert)
		if err != nil {
			return nil, err
		}
		inventoryConn.CACert = string(bytes)
	}
	if inventoryConn.ClientCert != "" {
		bytes, err := base64.StdEncoding.DecodeString(inventoryConn.ClientCert)
		if err != nil {
			return nil, err
		}
		inventoryConn.ClientCert = string(bytes)
	}
	if inventoryConn.ClientKey != "" {
		bytes, err := base64.StdEncoding.DecodeString(inventoryConn.ClientKey)
		if err != nil {
			return nil, err
		}
		inventoryConn.ClientKey = string(bytes)
	}
	return inventoryConn, nil
}
