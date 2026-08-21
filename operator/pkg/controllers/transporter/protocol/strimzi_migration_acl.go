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
	"fmt"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func migrationWriteACLKey(topic string) string {
	return utils.GenerateACLKey(utils.WriteTopicACL(topic))
}

func migrationReadACLKey(topic string) string {
	return utils.GenerateACLKey(utils.ReadTopicACL(topic, false))
}

func (k *strimziTransporter) hasMigrationWriteACL(acls []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem) bool {
	key := migrationWriteACLKey(config.GetMigrationTopic())
	for _, acl := range acls {
		if utils.GenerateACLKey(acl) == key {
			return true
		}
	}
	return false
}

func (k *strimziTransporter) hasMigrationReadACL(acls []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem) bool {
	key := migrationReadACLKey(config.GetMigrationTopic())
	for _, acl := range acls {
		if utils.GenerateACLKey(acl) == key {
			return true
		}
	}
	return false
}

// SyncMigrationWriteACL grants or revokes Write on the migration topic for a source hub.
func (k *strimziTransporter) SyncMigrationWriteACL(fromHub string, grant bool) error {
	return k.syncMigrationACL(fromHub, grant, k.hasMigrationWriteACL,
		migrationWriteACLKey, utils.WriteTopicACL)
}

// SyncMigrationReadACL grants or revokes Read+Describe on the migration topic for a hub
// involved in an active migration.
func (k *strimziTransporter) SyncMigrationReadACL(hub string, grant bool) error {
	return k.syncMigrationACL(hub, grant, k.hasMigrationReadACL,
		migrationReadACLKey, func(topic string) kafkav1beta2.KafkaUserSpecAuthorizationAclsElem {
			return utils.ReadTopicACL(topic, false)
		})
}

// syncMigrationACL is the shared implementation for granting/revoking a single ACL
// type (Read or Write) on the migration topic for a given hub's KafkaUser.
func (k *strimziTransporter) syncMigrationACL(
	hub string,
	grant bool,
	hasACL func([]kafkav1beta2.KafkaUserSpecAuthorizationAclsElem) bool,
	keyFn func(string) string,
	aclFn func(string) kafkav1beta2.KafkaUserSpecAuthorizationAclsElem,
) error {
	if hub == "" {
		return nil
	}

	userName := config.GetKafkaUserName(hub)
	kafkaUser := &kafkav1beta2.KafkaUser{}
	err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
		Name:      userName,
		Namespace: k.kafkaClusterNamespace,
	}, kafkaUser)
	if errors.IsNotFound(err) {
		if !grant {
			return nil
		}
		return fmt.Errorf("kafka user %s not found for migration ACL", userName)
	}
	if err != nil {
		return fmt.Errorf("get KafkaUser %s/%s: %w", k.kafkaClusterNamespace, userName, err)
	}

	migrationTopic := config.GetMigrationTopic()
	desiredACL := aclFn(migrationTopic)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latestKafkaUser := &kafkav1beta2.KafkaUser{}
		if err := k.manager.GetClient().Get(k.ctx, types.NamespacedName{
			Name:      userName,
			Namespace: k.kafkaClusterNamespace,
		}, latestKafkaUser); err != nil {
			return err
		}

		currentACLs := currentKafkaUserACLs(latestKafkaUser)
		if hasACL(currentACLs) == grant {
			return nil
		}

		updatedACLs := make([]kafkav1beta2.KafkaUserSpecAuthorizationAclsElem, 0, len(currentACLs))
		for _, acl := range currentACLs {
			if utils.GenerateACLKey(acl) == keyFn(migrationTopic) {
				continue
			}
			updatedACLs = append(updatedACLs, acl)
		}
		if grant {
			updatedACLs = append(updatedACLs, desiredACL)
		}

		if len(updatedACLs) == 0 {
			latestKafkaUser.Spec.Authorization = nil
		} else {
			if latestKafkaUser.Spec.Authorization == nil {
				latestKafkaUser.Spec.Authorization = &kafkav1beta2.KafkaUserSpecAuthorization{
					Type: kafkav1beta2.KafkaUserSpecAuthorizationTypeSimple,
				}
			}
			if latestKafkaUser.Spec.Authorization.Type == "" {
				latestKafkaUser.Spec.Authorization.Type = kafkav1beta2.KafkaUserSpecAuthorizationTypeSimple
			}
			latestKafkaUser.Spec.Authorization.Acls = updatedACLs
		}
		return k.manager.GetClient().Update(k.ctx, latestKafkaUser)
	})
}

func currentKafkaUserACLs(kafkaUser *kafkav1beta2.KafkaUser) []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem {
	if kafkaUser == nil || kafkaUser.Spec == nil || kafkaUser.Spec.Authorization == nil {
		return nil
	}
	return kafkaUser.Spec.Authorization.Acls
}
