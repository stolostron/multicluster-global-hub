// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package syncers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	cecontext "github.com/cloudevents/sdk-go/v2/context"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	klusterletConfigNamePrefix = "migration-"
	bootstrapSecretNamePrefix  = "bootstrap-"
	KlusterletConfigAnnotation = "agent.open-cluster-management.io/klusterlet-config"
)

type managedClusterMigrationFromSyncer struct {
	client            client.Client
	transportClient   transport.TransportClient
	transportConfig   *transport.TransportInternalConfig
	migrationProducer *producer.GenericProducer
	bundleVersion     *eventversion.Version
}

func NewManagedClusterMigrationFromSyncer(client client.Client,
	transportClient transport.TransportClient, transportConfig *transport.TransportInternalConfig,
) *managedClusterMigrationFromSyncer {
	return &managedClusterMigrationFromSyncer{
		client:          client,
		transportClient: transportClient,
		transportConfig: transportConfig,
		bundleVersion:   eventversion.NewVersion(),
	}
}

func (s *managedClusterMigrationFromSyncer) SetMigrationProducer(producer *producer.GenericProducer) {
	s.migrationProducer = producer
}

func (s *managedClusterMigrationFromSyncer) Sync(ctx context.Context, payload []byte) error {
	// handle migration.from cloud event
	migrationSourceHubEvent := &migration.ManagedClusterMigrationFromEvent{}
	if err := json.Unmarshal(payload, migrationSourceHubEvent); err != nil {
		return err
	}
	log.Debugf("received managed cluster migration event %s", string(payload))

	if migrationSourceHubEvent.Stage == migrationv1alpha1.PhaseInitializing {
		if err := s.initializing(ctx, migrationSourceHubEvent); err != nil {
			return err
		}

		if err := s.deploying(ctx, migrationSourceHubEvent); err != nil {
			return err
		}
	}

	if migrationSourceHubEvent.Stage == migrationv1alpha1.PhaseRegistering {
		log.Infof("registering managed cluster migration")
		if err := s.registering(ctx, migrationSourceHubEvent); err != nil {
			return err
		}
	}

	if migrationSourceHubEvent.Stage == migrationv1alpha1.PhaseCleaning {
		log.Infof("cleaning managed cluster migration")
		if err := s.cleaning(ctx, migrationSourceHubEvent); err != nil {
			return err
		}
	}
	return nil
}

func (m *managedClusterMigrationFromSyncer) cleaning(
	ctx context.Context, migratingEvt *migration.ManagedClusterMigrationFromEvent,
) error {
	bootstrapSecret := migratingEvt.BootstrapSecret
	// delete bootstrap kubeconfig secret
	foundBootstrapSecret := &corev1.Secret{}
	if err := m.client.Get(ctx,
		types.NamespacedName{
			Name:      bootstrapSecret.Name,
			Namespace: bootstrapSecret.Namespace,
		}, foundBootstrapSecret); err != nil {
		if apierrors.IsNotFound(err) {
			log.Infof("bootstrap secret %s is removed", bootstrapSecret.GetName())
		} else {
			return err
		}
	} else {
		log.Infof("delete bootstrap secret %s", bootstrapSecret.GetName())
		if err := m.client.Delete(ctx, bootstrapSecret); err != nil {
			return err
		}
	}

	// delete klusterletconfig
	klusterletConfig := &klusterletv1alpha1.KlusterletConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: klusterletConfigNamePrefix + migratingEvt.ToHub,
		},
	}
	if err := m.client.Get(ctx, client.ObjectKeyFromObject(klusterletConfig), klusterletConfig); err != nil {
		if apierrors.IsNotFound(err) {
			log.Infof("klusterletConfig %s is removed", klusterletConfig.GetName())
		} else {
			return err
		}
	} else {
		log.Infof("delete klusterletconfig secret %s", klusterletConfig.GetName())
		if err := m.client.Delete(ctx, klusterletConfig); err != nil {
			return err
		}
	}

	log.Infof("detach clusters %v", migratingEvt.ManagedClusters)
	if err := m.detachManagedClusters(ctx, migratingEvt.ManagedClusters); err != nil {
		log.Errorf("failed to detach managed clusters: %v", err)
		return err
	}
	if m.migrationProducer != nil && m.migrationProducer.Protocol() != nil {
		if err := m.migrationProducer.Protocol().Close(ctx); err != nil {
			log.Errorf("failed to close producer: %v", err)
		}
		m.migrationProducer = nil
	}

	// send the cleanup confirmation
	err := ReportMigrationStatus(ctx, m.transportClient,
		&migration.ManagedClusterMigrationBundle{
			MigrationId: migratingEvt.MigrationId,
			Stage:       migrationv1alpha1.ConditionTypeCleaned,
		},
		m.bundleVersion)
	if err != nil {
		return err
	}
	log.Info("migration cleaning up is finished")
	return nil
}

// deploying: send clusters and addon config into target hub
func (s *managedClusterMigrationFromSyncer) deploying(
	ctx context.Context, migratingEvt *migration.ManagedClusterMigrationFromEvent,
) error {
	if s.migrationProducer == nil && s.transportConfig != nil {
		var err error
		s.migrationProducer, err = producer.NewGenericProducer(s.transportConfig,
			s.transportConfig.KafkaCredential.MigrationTopic, s.resendMigrationResources)
		if err != nil {
			return err
		}
	}

	managedClusters := migratingEvt.ManagedClusters
	toHub := migratingEvt.ToHub
	id := migratingEvt.MigrationId
	err := s.SendSourceClusterMigrationResources(ctx, id, managedClusters, configs.GetLeafHubName(), toHub)
	if err != nil {
		return err
	}
	return nil
}

// initializing: attach klusterletconfig(with bootstrap kubeconfig secret) to managed clusters
func (m *managedClusterMigrationFromSyncer) initializing(
	ctx context.Context, migratingEvt *migration.ManagedClusterMigrationFromEvent,
) error {
	if migratingEvt.BootstrapSecret == nil {
		return fmt.Errorf("bootstrap secret is nil when initializing")
	}

	bootstrapSecret := migratingEvt.BootstrapSecret
	// ensure bootstrap kubeconfig secret
	foundBootstrapSecret := &corev1.Secret{}
	if err := m.client.Get(ctx,
		types.NamespacedName{
			Name:      bootstrapSecret.Name,
			Namespace: bootstrapSecret.Namespace,
		}, foundBootstrapSecret); err != nil {
		if apierrors.IsNotFound(err) {
			log.Infof("creating bootstrap secret %s", bootstrapSecret.GetName())
			if err := m.client.Create(ctx, bootstrapSecret); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		// update the bootstrap secret if it already exists
		log.Infof("updating bootstrap secret %s", bootstrapSecret.GetName())
		if err := m.client.Update(ctx, bootstrapSecret); err != nil {
			return err
		}
	}

	// ensure klusterletconfig
	klusterletConfig := &klusterletv1alpha1.KlusterletConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: klusterletConfigNamePrefix + migratingEvt.ToHub,
		},
		Spec: klusterletv1alpha1.KlusterletConfigSpec{
			BootstrapKubeConfigs: operatorv1.BootstrapKubeConfigs{
				Type: operatorv1.LocalSecrets,
				LocalSecrets: operatorv1.LocalSecretsConfig{
					KubeConfigSecrets: []operatorv1.KubeConfigSecret{
						{
							Name: bootstrapSecret.Name,
						},
					},
				},
			},
		},
	}
	if err := m.client.Get(ctx, client.ObjectKeyFromObject(klusterletConfig), klusterletConfig); err != nil {
		if apierrors.IsNotFound(err) {
			if err := m.client.Create(ctx, klusterletConfig); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	containBootstrapSecret := false
	kubeConfigSecrets := klusterletConfig.Spec.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets
	for _, kubeConfigSecret := range kubeConfigSecrets {
		if kubeConfigSecret.Name == bootstrapSecret.Name {
			containBootstrapSecret = true
		}
	}
	if !containBootstrapSecret {
		klusterletConfig.Spec.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets = append(kubeConfigSecrets,
			operatorv1.KubeConfigSecret{Name: bootstrapSecret.Name})
		if err := m.client.Update(ctx, klusterletConfig); err != nil {
			return err
		}
	}

	// update managed cluster annotations to point to the new klusterletconfig
	managedClusters := migratingEvt.ManagedClusters
	for _, managedCluster := range managedClusters {
		mc := &clusterv1.ManagedCluster{}
		if err := m.client.Get(ctx, types.NamespacedName{
			Name: managedCluster,
		}, mc); err != nil {
			return err
		}
		annotations := mc.Annotations
		if annotations == nil {
			annotations = make(map[string]string)
		}

		_, migrating := annotations[constants.ManagedClusterMigrating]
		if migrating && annotations[KlusterletConfigAnnotation] == klusterletConfig.Name {
			continue
		}
		annotations[KlusterletConfigAnnotation] = klusterletConfig.Name
		annotations[constants.ManagedClusterMigrating] = ""
		mc.SetAnnotations(annotations)
		if err := m.client.Update(ctx, mc); err != nil {
			return err
		}
	}

	// send the initialized confirmation
	err := ReportMigrationStatus(
		cecontext.WithTopic(ctx, m.transportConfig.KafkaCredential.StatusTopic), m.transportClient,
		&migration.ManagedClusterMigrationBundle{
			MigrationId: migratingEvt.MigrationId,
			Stage:       migrationv1alpha1.ConditionTypeInitialized,
		},
		m.bundleVersion)
	if err != nil {
		return err
	}
	return nil
}

func (m *managedClusterMigrationFromSyncer) registering(
	ctx context.Context, migratingEvt *migration.ManagedClusterMigrationFromEvent,
) error {
	managedClusters := migratingEvt.ManagedClusters
	// set the hub accept client into false to trigger the re-registering
	for _, managedCluster := range managedClusters {
		mc := &clusterv1.ManagedCluster{}
		if err := m.client.Get(ctx, types.NamespacedName{
			Name: managedCluster,
		}, mc); err != nil {
			return err
		}
		mc.Spec.HubAcceptsClient = false
		log.Infof("updating managedcluster %s to set HubAcceptsClient as false", mc.Name)
		if err := m.client.Update(ctx, mc); err != nil {
			return err
		}
	}

	return nil
}

func (s *managedClusterMigrationFromSyncer) resendMigrationResources(event *kafka.Message) {
	log.Debug("resend the migration resources due to topicPartition error")
	if err := wait.PollUntilContextCancel(context.TODO(), 5*time.Second, false,
		func(context.Context) (bool, error) {
			err := s.migrationProducer.KafkaProducer().Produce(event, nil)
			if err != nil {
				log.Debugf("failed to resend the migration resources due to %v", err)
				return false, nil
			}
			return true, nil
		}); err != nil {
		log.Errorf("failed to resend the migration resources due to %v", err)
	}
}

// SendSourceClusterMigrationResources sends required and customized resources to migration topic
func (s *managedClusterMigrationFromSyncer) SendSourceClusterMigrationResources(ctx context.Context,
	migrationId string, managedClusters []string, fromHub, toHub string,
) error {
	// send the managed cluster and klusterletAddonConfig to the target cluster
	migrationResources := &migration.SourceClusterMigrationResources{
		ManagedClusters:       []clusterv1.ManagedCluster{},
		KlusterletAddonConfig: []addonv1.KlusterletAddonConfig{},
	}

	for _, managedCluster := range managedClusters {
		// add cluster
		cluster := &clusterv1.ManagedCluster{}
		err := s.client.Get(ctx, types.NamespacedName{Name: managedCluster}, cluster)
		if err != nil {
			return err
		}
		cluster.SetManagedFields(nil)
		cluster.SetFinalizers(nil)
		cluster.SetOwnerReferences(nil)
		cluster.SetSelfLink("")
		cluster.SetResourceVersion("")
		cluster.SetGeneration(0)
		cluster.Spec.ManagedClusterClientConfigs = nil
		cluster.Status = clusterv1.ManagedClusterStatus{}
		migrationResources.ManagedClusters = append(migrationResources.ManagedClusters, *cluster)

		// add addon config
		addonConfig := &addonv1.KlusterletAddonConfig{}
		err = s.client.Get(ctx, types.NamespacedName{Name: managedCluster, Namespace: managedCluster}, addonConfig)
		if err != nil {
			return err
		}
		addonConfig.SetManagedFields(nil)
		addonConfig.SetFinalizers(nil)
		addonConfig.SetOwnerReferences(nil)
		addonConfig.SetSelfLink("")
		addonConfig.SetResourceVersion("")
		addonConfig.SetGeneration(0)
		addonConfig.Status = addonv1.KlusterletAddonConfigStatus{}
		migrationResources.KlusterletAddonConfig = append(migrationResources.KlusterletAddonConfig, *addonConfig)
	}

	payloadBytes, err := json.Marshal(migrationResources)
	if err != nil {
		return fmt.Errorf("failed to marshal SourceClusterMigrationResources (%v) - %w", migrationResources, err)
	}

	s.bundleVersion.Incr()
	e := utils.ToCloudEvent(string(enum.MigrationResourcesType), fromHub, toHub, payloadBytes)
	e.SetID(migrationId)
	e.SetExtension(eventversion.ExtVersion, s.bundleVersion.String())
	log.Info("send the migration resources to the migration topic from the source cluster")
	if err := s.migrationProducer.SendEvent(ctx, e); err != nil {
		return fmt.Errorf("failed to send event(%s) from %s to %s: %v",
			string(enum.MigrationResourcesType), fromHub, toHub, err)
	}
	s.bundleVersion.Next()
	return nil
}

func ReportMigrationStatus(
	ctx context.Context,
	transportClient transport.TransportClient,
	migrationBundle *migration.ManagedClusterMigrationBundle,
	version *eventversion.Version,
) error {
	source := configs.GetLeafHubName()
	clusterName := constants.CloudEventGlobalHubClusterName
	payloadBytes, err := json.Marshal(migrationBundle)
	if err != nil {
		return fmt.Errorf("failed to marshal ManagedClusterMigrationBundle %w", err)
	}

	version.Incr()
	eventType := string(enum.ManagedClusterMigrationType)
	e := utils.ToCloudEvent(eventType, source, clusterName, payloadBytes)
	e.SetExtension(eventversion.ExtVersion, version.String())
	if transportClient != nil {
		if err := transportClient.GetProducer().SendEvent(ctx, e); err != nil {
			return fmt.Errorf("failed to send event(%s) from %s to %s: %v", eventType, source, clusterName, err)
		}
		version.Next()
		return nil
	}
	return errors.New("transport client must not be nil")
}

func SendEvent(
	ctx context.Context,
	transportClient transport.TransportClient,
	eventType string,
	source string,
	clusterName string,
	payloadBytes []byte,
	version *eventversion.Version,
) error {
	version.Incr()
	e := utils.ToCloudEvent(eventType, source, clusterName, payloadBytes)
	e.SetExtension(eventversion.ExtVersion, version.String())
	if transportClient != nil {
		if err := transportClient.GetProducer().SendEvent(ctx, e); err != nil {
			return fmt.Errorf("failed to send event(%s) from %s to %s: %v", eventType, source, clusterName, err)
		}
		version.Next()
		return nil
	}
	return errors.New("transport client must not be nil")
}

func (s *managedClusterMigrationFromSyncer) detachManagedClusters(ctx context.Context, managedClusters []string) error {
	for _, managedCluster := range managedClusters {
		mc := &clusterv1.ManagedCluster{}
		if err := s.client.Get(ctx, types.NamespacedName{
			Name: managedCluster,
		}, mc); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			} else {
				return err
			}
		}
		if !mc.Spec.HubAcceptsClient {
			if err := s.client.Delete(ctx, mc); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				} else {
					return err
				}
			}
		}
	}
	return nil
}
