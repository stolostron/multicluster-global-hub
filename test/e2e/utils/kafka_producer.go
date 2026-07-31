// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"context"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cecontext "github.com/cloudevents/sdk-go/v2/context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorconfig "github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	genericproducer "github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
	pkgutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// KafkaEventPublisher sends CloudEvents to Kafka using transport credentials from a cluster secret.
type KafkaEventPublisher struct {
	producer    transport.Producer
	kafkaConfig *transport.KafkaConfig
}

// NewKafkaEventPublisher loads transport-config from namespace and builds a producer.
func NewKafkaEventPublisher(ctx context.Context, c client.Client, namespace string) (*KafkaEventPublisher, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{
		Name:      constants.GHTransportConfigSecret,
		Namespace: namespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("get transport secret in namespace %s: %w", namespace, err)
	}

	kafkaConfig, err := pkgutils.GetKafkaCredentialBySecret(secret, c)
	if err != nil {
		return nil, fmt.Errorf("parse kafka credentials: %w", err)
	}

	transportConfig := &transport.TransportInternalConfig{
		TransportType:   string(transport.Kafka),
		KafkaCredential: kafkaConfig,
	}

	producer, err := genericproducer.NewGenericProducer(transportConfig, kafkaConfig.SpecTopic, nil)
	if err != nil {
		return nil, fmt.Errorf("create kafka producer: %w", err)
	}

	return &KafkaEventPublisher{
		producer:    producer,
		kafkaConfig: kafkaConfig,
	}, nil
}

// SendToTopic publishes evt to the given Kafka topic.
func (p *KafkaEventPublisher) SendToTopic(ctx context.Context, topic string, evt cloudevents.Event) error {
	return p.producer.SendEvent(cecontext.WithTopic(ctx, topic), evt)
}

// SpecTopic returns the configured spec topic (typically gh-spec).
func (p *KafkaEventPublisher) SpecTopic() string {
	return p.kafkaConfig.SpecTopic
}

// MigrationTopic returns the configured migration topic (typically gh-migration).
func (p *KafkaEventPublisher) MigrationTopic() string {
	if p.kafkaConfig.MigrationTopic != "" {
		return p.kafkaConfig.MigrationTopic
	}
	return operatorconfig.GetMigrationTopic()
}

// StatusTopic returns the status topic for hubName (per-hub topic or credential override).
func (p *KafkaEventPublisher) StatusTopic(hubName string) string {
	if p.kafkaConfig.StatusTopic != "" && !strings.Contains(p.kafkaConfig.StatusTopic, "*") {
		return p.kafkaConfig.StatusTopic
	}
	return operatorconfig.GetStatusTopic(hubName)
}
