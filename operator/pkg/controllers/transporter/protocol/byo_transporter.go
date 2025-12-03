package protocol

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type BYOTransporter struct {
	ctx           context.Context
	log           logr.Logger
	name          string
	namespace     string
	runtimeClient client.Client
	mgh           *v1alpha4.MulticlusterGlobalHub
}

var byoTransporter *BYOTransporter

// create the transport with secret(BYO case), it should meet the following conditions
// 1. name: "multicluster-global-hub-transport"
// 2. properties: "bootstrap_server", "ca.crt", "client.crt" and "client.key"
func EnsureBYOTransport(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub,
	c client.Client,
) *BYOTransporter {
	if byoTransporter == nil {
		byoTransporter = &BYOTransporter{
			log:           logger.ZaprLogger(),
			ctx:           ctx,
			runtimeClient: c,
			mgh:           mgh,
		}
	}
	byoTransporter.name = mgh.Name
	byoTransporter.namespace = mgh.Namespace
	return byoTransporter
}

func (s *BYOTransporter) EnsureUser(clusterName string) (string, error) {
	return "", nil
}

func (s *BYOTransporter) EnsureTopic(clusterName string) (*transport.ClusterTopic, error) {
	return &transport.ClusterTopic{
		SpecTopic:   config.GetSpecTopic(),
		StatusTopic: config.GetStatusTopic(clusterName),
	}, nil
}

func (s *BYOTransporter) Validate() error {
	// set the topic
	specTopic := s.mgh.Spec.DataLayerSpec.Kafka.KafkaTopics.SpecTopic
	statusTopic := s.mgh.Spec.DataLayerSpec.Kafka.KafkaTopics.StatusTopic
	if !config.IsValidKafkaTopicName(specTopic) {
		return fmt.Errorf("the specTopic is invalid: %s", specTopic)
	}
	if !config.IsValidKafkaTopicName(statusTopic) {
		return fmt.Errorf("the specTopic is invalid: %s", statusTopic)
	}

	// BYO Case:
	// 1. change the default status topic from 'gh-status.*' to 'gh-status'
	// 2. ensure the status topic must not contain '*'
	if statusTopic == DEFAULT_STATUS_TOPIC {
		s.mgh.Spec.DataLayerSpec.Kafka.KafkaTopics.StatusTopic = DEFAULT_SHARED_STATUS_TOPIC
		statusTopic = DEFAULT_SHARED_STATUS_TOPIC
		if err := s.runtimeClient.Update(s.ctx, s.mgh); err != nil {
			return fmt.Errorf("failed to update the topic from %s to %s, err:%v",
				DEFAULT_STATUS_TOPIC, DEFAULT_SHARED_STATUS_TOPIC, err)
		}
	}

	if strings.Contains(statusTopic, "*") {
		return fmt.Errorf("status topic(%s) must not contain '*' for the BYO kafka", statusTopic)
	}
	return nil
}

func (s *BYOTransporter) Initialize() error {
	// do nothing
	return nil
}

func (s *BYOTransporter) Prune(clusterName string) error {
	return nil
}

func (s *BYOTransporter) GetConnCredential(clusterName string) (bool, *transport.KafkaConfig, error) {
	kafkaSecret := &corev1.Secret{}
	err := s.runtimeClient.Get(s.ctx, types.NamespacedName{
		Name:      s.name,
		Namespace: s.namespace,
	}, kafkaSecret)
	if err != nil {
		return false, nil, err
	}

	mgh, err := config.GetMulticlusterGlobalHub(context.Background(), s.runtimeClient)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get mgh: %w", err)
	}

	if mgh == nil {
		return false, nil, fmt.Errorf("multicluster global hub instance not found")
	}

	return true, &transport.KafkaConfig{
		ClusterID:       string(kafkaSecret.Data[filepath.Join("bootstrap_server")]),
		BootstrapServer: string(kafkaSecret.Data[filepath.Join("bootstrap_server")]),
		ConsumerGroupID: config.GetConsumerGroupID(mgh.Spec.DataLayerSpec.Kafka.ConsumerGroupPrefix, clusterName),

		// for the byo case, the status topic isn't change by the clusterName
		StatusTopic: config.GetStatusTopic(""),
		SpecTopic:   config.GetSpecTopic(),
		CACert:      base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("ca.crt")]),
		ClientCert:  base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("client.crt")]),
		ClientKey:   base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("client.key")]),
	}, nil
}
