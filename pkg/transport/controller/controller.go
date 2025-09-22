package controller

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/requester"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var log = logger.DefaultZapLogger()

type TransportCallback func(transportClient transport.TransportClient) error

type TransportCtrl struct {
	runtimeClient    client.Client
	secretNamespace  string
	secretName       string
	extraSecretNames []string
	workqueue        workqueue.TypedRateLimitingInterface[ctrl.Request]
	transportConfig  *transport.TransportInternalConfig

	// the use the producer and consumer to activate the callback, once it executed successful, then clear it.
	transportCallback TransportCallback
	transportClient   *TransportClient

	// producerTopic is current topic which is used to create a producer
	producerTopic string
	// consumerTopics is current topics which are used to create a consumer
	consumerTopics []string
	mutex          sync.Mutex
	// inManager is used to check if the controller is in the manager.
	// if it's true, then the controller is in the manager, otherwise it's in the agent.
	inManager       bool
	disableConsumer bool
}

type TransportClient struct {
	consumer  transport.Consumer
	producer  transport.Producer
	requester transport.Requester
}

func (c *TransportClient) GetProducer() transport.Producer {
	return c.producer
}

func (c *TransportClient) GetConsumer() transport.Consumer {
	return c.consumer
}

func (c *TransportClient) GetRequester() transport.Requester {
	return c.requester
}

func (c *TransportClient) SetProducer(producer transport.Producer) {
	c.producer = producer
}

func (c *TransportClient) SetConsumer(consumer transport.Consumer) {
	c.consumer = consumer
}

func (c *TransportClient) SetRequester(requester transport.Requester) {
	c.requester = requester
}

func NewTransportCtrl(namespace, name string, callback TransportCallback,
	transportConfig *transport.TransportInternalConfig, inManager bool,
) *TransportCtrl {
	return &TransportCtrl{
		secretNamespace:   namespace,
		secretName:        name,
		transportCallback: callback,
		transportClient:   &TransportClient{},
		transportConfig:   transportConfig,
		extraSecretNames:  make([]string, 2),
		inManager:         inManager,
		disableConsumer:   false,
	}
}

func (c *TransportCtrl) DisableConsumer() {
	c.disableConsumer = true
}

func (c *TransportCtrl) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	log.Infof("reconcile transport(producer/consumer): %v", request.NamespacedName)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: c.secretNamespace,
			Name:      c.secretName,
		},
	}
	if err := c.runtimeClient.Get(ctx, client.ObjectKeyFromObject(secret), secret); err != nil {
		return ctrl.Result{}, err
	}

	// currently, transport type is always kafka
	c.transportConfig.TransportType = string(transport.Kafka)
	var updated bool
	var err error

	_, enableKafka := secret.Data["kafka.yaml"]
	if enableKafka {
		updated, err = c.ReconcileKafkaCredential(ctx, secret)
		if err != nil {
			return ctrl.Result{}, err
		}

		if updated {
			// the consumer should reconcile when the credential is updated
			if !c.disableConsumer {
				if err := c.ReconcileConsumer(ctx); err != nil {
					log.Warnf("consumer error: %v", err)
					return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
				}
			}
			if err := c.ReconcileProducer(); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// if the rest.yaml exist, then build the rest client
	_, enableRestful := secret.Data["rest.yaml"]
	if enableRestful {
		updated, err = c.ReconcileRestfulCredential(ctx, secret)
		if err != nil {
			return ctrl.Result{}, err
		}
		if updated {
			if err := c.ReconcileRequester(ctx); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if !updated {
		return ctrl.Result{}, nil
	}

	if c.transportCallback != nil {
		if err := c.transportCallback(c.transportClient); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to invoke the callback function: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

// ReconcileProducer, transport config is changed, then create/update the producer
func (c *TransportCtrl) ReconcileProducer() error {
	// set producerTopic to spec or status topic based on running in manager or not
	if c.inManager {
		c.producerTopic = c.transportConfig.KafkaCredential.SpecTopic
	} else {
		c.producerTopic = c.transportConfig.KafkaCredential.StatusTopic
	}

	if c.transportClient.producer == nil {
		sender, err := producer.NewGenericProducer(c.transportConfig, c.producerTopic, nil)
		if err != nil {
			return fmt.Errorf("failed to create/update the producer: %w", err)
		}
		c.transportClient.producer = sender
	} else {
		if err := c.transportClient.producer.Reconnect(c.transportConfig, c.producerTopic); err != nil {
			return fmt.Errorf("failed to reconnect the producer: %w", err)
		}
	}
	return nil
}

// ReconcileConsumer, transport config is changed, then create/update the consumer
func (c *TransportCtrl) ReconcileConsumer(ctx context.Context) error {
	// if the consumer groupId is empty, then it's means the agent is in the standalone mode, don't create the consumer
	if c.transportConfig.KafkaCredential.ConsumerGroupID == "" {
		log.Infof("skip initializing consumer, consumer group id is not set")
		return nil
	}
	// set consumerTopics to status or spec topic based on running in manager or not
	if c.inManager {
		c.consumerTopics = []string{c.transportConfig.KafkaCredential.StatusTopic}
	} else {
		c.consumerTopics = []string{c.transportConfig.KafkaCredential.SpecTopic}
	}

	consumerGroupID := c.transportConfig.KafkaCredential.ConsumerGroupID
	// create/update the consumer with the kafka transport
	if c.transportClient.consumer == nil {
		receiver, err := consumer.NewGenericConsumer(c.transportConfig, c.consumerTopics)
		if err != nil {
			return fmt.Errorf("failed to create the consumer: %w", err)
		}
		c.transportClient.consumer = receiver
		go func() {
			log.Infof("start consumer: %s", consumerGroupID)
			if err = receiver.Start(ctx); err != nil {
				log.Warnf("stop the consumer(%s): %v", consumerGroupID, err)
			}
			// request and reset state(credential) to start a new one, error when starting
			log.Infof("consumer failed to start, requeue to start again: %s", consumerGroupID)
			c.workqueue.AddAfter(ctrl.Request{}, 10*time.Second)
		}()
	} else {
		err := c.transportClient.consumer.Reconnect(ctx, c.transportConfig, c.consumerTopics)
		if err != nil {
			return fmt.Errorf("failed to reconnect the consumer(%s): %v", consumerGroupID, err)
		}
	}
	return nil
}

// ReconcileInventory, transport config is changed, then create/update the inventory client
func (c *TransportCtrl) ReconcileRequester(ctx context.Context) error {
	if c.transportClient.requester == nil {

		if c.transportConfig.RestfulCredential == nil {
			return fmt.Errorf("the restful credential must not be nil")
		}
		inventoryClient, err := requester.NewInventoryClient(ctx, c.transportConfig.RestfulCredential)
		if err != nil {
			return fmt.Errorf("initial the inventory client error %w", err)
		}
		c.transportClient.requester = inventoryClient
	} else {
		if err := c.transportClient.requester.RefreshClient(ctx, c.transportConfig.RestfulCredential); err != nil {
			return err
		}
	}
	return nil
}

// ReconcileKafkaCredential update the kafka connection credential based on the secret, return true if the kafka
// credential is updated, It also create/update the consumer if not in the standalone mode
func (c *TransportCtrl) ReconcileKafkaCredential(ctx context.Context, secret *corev1.Secret) (bool, error) {
	// load the kafka connection credential based on the transport type. kafka, multiple
	kafkaConn, err := config.GetKafkaCredentialBySecret(secret, c.runtimeClient)
	if err != nil {
		return false, err
	}
	err = c.ResyncKafkaClientSecret(ctx, kafkaConn, secret)
	if err != nil {
		return false, err
	}
	// update the watching secret lits
	if kafkaConn.CASecretName != "" || !utils.ContainsString(c.extraSecretNames, kafkaConn.CASecretName) {
		c.extraSecretNames = append(c.extraSecretNames, kafkaConn.CASecretName)
	}
	if kafkaConn.ClientSecretName != "" || utils.ContainsString(c.extraSecretNames, kafkaConn.ClientSecretName) {
		c.extraSecretNames = append(c.extraSecretNames, kafkaConn.ClientSecretName)
	}

	// if credentials aren't updated, then return
	if reflect.DeepEqual(c.transportConfig.KafkaCredential, kafkaConn) {
		return false, nil
	}
	c.transportConfig.KafkaCredential = kafkaConn
	return true, nil
}

// Resync the kafka client secret because we recreate the kafka cluster in globalhub 1.4 and restore case.
func (c *TransportCtrl) ResyncKafkaClientSecret(ctx context.Context, kafkaConn *transport.KafkaConfig, secret *corev1.Secret) error {
	if kafkaConn.ClusterID == "" {
		return nil
	}

	isKafkaClusterIdEqual := true
	// if stored kafka cluster id is same as current cluster id, return
	if secret.Annotations != nil {
		if secret.Annotations[constants.KafkaClusterIdAnnotation] == kafkaConn.ClusterID {
			log.Debugf("cluster id is equal")
			return nil
		}
		isKafkaClusterIdEqual = false
	}

	log.Debugf("isKafkaClusterIdEqual : %v", isKafkaClusterIdEqual)

	// if it's new cluster, cluster is not equal (restore globalhub)
	if !isKafkaClusterIdEqual {
		if kafkaConn.ClientSecretName == "" {
			return nil
		}
		signedSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      kafkaConn.ClientSecretName,
				Namespace: secret.Namespace,
			},
		}
		log.Infof("remove kafka client secret: %v", kafkaConn.ClientSecretName)

		err := c.runtimeClient.Delete(ctx, signedSecret)
		if err != nil {
			return err
		}
	}

	transportSecret := secret.DeepCopy()
	if transportSecret.Annotations == nil {
		transportSecret.Annotations = make(map[string]string)
	}
	log.Infof("update transport config secret")

	transportSecret.Annotations[constants.KafkaClusterIdAnnotation] = kafkaConn.ClusterID
	return c.runtimeClient.Update(ctx, transportSecret)
}

func (c *TransportCtrl) ReconcileRestfulCredential(ctx context.Context, secret *corev1.Secret) (
	updated bool, err error,
) {
	restfulConn, err := config.GetRestfulConnBySecret(secret, c.runtimeClient)
	if err != nil {
		return updated, err
	}

	// update the watching secret lits
	if restfulConn.CASecretName != "" || !utils.ContainsString(c.extraSecretNames, restfulConn.CASecretName) {
		c.extraSecretNames = append(c.extraSecretNames, restfulConn.CASecretName)
	}
	if restfulConn.ClientSecretName != "" || utils.ContainsString(c.extraSecretNames, restfulConn.ClientSecretName) {
		c.extraSecretNames = append(c.extraSecretNames, restfulConn.ClientSecretName)
	}

	if reflect.DeepEqual(c.transportConfig.RestfulCredential, restfulConn) {
		return updated, err
	}
	updated = true
	c.transportConfig.RestfulCredential = restfulConn
	return updated, err
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
		WithOptions(
			controller.TypedOptions[ctrl.Request]{
				NewQueue: func(controllerName string, rateLimiter workqueue.TypedRateLimiter[ctrl.Request]) workqueue.TypedRateLimitingInterface[ctrl.Request] {
					c.workqueue = workqueue.NewTypedRateLimitingQueueWithConfig(rateLimiter, workqueue.TypedRateLimitingQueueConfig[ctrl.Request]{
						Name: controllerName,
					})
					return c.workqueue
				},
			},
		).Complete(c)
}

func (c *TransportCtrl) credentialSecret(name string) bool {
	if c.secretName == name {
		return true
	}
	if len(c.extraSecretNames) == 0 {
		return false
	}
	for _, secretName := range c.extraSecretNames {
		if name == secretName {
			return true
		}
	}
	return false
}
