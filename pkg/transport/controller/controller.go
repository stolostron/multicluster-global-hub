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
	conn, err := config.GetTransportCredentailBySecret(secret, c.runtimeClient)
	if err != nil {
		return ctrl.Result{}, err
	}

	// update the credential secret colletions for the predicates
	if conn.CASecretName != "" || !utils.ContainsString(c.extraSecretNames, conn.CASecretName) {
		c.extraSecretNames = append(c.extraSecretNames, conn.CASecretName)
	}
	if conn.ClientSecretName != "" || utils.ContainsString(c.extraSecretNames, conn.ClientSecretName) {
		c.extraSecretNames = append(c.extraSecretNames, conn.ClientSecretName)
	}

	// if credential not update, then return
	if reflect.DeepEqual(c.transportConfig.KafkaCredential, conn) {
		return ctrl.Result{}, nil
	}
	c.transportConfig.KafkaCredential = conn

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
	return true
}
