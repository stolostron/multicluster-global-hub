// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package syncers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
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
		log:             logger.DefaultZapLogger(),
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
		s.log.Infof("initializing managed cluster migration event")
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
		s.log.Infof("registering managed cluster migration event")
		if err := s.registering(ctx, managedClusterMigrationEvent); err != nil {
			return err
		}
		return nil
	}

	if managedClusterMigrationEvent.Stage == migrationv1alpha1.PhaseCompleted {
		s.log.Infof("completed managed cluster migration event")
		if err := s.cleanup(ctx, managedClusterMigrationEvent); err != nil {
			return err
		}
		return nil
	}
	return nil
}

func (m *managedClusterMigrationFromSyncer) cleanup(
	ctx context.Context, migratingEvt *bundleevent.ManagedClusterMigrationFromEvent,
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

	// ensure klusterletconfig
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
	return nil
}

func (m *managedClusterMigrationFromSyncer) registering(
	ctx context.Context, migratingEvt *bundleevent.ManagedClusterMigrationFromEvent,
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

	return SendEvent(ctx, s.transportClient, string(enum.KlusterletAddonConfigType), configs.GetLeafHubName(),
		toHub, payloadBytes, s.bundleVersion)
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
				return err
			}
		}
	}
	return nil
}
