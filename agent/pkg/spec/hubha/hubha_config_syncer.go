package hubha

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/util/retry"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	haconfigbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/haconfig"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var log = logger.DefaultZapLogger()

const (
	klusterletConfigAnnotation = "agent.open-cluster-management.io/klusterlet-config"
	klusterletConfigPrefix     = "ha-standby-"
)

type HAConfigSyncer struct {
	client      client.Client
	leafHubName string
}

func NewHAConfigSyncer(client client.Client,
	agentConfig *configs.AgentConfig,
) *HAConfigSyncer {
	return &HAConfigSyncer{
		client:      client,
		leafHubName: agentConfig.LeafHubName,
	}
}

func (s *HAConfigSyncer) Sync(ctx context.Context, evt *cloudevents.Event) error {
	bundle := &haconfigbundle.HAConfigBundle{}
	if err := json.Unmarshal(evt.Data(), bundle); err != nil {
		return fmt.Errorf("failed to unmarshal HA config bundle: %w", err)
	}
	activeHub := evt.Subject()
	standbyHub := evt.Source()
	log.Infof("received HA config event: activeHub=%s, standbyHub=%s", activeHub, standbyHub)

	if bundle.BootstrapSecret == nil {
		return fmt.Errorf("bootstrap secret is nil in HA config bundle")
	}

	if err := s.ensureBootstrapSecret(ctx, bundle.BootstrapSecret); err != nil {
		return fmt.Errorf("failed to ensure bootstrap secret: %w", err)
	}

	klusterletConfigName, err := s.ensureKlusterletConfig(ctx, standbyHub,
		bundle.BootstrapSecret.Name)
	if err != nil {
		return fmt.Errorf("failed to ensure klusterlet config: %w", err)
	}

	if err := s.annotateAllManagedClusters(ctx, klusterletConfigName); err != nil {
		return fmt.Errorf("failed to annotate managed clusters: %w", err)
	}

	configs.GetAgentConfig().SetStandbyHub(standbyHub)

	log.Infof("HA config applied: klusterletConfig=%s", klusterletConfigName)
	return nil
}

func (s *HAConfigSyncer) ensureBootstrapSecret(ctx context.Context, bootstrapSecret *corev1.Secret) error {
	currentSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:      bootstrapSecret.Name,
		Namespace: bootstrapSecret.Namespace,
	}}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		operation, err := controllerutil.CreateOrUpdate(ctx, s.client, currentSecret, func() error {
			currentSecret.Data = bootstrapSecret.Data
			return nil
		})
		log.Infof("bootstrap secret %s/%s is %s", bootstrapSecret.Namespace, bootstrapSecret.Name, operation)
		return err
	})
}

func (s *HAConfigSyncer) ensureKlusterletConfig(ctx context.Context,
	standbyHubName, bootstrapSecretName string,
) (string, error) {
	mch, err := utils.ListMCH(ctx, s.client)
	if err != nil {
		return "", err
	}
	if mch == nil {
		return "", fmt.Errorf("no MCH found")
	}

	configName := klusterletConfigPrefix + standbyHubName

	klusterletConfig213 := fmt.Sprintf(`
apiVersion: config.open-cluster-management.io/v1alpha1
kind: KlusterletConfig
metadata:
  name: %s
spec:
  bootstrapKubeConfigs:
    type: "LocalSecrets"
    localSecretsConfig:
      kubeConfigSecrets:
      - name: "%s"`, configName, bootstrapSecretName)

	klusterletConfig214 := fmt.Sprintf(`
apiVersion: config.open-cluster-management.io/v1alpha1
kind: KlusterletConfig
metadata:
  name: %s
spec:
  multipleHubsConfig:
    genBootstrapKubeConfigStrategy: "IncludeCurrentHub"
    bootstrapKubeConfigs:
      type: "LocalSecrets"
      localSecretsConfig:
        kubeConfigSecrets:
        - name: "%s"`, configName, bootstrapSecretName)

	klusterletConfigYAML := klusterletConfig214
	if strings.Contains(mch.Status.CurrentVersion, "2.13") {
		klusterletConfigYAML = klusterletConfig213
	}

	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	obj := &unstructured.Unstructured{}
	_, _, err = dec.Decode([]byte(klusterletConfigYAML), nil, obj)
	if err != nil {
		return "", err
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(obj.GroupVersionKind())
	if err := s.client.Get(ctx, client.ObjectKeyFromObject(obj), existing); err != nil {
		if apierrors.IsNotFound(err) {
			if err := s.client.Create(ctx, obj); err != nil {
				return "", err
			}
			log.Infof("created KlusterletConfig %s", configName)
			return configName, nil
		}
		return "", err
	}

	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := s.client.Get(ctx, client.ObjectKeyFromObject(obj), existing); err != nil {
			return err
		}
		existing.Object["spec"] = obj.Object["spec"]
		return s.client.Update(ctx, existing)
	}); err != nil {
		return "", fmt.Errorf("failed to update KlusterletConfig %s: %w", configName, err)
	}
	log.Infof("updated KlusterletConfig %s", configName)
	return configName, nil
}

func (s *HAConfigSyncer) annotateAllManagedClusters(ctx context.Context, klusterletConfigName string) error {
	mcList := &clusterv1.ManagedClusterList{}
	if err := s.client.List(ctx, mcList); err != nil {
		return err
	}

	for i := range mcList.Items {
		mc := &mcList.Items[i]
		if mc.Name == "local-cluster" {
			continue
		}

		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := s.client.Get(ctx, client.ObjectKeyFromObject(mc), mc); err != nil {
				return err
			}
			annotations := mc.GetAnnotations()
			if annotations == nil {
				annotations = make(map[string]string)
			}
			if annotations[klusterletConfigAnnotation] == klusterletConfigName {
				return nil
			}
			annotations[klusterletConfigAnnotation] = klusterletConfigName
			mc.SetAnnotations(annotations)
			if err := s.client.Update(ctx, mc); err != nil {
				return err
			}
			log.Infof("annotated managed cluster %s with klusterlet-config=%s", mc.Name, klusterletConfigName)
			return nil
		}); err != nil {
			return fmt.Errorf("failed to annotate managed cluster %s: %w", mc.Name, err)
		}
	}
	return nil
}
