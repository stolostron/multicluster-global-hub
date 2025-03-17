// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package syncers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
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
	bundleevent "github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	klusterletConfigNamePrefix = "migration-"
	bootstrapSecretNamePrefix  = "bootstrap-"
)

// This is a temporary solution to wait for applying the klusterletconfig
var sleepForApplying = 20 * time.Second

type managedClusterMigrationFromSyncer struct {
	log             *zap.SugaredLogger
	client          client.Client
	transportClient transport.TransportClient
	bundleVersion   *eventversion.Version
}

func NewManagedClusterMigrationFromSyncer(client client.Client,
	transportClient transport.TransportClient,
) *managedClusterMigrationFromSyncer {
	return &managedClusterMigrationFromSyncer{
		log:             logger.ZapLogger("managed-cluster-migration-from-syncer"),
		client:          client,
		transportClient: transportClient,
		bundleVersion:   eventversion.NewVersion(),
	}
}

func (s *managedClusterMigrationFromSyncer) Sync(ctx context.Context, payload []byte) error {
	// handle migration.from cloud event
	managedClusterMigrationEvent := &bundleevent.ManagedClusterMigrationFromEvent{}
	if err := json.Unmarshal(payload, managedClusterMigrationEvent); err != nil {
		return err
	}
	s.log.Debugf("received managed cluster migration event %s", string(payload))

	if managedClusterMigrationEvent.Stage == migrationv1alpha1.PhaseInitializing {
		managedClusters := managedClusterMigrationEvent.ManagedClusters
		toHub := managedClusterMigrationEvent.ToHub
		for _, managedCluster := range managedClusters {
			if err := s.sendKlusterletAddonConfig(ctx, managedCluster, toHub); err != nil {
				return err
			}
		}
		return nil
	}

	if managedClusterMigrationEvent.Stage == migrationv1alpha1.PhaseMigrating {
		if err := s.registering(ctx, managedClusterMigrationEvent); err != nil {
			return err
		}
		return nil
	}

	// TODO: deprecated in the migrating stage
	// create or update bootstrap secret
	bootstrapSecret := managedClusterMigrationEvent.BootstrapSecret
	foundBootstrapSecret := &corev1.Secret{}
	if err := s.client.Get(ctx,
		types.NamespacedName{
			Name:      bootstrapSecret.Name,
			Namespace: bootstrapSecret.Namespace,
		}, foundBootstrapSecret); err != nil {
		if apierrors.IsNotFound(err) {
			s.log.Infof("creating bootstrap secret %s", bootstrapSecret.GetName())
			if err := s.client.Create(ctx, bootstrapSecret); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		// update the bootstrap secret if it already exists
		s.log.Infof("updating bootstrap secret %s", bootstrapSecret.GetName())
		if err := s.client.Update(ctx, bootstrapSecret); err != nil {
			return err
		}
	}

	// create klusterlet config if it does not exist
	klusterletConfig := managedClusterMigrationEvent.KlusterletConfig
	// set the bootstrap kubeconfig secrets in klusterlet config
	klusterletConfig.Spec.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets = []operatorv1.KubeConfigSecret{
		{
			Name: bootstrapSecret.Name,
		},
	}
	foundKlusterletConfig := &klusterletv1alpha1.KlusterletConfig{}
	if err := s.client.Get(ctx,
		types.NamespacedName{
			Name: klusterletConfig.Name,
		}, foundKlusterletConfig); err != nil {
		if apierrors.IsNotFound(err) {
			s.log.Infof("creating klusterlet config %s", klusterletConfig.GetName())
			s.log.Debugf("creating klusterlet config %v", klusterletConfig)
			if err := s.client.Create(ctx, klusterletConfig); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	managedClusters := managedClusterMigrationEvent.ManagedClusters
	// update managed cluster annotations to point to the new klusterletconfig
	for _, managedCluster := range managedClusters {
		mc := &clusterv1.ManagedCluster{}
		if err := s.client.Get(ctx, types.NamespacedName{
			Name: managedCluster,
		}, mc); err != nil {
			return err
		}
		annotations := mc.Annotations
		if annotations == nil {
			annotations = make(map[string]string)
		}

		_, migrating := annotations[constants.ManagedClusterMigrating]
		if migrating && annotations["agent.open-cluster-management.io/klusterlet-config"] == klusterletConfig.Name {
			continue
		}
		annotations["agent.open-cluster-management.io/klusterlet-config"] = klusterletConfig.Name
		annotations[constants.ManagedClusterMigrating] = ""
		mc.SetAnnotations(annotations)
		if err := s.client.Update(ctx, mc); err != nil {
			return err
		}
	}
	// send KlusterletAddonConfig to the global hub and then propogate to the target cluster
	for _, managedCluster := range managedClusters {
		if err := s.sendKlusterletAddonConfig(ctx, managedCluster, ""); err != nil {
			return err
		}
	}

	// wait for 10 seconds to ensure the klusterletconfig is applied and then trigger the migration
	// right now, no condition indicates the klusterletconfig is applied
	time.Sleep(sleepForApplying)
	for _, managedCluster := range managedClusters {
		mc := &clusterv1.ManagedCluster{}
		if err := s.client.Get(ctx, types.NamespacedName{
			Name: managedCluster,
		}, mc); err != nil {
			return err
		}
		mc.Spec.HubAcceptsClient = false
		s.log.Infof("updating managedcluster %s to set HubAcceptsClient as false", mc.Name)
		if err := s.client.Update(ctx, mc); err != nil {
			return err
		}
	}
	time.Sleep(sleepForApplying)
	if err := s.detachManagedClusters(ctx, managedClusters); err != nil {
		s.log.Error(err, "failed to detach managed clusters")
		return err
	}

	return nil
}

func (m *managedClusterMigrationFromSyncer) registering(
	ctx context.Context, migratingEvt *bundleevent.ManagedClusterMigrationFromEvent,
) error {
	// ensure bootstrap kubeconfig secret
	bootstrapSecret := migratingEvt.BootstrapSecret
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
		if migrating && annotations["agent.open-cluster-management.io/klusterlet-config"] == klusterletConfig.Name {
			continue
		}
		annotations["agent.open-cluster-management.io/klusterlet-config"] = klusterletConfig.Name
		annotations[constants.ManagedClusterMigrating] = ""
		mc.SetAnnotations(annotations)
		if err := m.client.Update(ctx, mc); err != nil {
			return err
		}
	}

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

func (m *managedClusterMigrationFromSyncer) ensureKlusterletConfig(toHub string) *klusterletv1alpha1.KlusterletConfig {
	return &klusterletv1alpha1.KlusterletConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: klusterletConfigNamePrefix + toHub,
		},
		Spec: klusterletv1alpha1.KlusterletConfigSpec{
			BootstrapKubeConfigs: operatorv1.BootstrapKubeConfigs{
				Type: operatorv1.LocalSecrets,
				LocalSecrets: operatorv1.LocalSecretsConfig{
					KubeConfigSecrets: []operatorv1.KubeConfigSecret{
						{
							Name: bootstrapSecretNamePrefix + toHub,
						},
					},
				},
			},
		},
	}
}

// sendKlusterletAddonConfig sends the klusterletAddonConfig back to the global hub
func (s *managedClusterMigrationFromSyncer) sendKlusterletAddonConfig(ctx context.Context,
	managedCluster string, toHub string,
) error {
	config := &addonv1.KlusterletAddonConfig{}
	// send klusterletAddonConfig to global hub so that it can be transferred to the target cluster
	if err := s.client.Get(ctx, types.NamespacedName{
		Name:      managedCluster,
		Namespace: managedCluster,
	}, config); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		return nil
	}
	// do cleanup
	config.SetManagedFields(nil)
	config.SetFinalizers(nil)
	config.SetOwnerReferences(nil)
	config.SetSelfLink("")
	config.SetResourceVersion("")
	config.SetGeneration(0)
	config.Status = addonv1.KlusterletAddonConfigStatus{}

	payloadBytes, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal klusterletAddonConfig (%v) - %w", config, err)
	}

	s.bundleVersion.Incr()
	e := cloudevents.NewEvent()
	e.SetType(string(enum.KlusterletAddonConfigType))

	e.SetSource(configs.GetLeafHubName())
	e.SetExtension(constants.CloudEventExtensionKeyClusterName, toHub)
	e.SetExtension(eventversion.ExtVersion, s.bundleVersion.String())
	_ = e.SetData(cloudevents.ApplicationJSON, payloadBytes)
	if s.transportClient != nil {
		if err := s.transportClient.GetProducer().SendEvent(ctx, e); err != nil {
			return fmt.Errorf("failed to send klusterletAddonConfig back to the global hub, due to %v", err)
		}
		s.bundleVersion.Next()
	}
	return nil
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
				return err
			}
		}
	}
	return nil
}
