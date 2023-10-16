package hubofhubs

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/kafka"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
)

// EnsureKafkaSubscription verifies subscription needed for Kafka is created
func (r *MulticlusterGlobalHubReconciler) EnsureKafkaSubscription(ctx context.Context,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub,
) error {
	kafkaSub, err := utils.GetSubscriptionByName(ctx, r.Client, kafka.SubscriptionName)
	if err != nil {
		return err
	}

	// Generate sub config from mcgh CR
	subConfig := &subv1alpha1.SubscriptionConfig{
		NodeSelector: mgh.Spec.NodeSelector,
		Tolerations:  mgh.Spec.Tolerations,
	}

	createSub := false
	if kafkaSub == nil {
		// Sub is nil so create a new one
		kafkaSub = kafka.NewSubscription(mgh, subConfig, utils.IsCommunityMode())
		createSub = true
	}

	// Apply kafka sub
	calcSub := kafka.RenderSubscription(kafkaSub, subConfig, utils.IsCommunityMode())
	if createSub {
		err = r.Client.Create(ctx, calcSub)
	} else {
		if !equality.Semantic.DeepEqual(kafkaSub.Spec, calcSub.Spec) {
			err = r.Client.Update(ctx, calcSub)
		}
	}
	if err != nil {
		return fmt.Errorf("error updating subscription %s: %w", calcSub.Name, err)
	}

	return nil
}

// EnsureKafkaResources verifies resources needed for Kafka are created
// including kafka/kafkatopic/kafkauser
func (r *MulticlusterGlobalHubReconciler) EnsureKafkaResources(ctx context.Context,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub) error {
	kafkaCluster := &kafkav1beta2.Kafka{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      kafka.KafkaClusterName,
		Namespace: config.GetDefaultNamespace(),
	}, kafkaCluster)
	if err != nil {
		if errors.IsNotFound(err) {
			err = r.Client.Create(ctx, kafka.NewKafka(mgh, kafka.KafkaClusterName, config.GetDefaultNamespace()))
			if err != nil && !errors.IsAlreadyExists(err) {
				return err
			}
		} else {
			return err
		}
	}

	kafkaTopics := &kafkav1beta2.KafkaTopicList{}
	err = r.Client.List(ctx, kafkaTopics, &client.ListOptions{
		Namespace: config.GetDefaultNamespace(),
	})
	if err != nil {
		return err
	}
	for _, kafkaTopicName := range []string{
		kafka.KafkaSpecTopicName, kafka.KafkaStatusTopicName,
		kafka.KafkaEventTopicName,
	} {
		found := false
		for _, topic := range kafkaTopics.Items {
			if topic.Name == kafkaTopicName {
				found = true
				break
			}
		}
		if !found {
			err := r.Client.Create(ctx, kafka.NewKafkaTopic(kafkaTopicName, config.GetDefaultNamespace()))
			if err != nil && !errors.IsAlreadyExists(err) {
				return err
			}
		}
	}

	kafkaUser := &kafkav1beta2.KafkaUser{}
	err = r.Client.Get(ctx, types.NamespacedName{
		Name:      kafka.KafkaUserName,
		Namespace: config.GetDefaultNamespace(),
	}, kafkaUser)
	if err != nil {
		if errors.IsNotFound(err) {
			err = r.Client.Create(ctx, kafka.NewKafkaUser(kafka.KafkaUserName, config.GetDefaultNamespace()))
			if err != nil && !errors.IsAlreadyExists(err) {
				return err
			}
		} else {
			return err
		}
	}

	return nil
}

// WaitForKafkaClusterReady waits for kafka cluster to be ready
// and returns a kafka connection object once ready
func (r *MulticlusterGlobalHubReconciler) WaitForKafkaClusterReady(ctx context.Context) (
	*kafka.KafkaConnection, error,
) {
	kafkaCluster := &kafkav1beta2.Kafka{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      kafka.KafkaClusterName,
		Namespace: config.GetDefaultNamespace(),
	}, kafkaCluster)
	if err != nil {
		return nil, err
	}

	if kafkaCluster.Status == nil || kafkaCluster.Status.Conditions == nil {
		return nil, fmt.Errorf("kafka cluster %s has no status conditions", kafkaCluster.Name)
	}

	kafkaUserSecret := &corev1.Secret{}
	err = r.Client.Get(ctx, types.NamespacedName{
		Name:      kafka.KafkaUserName,
		Namespace: config.GetDefaultNamespace(),
	}, kafkaUserSecret)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("kafka user secret %s is nil", kafka.KafkaUserName)
		}
		return nil, err
	}

	for _, condition := range kafkaCluster.Status.Conditions {
		if *condition.Type == "Ready" && *condition.Status == "True" {
			return &kafka.KafkaConnection{
				BootstrapServer: *kafkaCluster.Status.Listeners[1].BootstrapServers,
				CACert: base64.StdEncoding.EncodeToString(
					[]byte(kafkaCluster.Status.Listeners[1].Certificates[0])),
				ClientCert: base64.StdEncoding.EncodeToString(kafkaUserSecret.Data["user.crt"]),
				ClientKey:  base64.StdEncoding.EncodeToString(kafkaUserSecret.Data["user.key"]),
			}, nil
		}
	}
	return nil, fmt.Errorf("kafka cluster %s is not ready", kafkaCluster.Name)
}

// GenerateKafkaConnectionFromGHTransportSecret returns a kafka connection object from the BYO kafka secret
func (r *MulticlusterGlobalHubReconciler) GenerateKafkaConnectionFromGHTransportSecret(ctx context.Context) (
	*kafka.KafkaConnection, error,
) {
	kafkaSecret := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      constants.GHTransportSecretName,
		Namespace: config.GetDefaultNamespace(),
	}, kafkaSecret)
	if err != nil {
		return nil, err
	}

	return &kafka.KafkaConnection{
		BootstrapServer: string(kafkaSecret.Data[filepath.Join("bootstrap_server")]),
		CACert:          base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("ca.crt")]),
		ClientCert:      base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("client.crt")]),
		ClientKey:       base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("client.key")]),
	}, nil
}
