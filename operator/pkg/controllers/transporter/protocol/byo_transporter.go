package protocol

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
}

var byoTransporter *BYOTransporter

// create the transport with secret(BYO case), it should meet the following conditions
// 1. name: "multicluster-global-hub-transport"
// 2. properties: "bootstrap_server", "ca.crt", "client.crt" and "client.key"
func NewBYOTransporter(ctx context.Context, namespacedName types.NamespacedName,
	c client.Client,
) *BYOTransporter {
	if byoTransporter == nil {
		byoTransporter = &BYOTransporter{
			log:           logger.ZaprLogger(),
			ctx:           ctx,
			runtimeClient: c,
		}
		config.SetTransporter(byoTransporter)
	}
	byoTransporter.name = namespacedName.Name
	byoTransporter.namespace = namespacedName.Namespace
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

func (s *BYOTransporter) EnsureKafka() (bool, error) {
	// do nothing
	return false, nil
}

func (s *BYOTransporter) Prune(clusterName string) error {
	return nil
}

func (s *BYOTransporter) GetConnCredential(clusterName string) (*transport.KafkaConfig, error) {
	kafkaSecret := &corev1.Secret{}
	err := s.runtimeClient.Get(s.ctx, types.NamespacedName{
		Name:      s.name,
		Namespace: s.namespace,
	}, kafkaSecret)
	if err != nil {
		return nil, err
	}

	mgh, err := config.GetMulticlusterGlobalHub(context.Background(), s.runtimeClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get mgh: %w", err)
	}

	return &transport.KafkaConfig{
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
