package protocol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	jsonpatch "github.com/evanphx/json-patch"
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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
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
	DefaultAMQChannel        = "amq-streams-3.1.x"
	DefaultAMQPackageName    = "amq-streams"
	DefaultCatalogSourceName = "redhat-operators"

	// subscription - community
	CommunityChannel           = "strimzi-0.49.x"
	CommunityPackageName       = "strimzi-kafka-operator"
	CommunityCatalogSourceName = "community-operators"
)

var (
	DefaultAMQKafkaVersion         = "4.1.0"
	KafkaStorageIdentifier   int32 = 0
	KafkaStorageDeleteClaim        = false
	DefaultPartition         int32 = 1
	DefaultPartitionReplicas int32 = 3
	// kafka metrics constants
	KafkaMetricsConfigMapName   = "kafka-metrics"
	KafkaMetricsConfigMapKeyRef = "kafka-metrics-config.yml"
)

// install the strimzi kafka cluster by operator
type strimziTransporter struct {
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
			ctx:                       context.TODO(),
			kafkaClusterName:          KafkaClusterName,
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
		if mgh.Spec.AvailabilityConfig == operatorv1alpha4.HABasic {
			transporter.topicPartitionReplicas = 1
		}
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
	}
	// user could customize the catalog config
	catalogSourceName, ok := mgh.Annotations[operatorconstants.CatalogSourceNameKey]
	if ok && catalogSourceName != "" {
		transporter.subCatalogSourceName = catalogSourceName
	}
	catalogSourceNamespace, ok := mgh.Annotations[operatorconstants.CatalogSourceNamespaceKey]
	if ok && catalogSourceNamespace != "" {
		transporter.subCatalogSourceNamespace = catalogSourceNamespace
	}
	subscriptionChannel, ok := mgh.Annotations[operatorconstants.SubscriptionChannel]
	if ok && catalogSourceNamespace != "" {
		transporter.subChannel = subscriptionChannel
	}
	subscriptionPackageName, ok := mgh.Annotations[operatorconstants.SubscriptionPackageName]
	if ok && catalogSourceNamespace != "" {
		transporter.subPackageName = subscriptionPackageName
	}

	return transporter
}

func (k *strimziTransporter) getCurrentReplicas() (int32, error) {
	existingKafkaNodepool := &kafkav1beta2.KafkaNodePool{}
	err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
		Name:      "kraft",
		Namespace: k.mgh.Namespace,
	}, existingKafkaNodepool)
	if err != nil {
		if errors.IsNotFound(err) {
			return k.topicPartitionReplicas, nil
		}
		return k.topicPartitionReplicas, err
	}
	log.Debugf("existing kafkaNodepool replicas: %s", existingKafkaNodepool.Spec.Replicas)
	return existingKafkaNodepool.Spec.Replicas, nil
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
	log.Debug("reconcile global hub kafka transport...")
	err := k.ensureSubscription(k.mgh)
	if err != nil {
		return false, err
	}

	installed, err := k.isCSVInstalled()
	if err != nil {
		return false, err
	}

	if !installed {
		return true, nil
	}

	topicPartitionReplicas, err := k.getCurrentReplicas()
	if err != nil {
		return true, err
	}
	k.topicPartitionReplicas = topicPartitionReplicas

	// Since the kafka cluster creation need the metric configmap, render the resource before creating the cluster
	// kafka metrics, monitor, global hub kafkaTopic and kafkaUser
	err = k.renderKafkaResources(k.mgh)
	if err != nil {
		return true, err
	}

	err, _ = k.CreateUpdateKafkaCluster(k.mgh)
	if err != nil {
		return true, err
	}

	return false, nil
}

// renderKafkaMetricsResources renders the kafka podmonitor and metrics, and kafkaUser and kafkaTopic for global hub
func (k *strimziTransporter) renderKafkaResources(mgh *operatorv1alpha4.MulticlusterGlobalHub) error {
	statusTopic := config.GetRawStatusTopic()
	statusPlaceholderTopic := config.GetRawStatusTopic()
	topicPattern := kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypeLiteral
	if strings.Contains(config.GetRawStatusTopic(), "*") {
		statusTopic = strings.ReplaceAll(config.GetRawStatusTopic(), "*", "")
		statusPlaceholderTopic = strings.ReplaceAll(config.GetRawStatusTopic(), "*", "global-hub")
		topicPattern = kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourcePatternTypePrefix
	}
	topicReplicas := k.topicPartitionReplicas

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
				StatusTopicPattern     string
				StatusPlaceholderTopic string
				TopicPartition         int32
				TopicReplicas          int32
				EnableInventoryAPI     bool
				KafkaInventoryTopic    string
				StorageSize            string
				StorageClass           string
			}{
				EnableMetrics:          mgh.Spec.EnableMetrics,
				Namespace:              mgh.GetNamespace(),
				KafkaCluster:           KafkaClusterName,
				GlobalHubKafkaUser:     DefaultGlobalHubKafkaUserName,
				SpecTopic:              config.GetSpecTopic(),
				StatusTopic:            statusTopic,
				StatusTopicPattern:     string(topicPattern),
				StatusPlaceholderTopic: statusPlaceholderTopic,
				TopicPartition:         DefaultPartition,
				TopicReplicas:          topicReplicas,
				EnableInventoryAPI:     config.WithInventory(mgh),
				KafkaInventoryTopic:    "kessel-inventory",
				StorageSize:            config.GetKafkaStorageSize(mgh),
				StorageClass:           mgh.Spec.DataLayerSpec.StorageClass,
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

func (k *strimziTransporter) isCSVInstalled() (bool, error) {
	existingSub := &subv1alpha1.Subscription{}
	err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
		Name:      k.subName,
		Namespace: k.mgh.Namespace,
	}, existingSub)
	if err != nil && !errors.IsNotFound(err) {
		return false, err
	}

	if existingSub.Status.InstalledCSV == "" {
		return false, nil
	}

	kafkaCsv := &subv1alpha1.ClusterServiceVersion{}
	if err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
		Name:      existingSub.Status.InstalledCSV,
		Namespace: k.mgh.Namespace,
	}, kafkaCsv); err != nil {
		return false, err
	}

	if kafkaCsv.Status.Phase != subv1alpha1.CSVPhaseSucceeded {
		return false, nil
	}
	return true, nil
}

// EnsureUser to reconcile the kafkaUser's setting(authn and authz)
// set the user can write to status
func (k *strimziTransporter) EnsureUser(clusterName string) (string, error) {
	userName := config.GetKafkaUserName(clusterName)
	clusterTopic := k.getClusterTopic(clusterName)

	authnType := kafkav1beta2.KafkaUserSpecAuthenticationTypeTlsExternal
	if clusterName == config.GetLocalClusterName() || clusterName == constants.LocalClusterName {
		authnType = kafkav1beta2.KafkaUserSpecAuthenticationTypeTls
	}

	simpleACLs := []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
		utils.ConsumeGroupReadACL(),
		// migration resource into mh: write access to the spec topic is required for cluster migration.
		utils.GetTopicACL(clusterTopic.SpecTopic, []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemDescribe,
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemWrite,
		}),
		// report status into gh: allow the current hub to write messages to the specific status topic
		utils.GetTopicACL(clusterTopic.StatusTopic, []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
			kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemWrite,
		}),
	}

	desiredKafkaUser := newKafkaUser(k.kafkaClusterNamespace, k.kafkaClusterName, userName, authnType, simpleACLs)

	kafkaUser := &kafkav1beta2.KafkaUser{}
	err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
		Name:      userName,
		Namespace: k.kafkaClusterNamespace,
	}, kafkaUser)
	if errors.IsNotFound(err) {
		log.Infof("create the kafakUser: %s", userName)
		return userName, k.manager.GetClient().Create(k.ctx, desiredKafkaUser, &client.CreateOptions{})
	} else if err != nil {
		return "", err
	}

	// Retry logic to handle concurrent updates with exponential backoff
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get the latest version of the KafkaUser
		latestKafkaUser := &kafkav1beta2.KafkaUser{}
		if err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
			Name:      userName,
			Namespace: k.kafkaClusterNamespace,
		}, latestKafkaUser); err != nil {
			return err
		}

		updatedKafkaUser := &kafkav1beta2.KafkaUser{}
		if err := operatorutils.MergeObjects(latestKafkaUser, desiredKafkaUser, updatedKafkaUser); err != nil {
			return err
		}
		// combine the acls of kafkaUser and the acls of desiredKafkaUser
		updatedKafkaUser.Spec.Authorization.Acls = combineACLs(latestKafkaUser.Spec.Authorization.Acls,
			desiredKafkaUser.Spec.Authorization.Acls)

		if !equality.Semantic.DeepDerivative(updatedKafkaUser.Spec, latestKafkaUser.Spec) {
			log.Infof("update the kafkaUser: %s", userName)
			return k.manager.GetClient().Update(k.ctx, updatedKafkaUser)
		}
		return nil
	})

	if retryErr != nil {
		return "", retryErr
	}
	return userName, nil
}

// combineACLs combines the existing acls and the desired acls
func combineACLs(kafkaUserAcls []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem,
	desiredKafkaUserAcls []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem,
) []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem {
	// Deduplicate ACLs based on resource.name + operations
	aclMap := make(map[string]kafkav1beta2.KafkaUserSpecAuthorizationAclsElem)

	for _, acl := range kafkaUserAcls {
		key := utils.GenerateACLKey(acl)
		aclMap[key] = acl
	}
	for _, acl := range desiredKafkaUserAcls {
		key := utils.GenerateACLKey(acl)
		aclMap[key] = acl
	}

	mergedAcls := []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{}
	for _, acl := range aclMap {
		mergedAcls = append(mergedAcls, acl)
	}

	// Sort ACLs to ensure consistent ordering for comparison
	// This prevents unnecessary updates when ACLs are functionally identical but ordered differently
	sort.Slice(mergedAcls, func(i, j int) bool {
		return utils.GenerateACLKey(mergedAcls[i]) < utils.GenerateACLKey(mergedAcls[j])
	})

	return mergedAcls
}

func (k *strimziTransporter) EnsureTopic(clusterName string) (*transport.ClusterTopic, error) {
	clusterTopic := k.getClusterTopic(clusterName)

	topicNames := []string{clusterTopic.SpecTopic, clusterTopic.StatusTopic}
	for _, topicName := range topicNames {
		if err := k.ensureTopic(topicName, nil); err != nil {
			return nil, err
		}
	}
	return clusterTopic, nil
}

func (k *strimziTransporter) ensureTopic(topicName string, config *apiextensions.JSON) error {
	desiredTopic := k.newKafkaTopic(topicName, config)
	kafkaTopic := &kafkav1beta2.KafkaTopic{}
	err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
		Name:      topicName,
		Namespace: k.kafkaClusterNamespace,
	}, kafkaTopic)
	if errors.IsNotFound(err) {
		if e := k.manager.GetClient().Create(k.ctx, desiredTopic); e != nil {
			return e
		}
		return nil
	} else if err != nil {
		return err
	}

	// update the topic
	updatedTopic := &kafkav1beta2.KafkaTopic{}
	err = operatorutils.MergeObjects(kafkaTopic, desiredTopic, updatedTopic)
	if err != nil {
		return err
	}
	// Kafka do not support change exitsting kafaka topic replica directly.
	updatedTopic.Spec.Replicas = kafkaTopic.Spec.Replicas

	if !equality.Semantic.DeepDerivative(updatedTopic.Spec, kafkaTopic.Spec) {
		if err = k.manager.GetClient().Update(k.ctx, updatedTopic); err != nil {
			return err
		}
	}
	return nil
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
	// If a topic is deleted, its offset should also be removed from database.
	// Otherwise, if the topic is recreated, old offsets in db may block message consumption
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
	credential, err := k.getConnCredentialByCluster()
	if err != nil {
		return nil, err
	}

	// certificates
	credential.CASecretName = GetClusterCASecret(k.kafkaClusterName)
	credential.ClientSecretName = config.AgentCertificateSecretName()

	// topics
	credential.StatusTopic = config.GetStatusTopic(clusterName)
	credential.SpecTopic = config.GetSpecTopic()

	// consumer group id
	credential.ConsumerGroupID = config.GetConsumerGroupID(k.mgh.Spec.DataLayerSpec.Kafka.ConsumerGroupPrefix,
		clusterName)

	// for the non local-cluster
	if clusterName != config.GetLocalClusterName() && clusterName != constants.LocalClusterName {
		return credential, nil
	}

	// for local-cluster, need to load the client cert/key from the kafka user
	userName := config.GetKafkaUserName(clusterName)
	if !k.enableTLS {
		log.Infof("the kafka cluster hasn't enable tls for user", "username", userName)
		return credential, nil
	}
	if err := k.loadUserCredential(userName, credential); err != nil {
		return nil, err
	}
	return credential, nil
}

func GetClusterCASecret(clusterName string) string {
	return fmt.Sprintf("%s-cluster-ca-cert", clusterName)
}

// loadUserCredential add credential with client cert, and key
func (k *strimziTransporter) loadUserCredential(kafkaUserName string, credential *transport.KafkaConfig) error {
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

// getConnCredentialByCluster gets credential with clusterId, bootstrapServer, and serverCA
func (k *strimziTransporter) getConnCredentialByCluster() (*transport.KafkaConfig, error) {
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
				BootstrapServer: *kafkaCluster.Status.Listeners[0].BootstrapServers,
				CACert:          base64.StdEncoding.EncodeToString([]byte(kafkaCluster.Status.Listeners[0].Certificates[0])),
			}
			return credential, nil
		}
	}
	return nil, fmt.Errorf("kafka cluster %s/%s is not ready", k.kafkaClusterNamespace, k.kafkaClusterName)
}

func (k *strimziTransporter) newKafkaTopic(topicName string, topicConfig *apiextensions.JSON) *kafkav1beta2.KafkaTopic {
	topic := &kafkav1beta2.KafkaTopic{
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
	if topicConfig != nil {
		topic.Spec.Config = topicConfig
	}
	return topic
}

func newKafkaUser(
	namespace, clusterName, userName string,
	authnType kafkav1beta2.KafkaUserSpecAuthenticationType,
	simpleACLs []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem,
) *kafkav1beta2.KafkaUser {
	return &kafkav1beta2.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userName,
			Namespace: namespace,
			Labels: map[string]string{
				// It is important to set the cluster label otherwise the user will not be ready
				"strimzi.io/cluster":             clusterName,
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
func (k *strimziTransporter) kafkaClusterReady() (KafkaStatus, error) {
	kafkaCluster := &kafkav1beta2.Kafka{}

	kafkaStatus := KafkaStatus{
		kafkaReady:   false,
		kafkaReason:  "KafkaNotReady",
		kafkaMessage: "Wait kafka cluster ready",
	}

	err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
		Name:      k.kafkaClusterName,
		Namespace: k.kafkaClusterNamespace,
	}, kafkaCluster)
	if err != nil {
		log.Debugw("fail to get the kafka cluster, waiting", "message", err.Error())
		if errors.IsNotFound(err) {
			return kafkaStatus, nil
		}
		return kafkaStatus, err
	}
	if kafkaCluster.Status == nil || kafkaCluster.Status.Conditions == nil {
		return kafkaStatus, nil
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
				log.Info("kafka cluster is ready")
				kafkaStatus.kafkaReady = true
				return kafkaStatus, nil
			}
			kafkaStatus.kafkaMessage = *condition.Message
			kafkaStatus.kafkaReason = *condition.Reason
			return kafkaStatus, nil
		}
	}
	log.Info("wait for the Kafka cluster to be ready")
	return kafkaStatus, nil
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
		log.Error(err, "failed to marshal kafka resources")
	}
	err = json.Unmarshal(jsonData, kafkaSpecRes)
	if err != nil {
		log.Error(err, "failed to unmarshal to KafkaSpecKafkaResources")
	}

	return kafkaSpecRes
}

func (k *strimziTransporter) newKafkaCluster(mgh *operatorv1alpha4.MulticlusterGlobalHub) *kafkav1beta2.Kafka {
	var nodePort int32 = 30093
	listeners := []kafkav1beta2.KafkaSpecKafkaListenersElem{
		{
			Name: "tls",
			Port: 9093,
			Tls:  true,
			Type: kafkav1beta2.KafkaSpecKafkaListenersElemTypeRoute,
			Authentication: &kafkav1beta2.KafkaSpecKafkaListenersElemAuthentication{
				Type: kafkav1beta2.KafkaSpecKafkaListenersElemAuthenticationTypeTls,
			},
		},
	}

	_, exists := mgh.Annotations[operatorconstants.KafkaUseNodeport]
	if exists {
		host := mgh.Annotations[operatorconstants.KinDClusterIPKey]
		listeners[0].Configuration = &kafkav1beta2.KafkaSpecKafkaListenersElemConfiguration{
			Bootstrap: &kafkav1beta2.KafkaSpecKafkaListenersElemConfigurationBootstrap{
				NodePort: &nodePort,
			},
			Brokers: []kafkav1beta2.KafkaSpecKafkaListenersElemConfigurationBrokersElem{
				{
					Broker:         0,
					AdvertisedHost: &host,
				},
			},
		}
		listeners[0].Type = kafkav1beta2.KafkaSpecKafkaListenersElemTypeNodeport
	}

	config := ""
	if k.topicPartitionReplicas != DefaultPartitionReplicas {
		config = `{
"default.replication.factor": 1,
"min.insync.replicas": 1,
"offsets.topic.replication.factor": 1,
"transaction.state.log.min.isr": 1,
"transaction.state.log.replication.factor": 1
}`
	} else {
		config = `{
"default.replication.factor": 3,
"min.insync.replicas": 2,
"offsets.topic.replication.factor": 3,
"transaction.state.log.min.isr": 2,
"transaction.state.log.replication.factor": 3
}`
	}

	kafkaCluster := &kafkav1beta2.Kafka{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k.kafkaClusterName,
			Namespace: k.kafkaClusterNamespace,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GlobalHubOwnerLabelVal,
			},
			Annotations: map[string]string{
				"strimzi.io/node-pools": "enabled",
				"strimzi.io/kraft":      "enabled",
			},
		},
		Spec: &kafkav1beta2.KafkaSpec{
			Kafka: kafkav1beta2.KafkaSpecKafka{
				Config:    &apiextensions.JSON{Raw: []byte(config)},
				Listeners: listeners,
				Resources: k.getKafkaResources(mgh),
				Authorization: &kafkav1beta2.KafkaSpecKafkaAuthorization{
					Type: kafkav1beta2.KafkaSpecKafkaAuthorizationTypeSimple,
				},
				Version: &DefaultAMQKafkaVersion,
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
	if mgh.Spec.EnableMetrics {
		kafkaMetricsConfig := &kafkav1beta2.KafkaSpecKafkaMetricsConfig{
			Type: kafkav1beta2.KafkaSpecKafkaMetricsConfigTypeJmxPrometheusExporter,
			ValueFrom: kafkav1beta2.KafkaSpecKafkaMetricsConfigValueFrom{
				ConfigMapKeyRef: &kafkav1beta2.KafkaSpecKafkaMetricsConfigValueFromConfigMapKeyRef{
					Name: &KafkaMetricsConfigMapName,
					Key:  &KafkaMetricsConfigMapKeyRef,
				},
			},
		}
		kafkaCluster.Spec.Kafka.MetricsConfig = kafkaMetricsConfig
	}
}

// set affinity for kafka cluster based on the mgh nodeSelector
func (k *strimziTransporter) setAffinity(mgh *operatorv1alpha4.MulticlusterGlobalHub,
	kafkaCluster *kafkav1beta2.Kafka,
) {
	kafkaPodAffinity := &kafkav1beta2.KafkaSpecKafkaTemplatePodAffinity{}
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
			log.Error(err)
		}

		kafkaNodeSelectorTermsElem := make([]kafkav1beta2.
			KafkaSpecKafkaTemplatePodAffinityNodeAffinityRequiredDuringSchedulingIgnoredDuringExecutionNodeSelectorTermsElem,
			0)

		entityOperatorNodeSelectorTermsElem := make([]kafkav1beta2.
			KafkaSpecEntityOperatorTemplatePodAffinityNodeAffinityRequiredDuringSchedulingIgnoredDuringExecutionNodeSelectorTermsElem, 0)

		err = json.Unmarshal(jsonData, &kafkaNodeSelectorTermsElem)
		if err != nil {
			log.Error("failed to unmarshal to kafkaNodeSelectorTermsElem: ", err)
		}
		err = json.Unmarshal(jsonData, &entityOperatorNodeSelectorTermsElem)
		if err != nil {
			log.Error("failed to unmarshal to entityOperatorNodeSelectorTermsElem: ", err)
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
			kafkaCluster.Spec.EntityOperator.Template = &kafkav1beta2.KafkaSpecEntityOperatorTemplate{
				Pod: &kafkav1beta2.KafkaSpecEntityOperatorTemplatePod{
					Affinity: entityOperatorPodAffinity,
				},
			}
		} else {
			kafkaCluster.Spec.Kafka.Template.Pod.Affinity = kafkaPodAffinity
			kafkaCluster.Spec.EntityOperator.Template.Pod.Affinity = entityOperatorPodAffinity
		}
	}
}

// setTolerations sets the kafka tolerations based on the mgh tolerations
func (k *strimziTransporter) setTolerations(mgh *operatorv1alpha4.MulticlusterGlobalHub,
	kafkaCluster *kafkav1beta2.Kafka,
) {
	kafkaTolerationsElem := make([]kafkav1beta2.KafkaSpecKafkaTemplatePodTolerationsElem, 0)
	entityOperatorTolerationsElem := make([]kafkav1beta2.KafkaSpecEntityOperatorTemplatePodTolerationsElem, 0)

	if mgh.Spec.Tolerations != nil {
		jsonData, err := json.Marshal(mgh.Spec.Tolerations)
		if err != nil {
			log.Error("failed to marshal tolerations: ", err)
		}
		err = json.Unmarshal(jsonData, &kafkaTolerationsElem)
		if err != nil {
			log.Error("failed to unmarshal to KafkaSpecruntimeKafkaTemplatePodTolerationsElem: ", err)
		}
		err = json.Unmarshal(jsonData, &entityOperatorTolerationsElem)
		if err != nil {
			log.Error("failed to unmarshal to KafkaSpecEntityOperatorTemplatePodTolerationsElem: ", err)
		}

		if kafkaCluster.Spec.Kafka.Template == nil {
			kafkaCluster.Spec.Kafka.Template = &kafkav1beta2.KafkaSpecKafkaTemplate{
				Pod: &kafkav1beta2.KafkaSpecKafkaTemplatePod{
					Tolerations: kafkaTolerationsElem,
				},
			}
			kafkaCluster.Spec.EntityOperator.Template = &kafkav1beta2.KafkaSpecEntityOperatorTemplate{
				Pod: &kafkav1beta2.KafkaSpecEntityOperatorTemplatePod{
					Tolerations: entityOperatorTolerationsElem,
				},
			}
		} else {
			kafkaCluster.Spec.Kafka.Template.Pod.Tolerations = kafkaTolerationsElem
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
		// marshal to json
		existingKafkaJson, _ := json.Marshal(existingKafkaSpec)
		desiredKafkaJson, _ := json.Marshal(desiredKafkaSpec)

		// patch the desired kafka cluster to the existing kafka cluster
		patchedData, err := jsonpatch.MergePatch(existingKafkaJson, desiredKafkaJson)
		if err != nil {
			log.Error("failed to merge patch: ", err)
			return
		}

		updatedKafkaSpec := &kafkav1beta2.KafkaSpec{}
		err = json.Unmarshal(patchedData, updatedKafkaSpec)
		if err != nil {
			log.Error("failed to umarshal kafkaspec: ", err)
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
	}

	// Use retry to handle conflict errors during update
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Re-fetch the subscription to get the latest version
		if err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
			Name:      k.subName,
			Namespace: mgh.GetNamespace(),
		}, existingSub); err != nil {
			return err
		}

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
	})
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
