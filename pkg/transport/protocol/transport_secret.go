package protocol

import (
	"context"
	"encoding/base64"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type transportSecret struct {
	ctx           context.Context
	log           logr.Logger
	name          string
	namespace     string
	runtimeClient client.Client
}

// create the transport with secret, it should meet the following conditions
// 1. name: "multicluster-global-hub-transport"
// 2. properties: "bootstrap_server", "ca.crt", "client.crt" and "client.key"
func NewTransportSecret(ctx context.Context, namespacedName types.NamespacedName, c client.Client) *transportSecret {
	return &transportSecret{
		log:           ctrl.Log.WithName("secret-transport"),
		ctx:           ctx,
		name:          namespacedName.Name,
		namespace:     namespacedName.Namespace,
		runtimeClient: c,
	}
}

func (s *transportSecret) CreateUser(name string) error {
	return nil
}

func (k *transportSecret) DeleteUser(username string) error {
	return nil
}

// create the transport topic(KafkaTopic) if not exist for each hub clusters
func (s *transportSecret) CreateTopic(names []string) error {
	return nil
}

func (k *transportSecret) DeleteTopic(names []string) error {
	return nil
}

func (k *transportSecret) GetTopicNames(clusterIdentity string) *transport.ClusterTopic {
	return &transport.ClusterTopic{
		SpecTopic:   "spec",
		StatusTopic: "status",
		EventTopic:  "event",
	}
}

func (k *transportSecret) GetUserName(clusterIdentity string) string {
	return ""
}

func (s *transportSecret) GetConnCredential(username string) (*transport.ConnCredential, error) {
	kafkaSecret := &corev1.Secret{}
	err := s.runtimeClient.Get(s.ctx, types.NamespacedName{
		Name:      s.name,
		Namespace: s.namespace,
	}, kafkaSecret)
	if err != nil {
		return nil, err
	}
	return &transport.ConnCredential{
		BootstrapServer: string(kafkaSecret.Data[filepath.Join("bootstrap_server")]),
		CACert:          base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("ca.crt")]),
		ClientCert:      base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("client.crt")]),
		ClientKey:       base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("client.key")]),
	}, nil
}
