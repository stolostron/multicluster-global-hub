// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package syncers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	klusterletConfigNamePrefix = "migration-"
	bootstrapSecretNamePrefix  = "bootstrap-"
	KlusterletConfigAnnotation = "agent.open-cluster-management.io/klusterlet-config"
)

// This is a temporary solution to wait for applying the klusterletconfig
var sleepForApplying = 20 * time.Second

type managedClusterMigrationFromSyncer struct {
	log               *zap.SugaredLogger
	client            client.Client
	transportClient   transport.TransportClient
	transportConfig   *transport.TransportInternalConfig
	migrationProducer *producer.GenericProducer
	bundleVersion     *eventversion.Version
	sendResources     bool
}

func NewManagedClusterMigrationFromSyncer(client client.Client,
	transportClient transport.TransportClient, transportConfig *transport.TransportInternalConfig,
) *managedClusterMigrationFromSyncer {
	return &managedClusterMigrationFromSyncer{
		log:             logger.DefaultZapLogger(),
		client:          client,
		transportClient: transportClient,
		transportConfig: transportConfig,
		bundleVersion:   eventversion.NewVersion(),
		sendResources:   false,
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
	s.log.Debugf("received managed cluster migration event %s", string(payload))
	// initialize the gh-migration producer
	if s.migrationProducer == nil && s.transportConfig != nil {
		var err error
		s.migrationProducer, err = producer.NewGenericProducer(s.transportConfig,
			s.transportConfig.KafkaCredential.MigrationTopic)
		if err != nil {
			return err
		}
	}

	// expected initialized
	if migrationSourceHubEvent.Stage == migrationv1alpha1.ConditionTypeInitialized {
		if migrationSourceHubEvent.BootstrapSecret == nil {
			return fmt.Errorf("bootstrap secret is nil when initializing")
		}
		// attach klusterletconfig(with bootstrap kubeconfig secret) to managed clusters
		if err := s.attachBootstrapKubeConfigToClusters(ctx, migrationSourceHubEvent); err != nil {
			return err
		}

		// send the initialized confirmation
		return SendMigrationEvent(ctx, s.transportClient,
			configs.GetLeafHubName(),
			constants.CloudEventGlobalHubClusterName,
			&migration.ManagedClusterMigrationBundle{
				Stage: migrationv1alpha1.ConditionTypeInitialized,
			},
			s.bundleVersion)
	}

	s.log.Infof("initializing managed cluster migration event")
	managedClusters := migrationSourceHubEvent.ManagedClusters
	toHub := migrationSourceHubEvent.ToHub
	id := migrationSourceHubEvent.MigrationId
	err := s.SendSourceClusterMigrationResources(ctx, id, managedClusters, configs.GetLeafHubName(), toHub)
	if err != nil {
		return err
	}

	// expected registered
	if migrationSourceHubEvent.Stage == migrationv1alpha1.ConditionTypeRegistered {
		s.log.Infof("registering managed cluster migration")
		if err := s.registering(ctx, migrationSourceHubEvent); err != nil {
			return err
		}
		return nil
	}

	// expected completed that means need to clean up resources from the source hub, and send the confirmation
	if migrationSourceHubEvent.Stage == migrationv1alpha1.ConditionTypeCleaned {
		s.log.Infof("completed managed cluster migration")
		if err := s.cleanup(ctx, migrationSourceHubEvent); err != nil {
			return err
		}
		// send the cleanup confirmation
		return SendMigrationEvent(ctx, s.transportClient, configs.GetLeafHubName(), migrationSourceHubEvent.ToHub,
			&migration.ManagedClusterMigrationBundle{
				Stage:           migrationv1alpha1.ConditionTypeCleaned,
				ManagedClusters: migrationSourceHubEvent.ManagedClusters,
			},
			s.bundleVersion)
	}
	return nil
}

func (m *managedClusterMigrationFromSyncer) cleanup(
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
			m.log.Infof("bootstrap secret %s is removed", bootstrapSecret.GetName())
		} else {
			return err
		}
	} else {
		m.log.Infof("delete bootstrap secret %s", bootstrapSecret.GetName())
		if err := m.client.Delete(ctx, bootstrapSecret); err != nil {
			return err
		}
	}

	// dleete klusterletconfig
	klusterletConfig := &klusterletv1alpha1.KlusterletConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: klusterletConfigNamePrefix + migratingEvt.ToHub,
		},
	}
	if err := m.client.Get(ctx, client.ObjectKeyFromObject(klusterletConfig), klusterletConfig); err != nil {
		if apierrors.IsNotFound(err) {
			m.log.Infof("klusterletConfig %s is removed", klusterletConfig.GetName())
		} else {
			return err
		}
	} else {
		m.log.Infof("delete klusterletconfig secret %s", klusterletConfig.GetName())
		if err := m.client.Delete(ctx, klusterletConfig); err != nil {
			return err
		}
	}

	m.log.Infof("detach clusters %v", migratingEvt.ManagedClusters)
	if err := m.detachManagedClusters(ctx, migratingEvt.ManagedClusters); err != nil {
		m.log.Errorf("failed to detach managed clusters: %v", err)
		return err
	}
	// TODO: need to check how to stop the producer gracefully
	m.migrationProducer = nil
	m.sendResources = false
	return nil
}

func (m *managedClusterMigrationFromSyncer) attachBootstrapKubeConfigToClusters(
	ctx context.Context, migratingEvt *migration.ManagedClusterMigrationFromEvent,
) error {
	bootstrapSecret := migratingEvt.BootstrapSecret
	// ensure bootstrap kubeconfig secret
	foundBootstrapSecret := &corev1.Secret{}
	if err := m.client.Get(ctx,
		types.NamespacedName{
			Name:      bootstrapSecret.Name,
			Namespace: bootstrapSecret.Namespace,
		}, foundBootstrapSecret); err != nil {
		if apierrors.IsNotFound(err) {
			m.log.Infof("creating bootstrap secret %s", bootstrapSecret.GetName())
			if err := m.client.Create(ctx, bootstrapSecret); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		// update the bootstrap secret if it already exists
		m.log.Infof("updating bootstrap secret %s", bootstrapSecret.GetName())
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
	return nil
}

func (m *managedClusterMigrationFromSyncer) registering(
	ctx context.Context, migratingEvt *migration.ManagedClusterMigrationFromEvent,
) error {
	bootstrapSecret := migratingEvt.BootstrapSecret
	// ensure bootstrap kubeconfig secret
	foundBootstrapSecret := &corev1.Secret{}
	if err := m.client.Get(ctx,
		types.NamespacedName{
			Name:      bootstrapSecret.Name,
			Namespace: bootstrapSecret.Namespace,
		}, foundBootstrapSecret); err != nil {
		if apierrors.IsNotFound(err) {
			m.log.Infof("creating bootstrap secret %s", bootstrapSecret.GetName())
			if err := m.client.Create(ctx, bootstrapSecret); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		// update the bootstrap secret if it already exists
		m.log.Infof("updating bootstrap secret %s", bootstrapSecret.GetName())
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

	// ensure the bootstrap secret is propagated into the managed cluster
	time.Sleep(sleepForApplying)

	// set the hub accept client into false to trigger the re-registering
	for _, managedCluster := range managedClusters {
		mc := &clusterv1.ManagedCluster{}
		if err := m.client.Get(ctx, types.NamespacedName{
			Name: managedCluster,
		}, mc); err != nil {
			return err
		}
		mc.Spec.HubAcceptsClient = false
		m.log.Infof("updating managedcluster %s to set HubAcceptsClient as false", mc.Name)
		if err := m.client.Update(ctx, mc); err != nil {
			return err
		}
	}

	return nil
}

func (s *managedClusterMigrationFromSyncer) getKlusterletAddonConfig(ctx context.Context,
	managedCluster string,
) (*addonv1.KlusterletAddonConfig, error) {
	config := &addonv1.KlusterletAddonConfig{
		ObjectMeta: metav1.ObjectMeta{Name: managedCluster, Namespace: managedCluster},
	}
	// send klusterletAddonConfig to global hub so that it can be transferred to the target cluster
	if err := s.client.Get(ctx, client.ObjectKeyFromObject(config), config); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		log.Infof("klusterletAddonConfig %s doesn't exist", managedCluster)
	}
	// do cleanup
	config.SetManagedFields(nil)
	config.SetFinalizers(nil)
	config.SetOwnerReferences(nil)
	config.SetSelfLink("")
	config.SetResourceVersion("")
	config.SetGeneration(0)
	config.Status = addonv1.KlusterletAddonConfigStatus{}

	return config, nil
}

// SendSourceClusterMigrationResources sends required and customized resources to migration topic
func (s *managedClusterMigrationFromSyncer) SendSourceClusterMigrationResources(ctx context.Context,
	migrationId string, managedClusters []string, fromHub, toHub string,
) error {
	// aovid duplicate sending
	if s.sendResources {
		return nil
	}
	// send the managed cluster and klusterletAddonConfig to the target cluster
	migrationResources := &migration.SourceClusterMigrationResources{
		ManagedClusters:       []clusterv1.ManagedCluster{},
		KlusterletAddonConfig: []addonv1.KlusterletAddonConfig{},
	}
	// list the managed clusters
	managedClusterList := &clusterv1.ManagedClusterList{}
	if err := s.client.List(ctx, managedClusterList, &client.ListOptions{}); err != nil {
		return err
	}
	// list the klusterletAddonConfigs
	klusterletAddonConfigList := &addonv1.KlusterletAddonConfigList{}
	if err := s.client.List(ctx, klusterletAddonConfigList, &client.ListOptions{}); err != nil {
		return err
	}

	for _, managedCluster := range managedClusters {
		for _, mc := range managedClusterList.Items {
			if mc.Name == managedCluster {
				// do cleanup
				mc.SetManagedFields(nil)
				mc.SetFinalizers(nil)
				mc.SetOwnerReferences(nil)
				mc.SetSelfLink("")
				mc.SetResourceVersion("")
				mc.SetGeneration(0)
				mc.Spec.ManagedClusterClientConfigs = nil
				mc.Status = clusterv1.ManagedClusterStatus{}
				migrationResources.ManagedClusters = append(migrationResources.ManagedClusters, mc)
				continue
			}
		}
		for _, config := range klusterletAddonConfigList.Items {
			if config.Name == managedCluster && config.Namespace == managedCluster {
				// do cleanup
				config.SetManagedFields(nil)
				config.SetFinalizers(nil)
				config.SetOwnerReferences(nil)
				config.SetSelfLink("")
				config.SetResourceVersion("")
				config.SetGeneration(0)
				config.Status = addonv1.KlusterletAddonConfigStatus{}
				migrationResources.KlusterletAddonConfig = append(migrationResources.KlusterletAddonConfig, config)
				continue
			}
		}
	}

	payloadBytes, err := json.Marshal(migrationResources)
	if err != nil {
		return fmt.Errorf("failed to marshal SourceClusterMigrationResources (%v) - %w", migrationResources, err)
	}

	s.bundleVersion.Incr()
	e := utils.ToCloudEvent(string(enum.MigrationResourcesType), fromHub, toHub, payloadBytes)
	e.SetID(migrationId)
	e.SetExtension(eventversion.ExtVersion, s.bundleVersion.String())
	s.log.Info("send the migration resources to the migration topic from the source cluster")
	if err := s.migrationProducer.SendEvent(ctx, e); err != nil {
		return fmt.Errorf("failed to send event(%s) from %s to %s: %v",
			string(enum.MigrationResourcesType), fromHub, toHub, err)
	}
	s.bundleVersion.Next()
	// set the sendResources to true to avoid duplicate sending
	s.sendResources = true
	return nil
}

func SendMigrationEvent(
	ctx context.Context,
	transportClient transport.TransportClient,
	source string,
	clusterName string,
	migrationBundle *migration.ManagedClusterMigrationBundle,
	version *eventversion.Version,
) error {
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
