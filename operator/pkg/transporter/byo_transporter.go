package transporter

import (
	"context"
	"encoding/base64"
	"path/filepath"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type BYOTransporter struct {
	ctx           context.Context
	log           logr.Logger
	name          string
	namespace     string
	runtimeClient client.Client
}

// create the transport with secret(BYO case), it should meet the following conditions
// 1. name: "multicluster-global-hub-transport"
// 2. properties: "bootstrap_server", "ca.crt", "client.crt" and "client.key"
func NewBYOTransporter(ctx context.Context, namespacedName types.NamespacedName,
	c client.Client,
) *BYOTransporter {
	return &BYOTransporter{
		log:           ctrl.Log.WithName("secret-transporter"),
		ctx:           ctx,
		name:          namespacedName.Name,
		namespace:     namespacedName.Namespace,
		runtimeClient: c,
	}
}

func (k *BYOTransporter) GenerateUserName(clusterIdentity string) string {
	return ""
}

func (s *BYOTransporter) CreateAndUpdateUser(name string) error {
	return nil
}

func (k *BYOTransporter) DeleteUser(username string) error {
	return nil
}

// create the transport topic(KafkaTopic) if not exist for each hub clusters
func (s *BYOTransporter) CreateAndUpdateTopic(topic *transport.ClusterTopic) error {
	return nil
}

func (k *BYOTransporter) DeleteTopic(topic *transport.ClusterTopic) error {
	return nil
}

// authorize
func (k *BYOTransporter) GrantRead(userName string, topicName string) error {
	return nil
}

func (k *BYOTransporter) GrantWrite(userName string, topicName string) error {
	return nil
}

func (k *BYOTransporter) GenerateClusterTopic(clusterIdentity string) *transport.ClusterTopic {
	return &transport.ClusterTopic{
		SpecTopic:   "spec",
		StatusTopic: "status",
		EventTopic:  "event",
	}
}

func (s *BYOTransporter) GetConnCredential(username string) (*transport.ConnCredential, error) {
	kafkaSecret := &corev1.Secret{}
	err := s.runtimeClient.Get(s.ctx, types.NamespacedName{
		Name:      s.name,
		Namespace: s.namespace,
	}, kafkaSecret)
	if err != nil {
		return nil, err
	}
	return &transport.ConnCredential{
		Identity:        string(kafkaSecret.Data[filepath.Join("bootstrap_server")]),
		BootstrapServer: string(kafkaSecret.Data[filepath.Join("bootstrap_server")]),
		CACert:          base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("ca.crt")]),
		ClientCert:      base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("client.crt")]),
		ClientKey:       base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("client.key")]),
	}, nil
}
