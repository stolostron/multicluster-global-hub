// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package protocol

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
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

// NewBYOTransporter creates the transport from customer-provided Kafka secrets.
// The manager uses the shared secret "multicluster-global-hub-transport".
// Each managed hub may supply "multicluster-global-hub-transport-<clusterName>"
// so Kafka ACLs can bind a distinct client certificate to that hub. When the
// per-hub secret is absent, the shared secret is used (legacy BYO).
func NewBYOTransporter(ctx context.Context, namespacedName types.NamespacedName,
	c client.Client,
) *BYOTransporter {
	transporter := &BYOTransporter{
		log:           logger.ZaprLogger(),
		ctx:           ctx,
		runtimeClient: c,
		name:          namespacedName.Name,
		namespace:     namespacedName.Namespace,
	}
	config.SetTransporter(transporter)
	return transporter
}

// EnsureUser validates that a BYO transport secret exists for the cluster and
// that per-hub client certificates are distinct. Kafka users and ACLs remain
// customer-managed; see doc/byo.md. A missing secret is logged, not rejected,
// so addon install can proceed before credentials are created. Other API
// errors are returned to the caller.
func (s *BYOTransporter) EnsureUser(clusterName string) (string, error) {
	if isManagerCluster(clusterName) {
		return "", nil
	}
	secret, secretName, err := s.getTransportSecret(clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			s.log.Info("BYO Kafka transport secret not found; provide a per-hub or shared secret")
			return config.GetKafkaUserName(clusterName), nil
		}
		return "", fmt.Errorf("failed to get BYO Kafka transport secret: %w", err)
	}
	if secretName == s.sharedSecretName() {
		s.log.Info("BYO Kafka is using the shared transport secret; " +
			"provide a per-hub secret for isolated credentials")
	}
	if err := s.validateDistinctClientCerts(clusterName, secret.Data[filepath.Join("client.crt")]); err != nil {
		return "", err
	}
	return config.GetKafkaUserName(clusterName), nil
}

func (s *BYOTransporter) EnsureTopic(clusterName string) (*transport.ClusterTopic, error) {
	return &transport.ClusterTopic{
		SpecTopic:      config.GetSpecTopic(),
		MigrationTopic: config.GetMigrationTopic(),
		// BYO Kafka uses one configured status topic for every hub.
		StatusTopic: config.GetStatusTopic(""),
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
	kafkaSecret, _, err := s.getTransportSecret(clusterName)
	if err != nil {
		return nil, err
	}
	if !isManagerCluster(clusterName) {
		if err := s.validateDistinctClientCerts(clusterName, kafkaSecret.Data[filepath.Join("client.crt")]); err != nil {
			return nil, err
		}
	}

	mgh, err := config.GetMulticlusterGlobalHub(s.ctx, s.runtimeClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get mgh: %w", err)
	}

	if mgh == nil {
		return nil, fmt.Errorf("multicluster global hub instance not found")
	}

	return &transport.KafkaConfig{
		ClusterID:       string(kafkaSecret.Data[filepath.Join("bootstrap_server")]),
		BootstrapServer: string(kafkaSecret.Data[filepath.Join("bootstrap_server")]),
		ConsumerGroupID: config.GetConsumerGroupID(mgh.Spec.DataLayerSpec.Kafka.ConsumerGroupPrefix, clusterName),

		// BYO status topic is shared; clusterName does not change it
		StatusTopic:    config.GetStatusTopic(""),
		SpecTopic:      config.GetSpecTopic(),
		MigrationTopic: config.GetMigrationTopic(),
		CACert:         base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("ca.crt")]),
		ClientCert:     base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("client.crt")]),
		ClientKey:      base64.StdEncoding.EncodeToString(kafkaSecret.Data[filepath.Join("client.key")]),
	}, nil
}

func isManagerCluster(clusterName string) bool {
	return clusterName == "" || clusterName == constants.CloudEventGlobalHubClusterName
}

func (s *BYOTransporter) sharedSecretName() string {
	if s.name != "" {
		return s.name
	}
	return constants.GHTransportSecretName
}

func (s *BYOTransporter) getTransportSecret(clusterName string) (*corev1.Secret, string, error) {
	if !isManagerCluster(clusterName) {
		perHubName := constants.GHTransportSecretNameForCluster(clusterName)
		perHubSecret := &corev1.Secret{}
		err := s.runtimeClient.Get(s.ctx, types.NamespacedName{
			Name:      perHubName,
			Namespace: s.namespace,
		}, perHubSecret)
		if err == nil {
			return perHubSecret, perHubName, nil
		}
		if !apierrors.IsNotFound(err) {
			return nil, "", err
		}
	}

	sharedName := s.sharedSecretName()
	sharedSecret := &corev1.Secret{}
	err := s.runtimeClient.Get(s.ctx, types.NamespacedName{
		Name:      sharedName,
		Namespace: s.namespace,
	}, sharedSecret)
	if err != nil {
		if apierrors.IsNotFound(err) && !isManagerCluster(clusterName) {
			return nil, "", fmt.Errorf("BYO Kafka secret %q or %q not found in namespace %q: %w",
				constants.GHTransportSecretNameForCluster(clusterName), sharedName, s.namespace, err)
		}
		return nil, "", err
	}
	return sharedSecret, sharedName, nil
}

func (s *BYOTransporter) validateDistinctClientCerts(clusterName string, clientCert []byte) error {
	if isManagerCluster(clusterName) || len(clientCert) == 0 {
		return nil
	}

	secretList := &corev1.SecretList{}
	if err := s.runtimeClient.List(s.ctx, secretList, client.InNamespace(s.namespace)); err != nil {
		return fmt.Errorf("failed to list BYO transport secrets: %w", err)
	}

	for i := range secretList.Items {
		secret := &secretList.Items[i]
		otherCluster := constants.ClusterNameFromGHTransportSecret(secret.Name)
		if otherCluster == "" || otherCluster == clusterName {
			continue
		}
		otherCert := secret.Data[filepath.Join("client.crt")]
		if len(otherCert) == 0 {
			continue
		}
		if bytes.Equal(clientCert, otherCert) {
			return fmt.Errorf("BYO Kafka client cert for hub %q is identical to hub %q; "+
				"each hub must have a distinct client certificate (secret %s)",
				clusterName, otherCluster, secret.Name)
		}
	}
	return nil
}
