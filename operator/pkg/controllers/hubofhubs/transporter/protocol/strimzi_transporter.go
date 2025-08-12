package protocol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/go-logr/logr"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	operatorv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/agent/certificates"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	KafkaClusterName = "kafka"
	// kafka storage
	DefaultKafkaDefaultStorageSize = "10Gi"

	// Global hub kafkaUser name
	DefaultGlobalHubKafkaUserName = "global-hub-kafka-user"

	// subscription - common
	DefaultKafkaSubName           = "strimzi-kafka-operator"
	DefaultInstallPlanApproval    = subv1alpha1.ApprovalAutomatic
	DefaultCatalogSourceNamespace = "openshift-marketplace"

	// subscription - production
	DefaultAMQChannel        = "amq-streams-2.7.x"
	DefaultAMQPackageName    = "amq-streams"
	DefaultCatalogSourceName = "redhat-operators"

	// subscription - community
	CommunityChannel           = "strimzi-0.41.x"
	CommunityPackageName       = "strimzi-kafka-operator"
	CommunityCatalogSourceName = "community-operators"
)

var (
	KafkaStorageIdentifier   int32 = 0
	KafkaStorageDeleteClaim        = false
	KafkaVersion                   = "3.7.0"
	DefaultPartition         int32 = 1
	DefaultPartitionReplicas int32 = 3
	// kafka metrics constants
	KakfaMetricsConfigmapName       = "kafka-metrics"
	KafkaMetricsConfigmapKeyRef     = "kafka-metrics-config.yml"
	ZooKeeperMetricsConfigmapKeyRef = "zookeeper-metrics-config.yml"
)

// install the strimzi kafka cluster by operator
type strimziTransporter struct {
	log                   logr.Logger
	ctx                   context.Context
	kafkaClusterName      string
	kafkaClusterNamespace string

	// subscription properties
	subName                   string
	subCommunity              bool
	subChannel                string
	subCatalogSourceName      string
	subCatalogSourceNamespace string
	subPackageName            string

	// global hub config
	mgh     *operatorv1alpha4.MulticlusterGlobalHub
	manager ctrl.Manager

	// wait until kafka cluster status is ready when initialize
	waitReady bool
	enableTLS bool
	// default is false, to create topic for each managed hub
	sharedTopics           bool
	topicPartitionReplicas int32
}

type KafkaOption func(*strimziTransporter)

var transporter *strimziTransporter

func NewStrimziTransporter(mgr ctrl.Manager, mgh *operatorv1alpha4.MulticlusterGlobalHub,
	opts ...KafkaOption,
) *strimziTransporter {
	if transporter == nil {
		transporter = &strimziTransporter{
			log:              ctrl.Log.WithName("strimzi-transporter"),
			ctx:              context.TODO(),
			kafkaClusterName: KafkaClusterName,

			subName:                   DefaultKafkaSubName,
			subCommunity:              false,
			subChannel:                DefaultAMQChannel,
			subPackageName:            DefaultAMQPackageName,
			subCatalogSourceName:      DefaultCatalogSourceName,
			subCatalogSourceNamespace: DefaultCatalogSourceNamespace,

			waitReady:              true,
			enableTLS:              true,
			sharedTopics:           false,
			topicPartitionReplicas: DefaultPartitionReplicas,

			manager: mgr,
		}
		config.SetTransporter(transporter)
	}

	transporter.mgh = mgh
	transporter.kafkaClusterNamespace = mgh.Namespace
	// apply options
	for _, opt := range opts {
		opt(transporter)
	}

	if transporter.subCommunity {
		transporter.subChannel = CommunityChannel
		transporter.subPackageName = CommunityPackageName
		transporter.subCatalogSourceName = CommunityCatalogSourceName
		// it will be operatorhubio-catalog to install kafka in KinD cluster
		catalogSourceName, ok := mgh.Annotations[operatorconstants.CommunityCatalogSourceNameKey]
		if ok && catalogSourceName != "" {
			transporter.subCatalogSourceName = catalogSourceName
		}
		catalogSourceNamespace, ok := mgh.Annotations[operatorconstants.CommunityCatalogSourceNamespaceKey]
		if ok && catalogSourceNamespace != "" {
			transporter.subCatalogSourceNamespace = catalogSourceNamespace
		}
	}

	if mgh.Spec.AvailabilityConfig == operatorv1alpha4.HABasic {
		transporter.topicPartitionReplicas = 1
	}

	return transporter
}

func WithNamespacedName(name types.NamespacedName) KafkaOption {
	return func(sk *strimziTransporter) {
		sk.kafkaClusterName = name.Name
		sk.kafkaClusterNamespace = name.Namespace
	}
}

func WithContext(ctx context.Context) KafkaOption {
	return func(sk *strimziTransporter) {
		sk.ctx = ctx
	}
}

func WithCommunity(val bool) KafkaOption {
	return func(sk *strimziTransporter) {
		sk.subCommunity = val
	}
}

func WithSubName(name string) KafkaOption {
	return func(sk *strimziTransporter) {
		sk.subName = name
	}
}

// EnsureKafka the kafka subscription, cluster, metrics, global hub user and topic
func (k *strimziTransporter) EnsureKafka() (bool, error) {
	k.log.V(2).Info("reconcile global hub kafka transport...")
	err := k.ensureSubscription(k.mgh)
	if err != nil {
		return true, err
	}

	if !config.GetKafkaResourceReady() {
		klog.Infof("wait for the Kafka CRD to be ready")
		return true, nil
	}

	_, enableKRaft := k.mgh.Annotations[operatorconstants.EnableKRaft]

	// Since the kafka cluster creation need the metric configmap, render the resource before creating the cluster
	// kafka metrics, monitor, global hub kafkaTopic and kafkaUser
	err = k.renderKafkaResources(k.mgh, enableKRaft)
	if err != nil {
		return true, err
	}

	if !enableKRaft {
		// TODO: use manifest to create kafka cluster
		err, _ = k.CreateUpdateKafkaCluster(k.mgh)
		if err != nil {
			return true, err
		}
	}

	return false, nil
}

// renderKafkaMetricsResources renders the kafka podmonitor and metrics, and kafkaUser and kafkaTopic for global hub
func (k *strimziTransporter) renderKafkaResources(mgh *operatorv1alpha4.MulticlusterGlobalHub,
	enableKRaft bool,
) error {
	statusTopic := config.GetRawStatusTopic()
	statusPlaceholderTopic := config.GetRawStatusTopic()
	topicParttern := kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral
	if strings.Contains(config.GetRawStatusTopic(), "*") {
		statusTopic = strings.Replace(config.GetRawStatusTopic(), "*", "", -1)
		statusPlaceholderTopic = strings.Replace(config.GetRawStatusTopic(), "*", "global-hub", -1)
		topicParttern = kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypePrefix
	}
	topicReplicas := DefaultPartitionReplicas
	if mgh.Spec.AvailabilityConfig == operatorv1alpha4.HABasic || enableKRaft {
		topicReplicas = 1
	}
	// brokerAdvertisedHost is used for test in KinD cluster. we need to use AdvertisedHost to pass tls authn.
	brokerAdvertisedHost := mgh.Annotations[operatorconstants.KafkaBrokerAdvertisedHostKey]

	// render the kafka objects
	kafkaRenderer, kafkaDeployer := renderer.NewHoHRenderer(manifests), deployer.NewHoHDeployer(k.manager.GetClient())
	kafkaObjects, err := kafkaRenderer.Render("manifests", "",
		func(profile string) (interface{}, error) {
			return struct {
				EnableMetrics          bool
				Namespace              string
				KafkaCluster           string
				GlobalHubKafkaUser     string
				SpecTopic              string
				StatusTopic            string
				StatusTopicParttern    string
				StatusPlaceholderTopic string
				TopicPartition         int32
				TopicReplicas          int32
				EnableKRaft            bool
				KinDClusterIPAddress   string
				EnableInventoryAPI     bool
				KafkaInventoryTopic    string
			}{
				EnableMetrics:          mgh.Spec.EnableMetrics,
				Namespace:              mgh.GetNamespace(),
				KafkaCluster:           KafkaClusterName,
				GlobalHubKafkaUser:     DefaultGlobalHubKafkaUserName,
				SpecTopic:              config.GetSpecTopic(),
				StatusTopic:            statusTopic,
				StatusTopicParttern:    string(topicParttern),
				StatusPlaceholderTopic: statusPlaceholderTopic,
				TopicPartition:         DefaultPartition,
				TopicReplicas:          topicReplicas,
				EnableKRaft:            enableKRaft,
				KinDClusterIPAddress:   brokerAdvertisedHost,
				EnableInventoryAPI:     config.WithInventory(mgh),
				KafkaInventoryTopic:    "kessel-inventory",
			}, nil
		})
	if err != nil {
		return fmt.Errorf("failed to render kafka manifests: %w", err)
	}
	// create restmapper for deployer to find GVR
	dc, err := discovery.NewDiscoveryClientForConfig(k.manager.GetConfig())
	if err != nil {
		return err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	if err = operatorutils.ManipulateGlobalHubObjects(kafkaObjects, mgh, kafkaDeployer, mapper,
		k.manager.GetScheme()); err != nil {
		return fmt.Errorf("failed to create/update kafka objects: %w", err)
	}
	return nil
}

// EnsureUser to reconcile the kafkaUser's setting(authn and authz)
func (k *strimziTransporter) EnsureUser(clusterName string) (string, error) {
	userName := config.GetKafkaUserName(clusterName)
	clusterTopic := k.getClusterTopic(clusterName)

	authnType := kafkav1beta2.KafkaUserSpecAuthenticationTypeTlsExternal
	simpleACLs := []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
		ConsumeGroupReadACL(),
		ReadTopicACL(clusterTopic.SpecTopic, false),
		WriteTopicACL(clusterTopic.StatusTopic),
	}

	desiredKafkaUser := k.newKafkaUser(userName, authnType, simpleACLs)

	kafkaUser := &kafkav1beta2.KafkaUser{}
	err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
		Name:      userName,
		Namespace: k.kafkaClusterNamespace,
	}, kafkaUser)
	if errors.IsNotFound(err) {
		klog.Infof("create the kafakUser: %s", userName)
		return userName, k.manager.GetClient().Create(k.ctx, desiredKafkaUser, &client.CreateOptions{})
	} else if err != nil {
		return "", err
	}

	updatedKafkaUser := &kafkav1beta2.KafkaUser{}
	err = operatorutils.MergeObjects(kafkaUser, desiredKafkaUser, updatedKafkaUser)
	if err != nil {
		return "", err
	}

	if !equality.Semantic.DeepDerivative(updatedKafkaUser.Spec, kafkaUser.Spec) {
		klog.Infof("update the kafkaUser: %s", userName)
		if err = k.manager.GetClient().Update(k.ctx, updatedKafkaUser); err != nil {
			return "", err
		}
	}
	return userName, nil
}

func (k *strimziTransporter) EnsureTopic(clusterName string) (*transport.ClusterTopic, error) {
	clusterTopic := k.getClusterTopic(clusterName)

	topicNames := []string{clusterTopic.SpecTopic, clusterTopic.StatusTopic}

	for _, topicName := range topicNames {
		kafkaTopic := &kafkav1beta2.KafkaTopic{}
		err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
			Name:      topicName,
			Namespace: k.kafkaClusterNamespace,
		}, kafkaTopic)
		if errors.IsNotFound(err) {
			if e := k.manager.GetClient().Create(k.ctx, k.newKafkaTopic(topicName)); e != nil {
				return nil, e
			}
			continue // reconcile the next topic
		} else if err != nil {
			return nil, err
		}

		// update the topic
		desiredTopic := k.newKafkaTopic(topicName)

		updatedTopic := &kafkav1beta2.KafkaTopic{}
		err = operatorutils.MergeObjects(kafkaTopic, desiredTopic, updatedTopic)
		if err != nil {
			return nil, err
		}
		// Kafka do not support change exitsting kafaka topic replica directly.
		updatedTopic.Spec.Replicas = kafkaTopic.Spec.Replicas

		if !equality.Semantic.DeepDerivative(updatedTopic.Spec, kafkaTopic.Spec) {
			if err = k.manager.GetClient().Update(k.ctx, updatedTopic); err != nil {
				return nil, err
			}
		}
	}
	return clusterTopic, nil
}

func (k *strimziTransporter) Prune(clusterName string) error {
	// cleanup kafkaUser
	kafkaUser := &kafkav1beta2.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.GetKafkaUserName(clusterName),
			Namespace: k.kafkaClusterNamespace,
		},
	}
	err := k.manager.GetClient().Get(k.ctx, client.ObjectKeyFromObject(kafkaUser), kafkaUser)
	if err == nil {
		if err := k.manager.GetClient().Delete(k.ctx, kafkaUser); err != nil {
			return err
		}
	} else if !errors.IsNotFound(err) {
		return err
	}

	// only delete the topic when removing the CR, otherwise the manager throws error like "Unknown topic or partition"
	// if k.sharedTopics || !strings.Contains(config.GetRawStatusTopic(), "*") {
	// 	return nil
	// }

	// clusterTopic := k.getClusterTopic(clusterName)
	// kafkaTopic := &kafkav1beta2.KafkaTopic{
	// 	ObjectMeta: metav1.ObjectMeta{
	// 		Name:      clusterTopic.StatusTopic,
	// 		Namespace: k.kafkaClusterNamespace,
	// 	},
	// }
	// err = k.manager.GetClient().Get(k.ctx, client.ObjectKeyFromObject(kafkaTopic), kafkaTopic)
	// if err == nil {
	// 	if err := k.manager.GetClient().Delete(k.ctx, kafkaTopic); err != nil {
	// 		return err
	// 	}
	// } else if !errors.IsNotFound(err) {
	// 	return err
	// }

	return nil
}

func (k *strimziTransporter) getClusterTopic(clusterName string) *transport.ClusterTopic {
	topic := &transport.ClusterTopic{
		SpecTopic:   config.GetSpecTopic(),
		StatusTopic: config.GetStatusTopic(clusterName),
	}
	return topic
}

// the username is the kafkauser, it's the same as the secret name
func (k *strimziTransporter) GetConnCredential(clusterName string) (*transport.KafkaConfig, error) {
	// bootstrapServer, clusterId, clusterCA
	credential, err := k.getConnCredentailByCluster()
	if err != nil {
		return nil, err
	}

	// certificates
	credential.CASecretName = GetClusterCASecret(k.kafkaClusterName)
	credential.ClientSecretName = certificates.AgentCertificateSecretName()

	// topics
	credential.StatusTopic = config.GetStatusTopic(clusterName)
	credential.SpecTopic = config.GetSpecTopic()

	// don't need to load the client cert/key from the kafka user, since it use the external kafkaUser
	// userName := config.GetKafkaUserName(clusterName)
	// if !k.enableTLS {
	// 	k.log.Info("the kafka cluster hasn't enable tls for user", "username", userName)
	// 	return credential, nil
	// }
	// if err := k.loadUserCredentail(userName, credential); err != nil {
	// 	return nil, err
	// }
	return credential, nil
}

func GetClusterCASecret(clusterName string) string {
	return fmt.Sprintf("%s-cluster-ca-cert", clusterName)
}

// loadUserCredentail add credential with client cert, and key
func (k *strimziTransporter) loadUserCredentail(kafkaUserName string, credential *transport.KafkaConfig) error {
	kafkaUserSecret := &corev1.Secret{}
	err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
		Name:      kafkaUserName,
		Namespace: k.kafkaClusterNamespace,
	}, kafkaUserSecret)
	if err != nil {
		return err
	}
	credential.ClientCert = base64.StdEncoding.EncodeToString(kafkaUserSecret.Data["user.crt"])
	credential.ClientKey = base64.StdEncoding.EncodeToString(kafkaUserSecret.Data["user.key"])
	return nil
}

// getConnCredentailByCluster gets credential with clusterId, bootstrapServer, and serverCA
func (k *strimziTransporter) getConnCredentailByCluster() (*transport.KafkaConfig, error) {
	kafkaCluster := &kafkav1beta2.Kafka{}
	err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
		Name:      k.kafkaClusterName,
		Namespace: k.kafkaClusterNamespace,
	}, kafkaCluster)
	if err != nil {
		return nil, err
	}

	if kafkaCluster.Status == nil || kafkaCluster.Status.Conditions == nil {
		return nil, fmt.Errorf("kafka cluster %s has no status conditions", kafkaCluster.Name)
	}

	for _, condition := range kafkaCluster.Status.Conditions {
		if *condition.Type == "Ready" && *condition.Status == "True" {
			clusterIdentity := string(kafkaCluster.GetUID())
			if kafkaCluster.Status.ClusterId != nil {
				clusterIdentity = *kafkaCluster.Status.ClusterId
			}
			credential := &transport.KafkaConfig{
				ClusterID:       clusterIdentity,
				BootstrapServer: *kafkaCluster.Status.Listeners[1].BootstrapServers,
				CACert:          base64.StdEncoding.EncodeToString([]byte(kafkaCluster.Status.Listeners[1].Certificates[0])),
			}
			return credential, nil
		}
	}
	return nil, fmt.Errorf("kafka cluster %s/%s is not ready", k.kafkaClusterNamespace, k.kafkaClusterName)
}

func (k *strimziTransporter) newKafkaTopic(topicName string) *kafkav1beta2.KafkaTopic {
	return &kafkav1beta2.KafkaTopic{
		ObjectMeta: metav1.ObjectMeta{
			Name:      topicName,
			Namespace: k.kafkaClusterNamespace,
			Labels: map[string]string{
				// It is important to set the cluster label otherwise the topic will not be ready
				"strimzi.io/cluster":             k.kafkaClusterName,
				constants.GlobalHubOwnerLabelKey: constants.GlobalHubOwnerLabelVal,
			},
		},
		Spec: &kafkav1beta2.KafkaTopicSpec{
			Partitions: &DefaultPartition,
			Replicas:   &k.topicPartitionReplicas,
			Config: &apiextensions.JSON{Raw: []byte(`{
				"cleanup.policy": "compact"
			}`)},
		},
	}
}

func (k *strimziTransporter) newKafkaUser(
	userName string,
	authnType kafkav1beta2.KafkaUserSpecAuthenticationType,
	simpleACLs []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem,
) *kafkav1beta2.KafkaUser {
	return &kafkav1beta2.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userName,
			Namespace: k.kafkaClusterNamespace,
			Labels: map[string]string{
				// It is important to set the cluster label otherwise the user will not be ready
				"strimzi.io/cluster":             k.kafkaClusterName,
				constants.GlobalHubOwnerLabelKey: constants.GlobalHubOwnerLabelVal,
			},
		},
		Spec: &kafkav1beta2.KafkaUserSpec{
			Authentication: &kafkav1beta2.KafkaUserSpecAuthentication{
				Type: authnType,
			},
			Authorization: &kafkav1beta2.KafkaUserSpecAuthorization{
				Type: kafkav1beta2.KafkaUserSpecAuthorizationTypeSimple,
				Acls: simpleACLs,
			},
		},
	}
}

// waits for kafka cluster to be ready and returns nil if kafka cluster ready
func (k *strimziTransporter) kafkaClusterReady() (bool, error) {
	kafkaCluster := &kafkav1beta2.Kafka{}
	isReady := false
	var kakfaReason, kafkaMsg string
	var err error
	defer func() {
		err = k.updateMghKafkaComponent(isReady, k.manager.GetClient(), kakfaReason, kafkaMsg)
		if err != nil {
			klog.Errorf("failed to update mgh status, err: %v", err)
		}
	}()
	err = k.manager.GetClient().Get(k.ctx, types.NamespacedName{
		Name:      k.kafkaClusterName,
		Namespace: k.kafkaClusterNamespace,
	}, kafkaCluster)
	if err != nil {
		k.log.V(2).Info("fail to get the kafka cluster, waiting", "message", err.Error())
		if errors.IsNotFound(err) {
			return isReady, nil
		}
		return isReady, err
	}
	if kafkaCluster.Status == nil || kafkaCluster.Status.Conditions == nil {
		return isReady, nil
	}

	if kafkaCluster.Spec != nil && kafkaCluster.Spec.Kafka.Listeners != nil {
		// if the kafka cluster is already created, check if the tls is enabled
		enableTLS := false
		for _, listener := range kafkaCluster.Spec.Kafka.Listeners {
			if listener.Tls {
				enableTLS = true
				break
			}
		}
		k.enableTLS = enableTLS
	}

	for _, condition := range kafkaCluster.Status.Conditions {
		if *condition.Type == "Ready" {
			if *condition.Status == "True" {
				k.log.Info("kafka cluster is ready")
				isReady = true
				return isReady, nil
			}
			kafkaMsg = *condition.Message
			kakfaReason = *condition.Reason
			return isReady, nil
		}
	}
	klog.Infof("wait for the Kafka cluster to be ready")
	return isReady, nil
}

func (k *strimziTransporter) updateMghKafkaComponent(ready bool, c client.Client, kafkaReason, kafkaMsg string) error {
	var kafkastatus v1alpha4.StatusCondition
	if ready {
		kafkastatus = v1alpha4.StatusCondition{
			Kind:    "Kafka",
			Name:    config.COMPONENTS_KAFKA_NAME,
			Type:    config.COMPONENTS_AVAILABLE,
			Status:  config.CONDITION_STATUS_TRUE,
			Reason:  config.CONDITION_REASON_KAFKA_READY,
			Message: config.CONDITION_MESSAGE_KAFKA_READY,
		}
	} else {
		reason := "KafkaNotReady"
		message := "Kafka cluster is not ready"
		if kafkaReason != "" {
			reason = kafkaReason
		}
		if kafkaMsg != "" {
			message = kafkaMsg
		}
		kafkastatus = v1alpha4.StatusCondition{
			Kind:    "Kafka",
			Name:    config.COMPONENTS_KAFKA_NAME,
			Type:    config.COMPONENTS_AVAILABLE,
			Status:  config.CONDITION_STATUS_FALSE,
			Reason:  reason,
			Message: message,
		}
	}
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		now := time.Now()
		kafkastatus.LastTransitionTime = metav1.Time{Time: now}
		curmgh := &v1alpha4.MulticlusterGlobalHub{}
		err := c.Get(k.ctx, types.NamespacedName{
			Name:      k.mgh.Name,
			Namespace: k.mgh.Namespace,
		}, curmgh)
		if err != nil {
			return err
		}
		if curmgh.Status.Components == nil {
			curmgh.Status.Components = map[string]operatorv1alpha4.StatusCondition{
				config.COMPONENTS_KAFKA_NAME: kafkastatus,
			}
		} else {
			curmghKafkaStatus := curmgh.Status.Components[config.COMPONENTS_KAFKA_NAME]
			curmghKafkaStatus.LastTransitionTime = metav1.Time{Time: now}
			if reflect.DeepEqual(curmghKafkaStatus, kafkastatus) {
				return nil
			}
		}
		curmgh.Status.Components[config.COMPONENTS_KAFKA_NAME] = kafkastatus
		return c.Status().Update(k.ctx, curmgh)
	})
}

func (k *strimziTransporter) CreateUpdateKafkaCluster(mgh *operatorv1alpha4.MulticlusterGlobalHub) (error, bool) {
	existingKafka := &kafkav1beta2.Kafka{}
	err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
		Name:      k.kafkaClusterName,
		Namespace: mgh.Namespace,
	}, existingKafka)
	if err != nil {
		if errors.IsNotFound(err) {
			return k.manager.GetClient().Create(k.ctx, k.newKafkaCluster(mgh)), true
		}
		return err, false
	}

	desiredKafka := k.newKafkaCluster(mgh)

	updatedKafka := &kafkav1beta2.Kafka{}
	err = operatorutils.MergeObjects(existingKafka, desiredKafka, updatedKafka)
	if err != nil {
		return err, false
	}

	updatedKafka.Spec.Kafka.MetricsConfig = desiredKafka.Spec.Kafka.MetricsConfig
	updatedKafka.Spec.Zookeeper.MetricsConfig = desiredKafka.Spec.Zookeeper.MetricsConfig

	if !reflect.DeepEqual(updatedKafka.Spec, existingKafka.Spec) {
		return k.manager.GetClient().Update(k.ctx, updatedKafka), true
	}
	return nil, false
}

func (k *strimziTransporter) getKafkaResources(
	mgh *operatorv1alpha4.MulticlusterGlobalHub,
) *kafkav1beta2.KafkaSpecKafkaResources {
	kafkaRes := operatorutils.GetResources(operatorconstants.Kafka, mgh.Spec.AdvancedSpec)
	kafkaSpecRes := &kafkav1beta2.KafkaSpecKafkaResources{}
	jsonData, err := json.Marshal(kafkaRes)
	if err != nil {
		k.log.Error(err, "failed to marshal kafka resources")
	}
	err = json.Unmarshal(jsonData, kafkaSpecRes)
	if err != nil {
		k.log.Error(err, "failed to unmarshal to KafkaSpecKafkaResources")
	}

	return kafkaSpecRes
}

func (k *strimziTransporter) getZookeeperResources(
	mgh *operatorv1alpha4.MulticlusterGlobalHub,
) *kafkav1beta2.KafkaSpecZookeeperResources {
	zookeeperRes := operatorutils.GetResources(operatorconstants.Zookeeper, mgh.Spec.AdvancedSpec)

	zookeeperSpecRes := &kafkav1beta2.KafkaSpecZookeeperResources{}
	jsonData, err := json.Marshal(zookeeperRes)
	if err != nil {
		k.log.Error(err, "failed to marshal zookeeper resources")
	}
	err = json.Unmarshal(jsonData, zookeeperSpecRes)
	if err != nil {
		k.log.Error(err, "failed to unmarshal to KafkaSpecZookeeperResources")
	}
	return zookeeperSpecRes
}

func (k *strimziTransporter) newKafkaCluster(mgh *operatorv1alpha4.MulticlusterGlobalHub) *kafkav1beta2.Kafka {
	storageSize := config.GetKafkaStorageSize(mgh)
	kafkaSpecKafkaStorageVolumesElem := kafkav1beta2.KafkaSpecKafkaStorageVolumesElem{
		Id:          &KafkaStorageIdentifier,
		Size:        &storageSize,
		Type:        kafkav1beta2.KafkaSpecKafkaStorageVolumesElemTypePersistentClaim,
		DeleteClaim: &KafkaStorageDeleteClaim,
	}
	kafkaSpecZookeeperStorage := kafkav1beta2.KafkaSpecZookeeperStorage{
		Type:        kafkav1beta2.KafkaSpecZookeeperStorageTypePersistentClaim,
		Size:        &storageSize,
		DeleteClaim: &KafkaStorageDeleteClaim,
	}

	if mgh.Spec.DataLayerSpec.StorageClass != "" {
		kafkaSpecKafkaStorageVolumesElem.Class = &mgh.Spec.DataLayerSpec.StorageClass
		kafkaSpecZookeeperStorage.Class = &mgh.Spec.DataLayerSpec.StorageClass
	}

	kafkaCluster := &kafkav1beta2.Kafka{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k.kafkaClusterName,
			Namespace: k.kafkaClusterNamespace,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GlobalHubOwnerLabelVal,
			},
		},
		Spec: &kafkav1beta2.KafkaSpec{
			Kafka: kafkav1beta2.KafkaSpecKafka{
				Config: &apiextensions.JSON{Raw: []byte(`{
"default.replication.factor": 3,
"inter.broker.protocol.version": "3.7",
"min.insync.replicas": 2,
"offsets.topic.replication.factor": 3,
"transaction.state.log.min.isr": 2,
"transaction.state.log.replication.factor": 3
}`)},
				Listeners: []kafkav1beta2.KafkaSpecKafkaListenersElem{
					{
						Name: "plain",
						Port: 9092,
						Tls:  false,
						Type: kafkav1beta2.KafkaSpecKafkaListenersElemTypeInternal,
					},
					{
						Name: "tls",
						Port: 9093,
						Tls:  true,
						Type: kafkav1beta2.KafkaSpecKafkaListenersElemTypeRoute,
						Authentication: &kafkav1beta2.KafkaSpecKafkaListenersElemAuthentication{
							Type: kafkav1beta2.KafkaSpecKafkaListenersElemAuthenticationTypeTls,
						},
					},
				},
				Resources: k.getKafkaResources(mgh),
				Authorization: &kafkav1beta2.KafkaSpecKafkaAuthorization{
					Type: kafkav1beta2.KafkaSpecKafkaAuthorizationTypeSimple,
				},
				Replicas: &DefaultPartitionReplicas,
				Storage: &kafkav1beta2.KafkaSpecKafkaStorage{
					Type: kafkav1beta2.KafkaSpecKafkaStorageTypeJbod,
					Volumes: []kafkav1beta2.KafkaSpecKafkaStorageVolumesElem{
						kafkaSpecKafkaStorageVolumesElem,
					},
				},
				Version: &KafkaVersion,
			},
			Zookeeper: &kafkav1beta2.KafkaSpecZookeeper{
				Replicas:  3,
				Storage:   kafkaSpecZookeeperStorage,
				Resources: k.getZookeeperResources(mgh),
			},
			EntityOperator: &kafkav1beta2.KafkaSpecEntityOperator{
				TopicOperator: &kafkav1beta2.KafkaSpecEntityOperatorTopicOperator{},
				UserOperator:  &kafkav1beta2.KafkaSpecEntityOperatorUserOperator{},
			},
		},
	}

	k.setAffinity(mgh, kafkaCluster)
	k.setTolerations(mgh, kafkaCluster)
	k.setMetricsConfig(mgh, kafkaCluster)
	k.setImagePullSecret(mgh, kafkaCluster)

	return kafkaCluster
}

// set metricsConfig for kafka cluster based on the mgh enableMetrics
func (k *strimziTransporter) setMetricsConfig(mgh *operatorv1alpha4.MulticlusterGlobalHub,
	kafkaCluster *kafkav1beta2.Kafka,
) {
	kafkaMetricsConfig := &kafkav1beta2.KafkaSpecKafkaMetricsConfig{}
	zookeeperMetricsConfig := &kafkav1beta2.KafkaSpecZookeeperMetricsConfig{}
	if mgh.Spec.EnableMetrics {
		kafkaMetricsConfig = &kafkav1beta2.KafkaSpecKafkaMetricsConfig{
			Type: kafkav1beta2.KafkaSpecKafkaMetricsConfigTypeJmxPrometheusExporter,
			ValueFrom: kafkav1beta2.KafkaSpecKafkaMetricsConfigValueFrom{
				ConfigMapKeyRef: &kafkav1beta2.KafkaSpecKafkaMetricsConfigValueFromConfigMapKeyRef{
					Name: &KakfaMetricsConfigmapName,
					Key:  &KafkaMetricsConfigmapKeyRef,
				},
			},
		}
		zookeeperMetricsConfig = &kafkav1beta2.KafkaSpecZookeeperMetricsConfig{
			Type: kafkav1beta2.KafkaSpecZookeeperMetricsConfigTypeJmxPrometheusExporter,
			ValueFrom: kafkav1beta2.KafkaSpecZookeeperMetricsConfigValueFrom{
				ConfigMapKeyRef: &kafkav1beta2.KafkaSpecZookeeperMetricsConfigValueFromConfigMapKeyRef{
					Name: &KakfaMetricsConfigmapName,
					Key:  &ZooKeeperMetricsConfigmapKeyRef,
				},
			},
		}
		kafkaCluster.Spec.Kafka.MetricsConfig = kafkaMetricsConfig
		kafkaCluster.Spec.Zookeeper.MetricsConfig = zookeeperMetricsConfig
	}
}

// set affinity for kafka cluster based on the mgh nodeSelector
func (k *strimziTransporter) setAffinity(mgh *operatorv1alpha4.MulticlusterGlobalHub,
	kafkaCluster *kafkav1beta2.Kafka,
) {
	kafkaPodAffinity := &kafkav1beta2.KafkaSpecKafkaTemplatePodAffinity{}
	zookeeperPodAffinity := &kafkav1beta2.KafkaSpecZookeeperTemplatePodAffinity{}
	entityOperatorPodAffinity := &kafkav1beta2.KafkaSpecEntityOperatorTemplatePodAffinity{}

	if mgh.Spec.NodeSelector != nil {
		nodeSelectorReqs := []corev1.NodeSelectorRequirement{}

		for key, value := range mgh.Spec.NodeSelector {
			req := corev1.NodeSelectorRequirement{
				Key:      key,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{value},
			}
			nodeSelectorReqs = append(nodeSelectorReqs, req)
		}
		nodeSelectorTerms := []corev1.NodeSelectorTerm{
			{
				MatchExpressions: nodeSelectorReqs,
			},
		}

		jsonData, err := json.Marshal(nodeSelectorTerms)
		if err != nil {
			k.log.Error(err, "failed to marshall nodeSelector terms")
		}

		kafkaNodeSelectorTermsElem := make([]kafkav1beta2.
			KafkaSpecKafkaTemplatePodAffinityNodeAffinityRequiredDuringSchedulingIgnoredDuringExecutionNodeSelectorTermsElem,
			0)

		zookeeperNodeSelectorTermsElem := make([]kafkav1beta2.
			KafkaSpecZookeeperTemplatePodAffinityNodeAffinityRequiredDuringSchedulingIgnoredDuringExecutionNodeSelectorTermsElem, 0)
		entityOperatorNodeSelectorTermsElem := make([]kafkav1beta2.
			KafkaSpecEntityOperatorTemplatePodAffinityNodeAffinityRequiredDuringSchedulingIgnoredDuringExecutionNodeSelectorTermsElem, 0)

		err = json.Unmarshal(jsonData, &kafkaNodeSelectorTermsElem)
		if err != nil {
			k.log.Error(err, "failed to unmarshal to kafkaNodeSelectorTermsElem")
		}
		err = json.Unmarshal(jsonData, &zookeeperNodeSelectorTermsElem)
		if err != nil {
			k.log.Error(err, "failed to unmarshal to zookeeperNodeSelectorTermsElem")
		}
		err = json.Unmarshal(jsonData, &entityOperatorNodeSelectorTermsElem)
		if err != nil {
			k.log.Error(err, "failed to unmarshal to entityOperatorNodeSelectorTermsElem")
		}

		zookeeperPodAffinity.NodeAffinity = &kafkav1beta2.KafkaSpecZookeeperTemplatePodAffinityNodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &kafkav1beta2.
				KafkaSpecZookeeperTemplatePodAffinityNodeAffinityRequiredDuringSchedulingIgnoredDuringExecution{
				NodeSelectorTerms: zookeeperNodeSelectorTermsElem,
			},
		}
		kafkaPodAffinity.NodeAffinity = &kafkav1beta2.KafkaSpecKafkaTemplatePodAffinityNodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &kafkav1beta2.
				KafkaSpecKafkaTemplatePodAffinityNodeAffinityRequiredDuringSchedulingIgnoredDuringExecution{
				NodeSelectorTerms: kafkaNodeSelectorTermsElem,
			},
		}
		entityOperatorPodAffinity.NodeAffinity = &kafkav1beta2.KafkaSpecEntityOperatorTemplatePodAffinityNodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &kafkav1beta2.
				KafkaSpecEntityOperatorTemplatePodAffinityNodeAffinityRequiredDuringSchedulingIgnoredDuringExecution{
				NodeSelectorTerms: entityOperatorNodeSelectorTermsElem,
			},
		}

		if kafkaCluster.Spec.Kafka.Template == nil {
			kafkaCluster.Spec.Kafka.Template = &kafkav1beta2.KafkaSpecKafkaTemplate{
				Pod: &kafkav1beta2.KafkaSpecKafkaTemplatePod{
					Affinity: kafkaPodAffinity,
				},
			}
			kafkaCluster.Spec.Zookeeper.Template = &kafkav1beta2.KafkaSpecZookeeperTemplate{
				Pod: &kafkav1beta2.KafkaSpecZookeeperTemplatePod{
					Affinity: zookeeperPodAffinity,
				},
			}
			kafkaCluster.Spec.EntityOperator.Template = &kafkav1beta2.KafkaSpecEntityOperatorTemplate{
				Pod: &kafkav1beta2.KafkaSpecEntityOperatorTemplatePod{
					Affinity: entityOperatorPodAffinity,
				},
			}
		} else {
			kafkaCluster.Spec.Kafka.Template.Pod.Affinity = kafkaPodAffinity
			kafkaCluster.Spec.Zookeeper.Template.Pod.Affinity = zookeeperPodAffinity
			kafkaCluster.Spec.EntityOperator.Template.Pod.Affinity = entityOperatorPodAffinity
		}
	}
}

// setTolerations sets the kafka tolerations based on the mgh tolerations
func (k *strimziTransporter) setTolerations(mgh *operatorv1alpha4.MulticlusterGlobalHub,
	kafkaCluster *kafkav1beta2.Kafka,
) {
	kafkaTolerationsElem := make([]kafkav1beta2.KafkaSpecKafkaTemplatePodTolerationsElem, 0)
	zookeeperTolerationsElem := make([]kafkav1beta2.KafkaSpecZookeeperTemplatePodTolerationsElem, 0)
	entityOperatorTolerationsElem := make([]kafkav1beta2.KafkaSpecEntityOperatorTemplatePodTolerationsElem, 0)

	if mgh.Spec.Tolerations != nil {
		jsonData, err := json.Marshal(mgh.Spec.Tolerations)
		if err != nil {
			k.log.Error(err, "failed to marshal tolerations")
		}
		err = json.Unmarshal(jsonData, &kafkaTolerationsElem)
		if err != nil {
			k.log.Error(err, "failed to unmarshal to KafkaSpecruntimeKafkaTemplatePodTolerationsElem")
		}
		err = json.Unmarshal(jsonData, &zookeeperTolerationsElem)
		if err != nil {
			k.log.Error(err, "failed to unmarshal to KafkaSpecZookeeperTemplatePodTolerationsElem")
		}
		err = json.Unmarshal(jsonData, &entityOperatorTolerationsElem)
		if err != nil {
			k.log.Error(err, "failed to unmarshal to KafkaSpecEntityOperatorTemplatePodTolerationsElem")
		}

		if kafkaCluster.Spec.Kafka.Template == nil {
			kafkaCluster.Spec.Kafka.Template = &kafkav1beta2.KafkaSpecKafkaTemplate{
				Pod: &kafkav1beta2.KafkaSpecKafkaTemplatePod{
					Tolerations: kafkaTolerationsElem,
				},
			}
			kafkaCluster.Spec.Zookeeper.Template = &kafkav1beta2.KafkaSpecZookeeperTemplate{
				Pod: &kafkav1beta2.KafkaSpecZookeeperTemplatePod{
					Tolerations: zookeeperTolerationsElem,
				},
			}
			kafkaCluster.Spec.EntityOperator.Template = &kafkav1beta2.KafkaSpecEntityOperatorTemplate{
				Pod: &kafkav1beta2.KafkaSpecEntityOperatorTemplatePod{
					Tolerations: entityOperatorTolerationsElem,
				},
			}
		} else {
			kafkaCluster.Spec.Kafka.Template.Pod.Tolerations = kafkaTolerationsElem
			kafkaCluster.Spec.Zookeeper.Template.Pod.Tolerations = zookeeperTolerationsElem
			kafkaCluster.Spec.EntityOperator.Template.Pod.Tolerations = entityOperatorTolerationsElem
		}
	}
}

// setImagePullSecret sets the kafka image pull secret based on the mgh imagepullsecret
func (k *strimziTransporter) setImagePullSecret(mgh *operatorv1alpha4.MulticlusterGlobalHub,
	kafkaCluster *kafkav1beta2.Kafka,
) {
	if mgh.Spec.ImagePullSecret != "" {
		existingKafkaSpec := kafkaCluster.Spec
		desiredKafkaSpec := kafkaCluster.Spec.DeepCopy()
		desiredKafkaSpec.EntityOperator.Template = &kafkav1beta2.KafkaSpecEntityOperatorTemplate{
			Pod: &kafkav1beta2.KafkaSpecEntityOperatorTemplatePod{
				ImagePullSecrets: []kafkav1beta2.KafkaSpecEntityOperatorTemplatePodImagePullSecretsElem{
					{
						Name: &mgh.Spec.ImagePullSecret,
					},
				},
			},
		}
		desiredKafkaSpec.Kafka.Template = &kafkav1beta2.KafkaSpecKafkaTemplate{
			Pod: &kafkav1beta2.KafkaSpecKafkaTemplatePod{
				ImagePullSecrets: []kafkav1beta2.KafkaSpecKafkaTemplatePodImagePullSecretsElem{
					{
						Name: &mgh.Spec.ImagePullSecret,
					},
				},
			},
		}
		desiredKafkaSpec.Zookeeper.Template = &kafkav1beta2.KafkaSpecZookeeperTemplate{
			Pod: &kafkav1beta2.KafkaSpecZookeeperTemplatePod{
				ImagePullSecrets: []kafkav1beta2.KafkaSpecZookeeperTemplatePodImagePullSecretsElem{
					{
						Name: &mgh.Spec.ImagePullSecret,
					},
				},
			},
		}
		// marshal to json
		existingKafkaJson, _ := json.Marshal(existingKafkaSpec)
		desiredKafkaJson, _ := json.Marshal(desiredKafkaSpec)

		// patch the desired kafka cluster to the existing kafka cluster
		patchedData, err := jsonpatch.MergePatch(existingKafkaJson, desiredKafkaJson)
		if err != nil {
			klog.Errorf("failed to merge patch, error: %v", err)
			return
		}

		updatedKafkaSpec := &kafkav1beta2.KafkaSpec{}
		err = json.Unmarshal(patchedData, updatedKafkaSpec)
		if err != nil {
			klog.Errorf("failed to umarshal kafkaspec, error: %v", err)
			return
		}
		kafkaCluster.Spec = updatedKafkaSpec
	}
}

// create/ update the kafka subscription
func (k *strimziTransporter) ensureSubscription(mgh *operatorv1alpha4.MulticlusterGlobalHub) error {
	// get subscription
	existingSub := &subv1alpha1.Subscription{}
	err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
		Name:      k.subName,
		Namespace: mgh.GetNamespace(),
	}, existingSub)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	expectedSub := k.newSubscription(mgh)
	if errors.IsNotFound(err) {
		return k.manager.GetClient().Create(k.ctx, expectedSub)
	} else {
		startingCSV := expectedSub.Spec.StartingCSV
		// if updating channel must remove startingCSV
		if existingSub.Spec.Channel != expectedSub.Spec.Channel {
			startingCSV = ""
		}
		if !equality.Semantic.DeepEqual(existingSub.Spec, expectedSub.Spec) {
			existingSub.Spec = expectedSub.Spec
		}
		existingSub.Spec.StartingCSV = startingCSV
		return k.manager.GetClient().Update(k.ctx, existingSub)
	}
}

// newSubscription returns an CrunchyPostgres subscription with desired default values
func (k *strimziTransporter) newSubscription(mgh *operatorv1alpha4.MulticlusterGlobalHub) *subv1alpha1.Subscription {
	labels := map[string]string{
		"installer.name":                 mgh.Name,
		"installer.namespace":            mgh.Namespace,
		constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
	}
	// Generate sub config from mgh CR
	subConfig := &subv1alpha1.SubscriptionConfig{
		NodeSelector: mgh.Spec.NodeSelector,
		Tolerations:  mgh.Spec.Tolerations,
	}

	sub := &subv1alpha1.Subscription{
		TypeMeta: metav1.TypeMeta{
			APIVersion: subv1alpha1.SubscriptionCRDAPIVersion,
			Kind:       subv1alpha1.SubscriptionKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      k.subName,
			Namespace: mgh.Namespace,
			Labels:    labels,
		},
		Spec: &subv1alpha1.SubscriptionSpec{
			Channel:                k.subChannel,
			InstallPlanApproval:    DefaultInstallPlanApproval,
			Package:                k.subPackageName,
			CatalogSource:          k.subCatalogSourceName,
			CatalogSourceNamespace: k.subCatalogSourceNamespace,
			Config:                 subConfig,
		},
	}
	return sub
}

func WriteTopicACL(topicName string) kafkav1beta2.KafkaUserSpecAuthorizationAclsElem {
	host := "*"
	patternType := kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral
	writeAcl := kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
		Host: &host,
		Resource: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResource{
			Type:        kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
			Name:        &topicName,
			PatternType: &patternType,
		},
		Operations: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemWrite,
		},
	}
	return writeAcl
}

func ReadTopicACL(topicName string, prefixParttern bool) kafkav1beta2.KafkaUserSpecAuthorizationAclsElem {
	host := "*"
	patternType := kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral
	if prefixParttern {
		patternType = kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypePrefix
	}

	return kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
		Host: &host,
		Resource: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResource{
			Type:        kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
			Name:        &topicName,
			PatternType: &patternType,
		},
		Operations: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemDescribe,
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
		},
	}
}

func ConsumeGroupReadACL() kafkav1beta2.KafkaUserSpecAuthorizationAclsElem {
	host := "*"
	consumerGroup := "*"
	consumerPatternType := kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral
	consumerAcl := kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
		Host: &host,
		Resource: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResource{
			Type:        kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeGroup,
			Name:        &consumerGroup,
			PatternType: &consumerPatternType,
		},
		Operations: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
		},
	}
	return consumerAcl
}
